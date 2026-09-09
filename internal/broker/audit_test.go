package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/michaelquigley/terminus/internal/errs"
	"github.com/michaelquigley/terminus/internal/report"
	"github.com/michaelquigley/theharnessbody/reviewer"
	"github.com/michaelquigley/theharnessbody/reviewer/dummy"
)

type failReviewer struct{}

func (failReviewer) Review(context.Context, reviewer.ReviewRequest) (reviewer.ReviewResponse, error) {
	return reviewer.ReviewResponse{}, errors.New("reviewer must not run during audit")
}

func TestAuditCoverageReadOnlyAndFresh(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n")
	writeFile(t, filepath.Join(repo, "internal", "a.go"), "package internal\n")
	writeFile(t, filepath.Join(repo, "internal", "b.md"), "notes\n")
	writeFile(t, filepath.Join(repo, "docs", "readme.md"), "docs\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")

	canonRoot := auditFixtureCanon(t, filepath.Base(repo))
	blockedLog := filepath.Join(t.TempDir(), "not-a-directory")
	writeFile(t, blockedLog, "do not touch\n")
	b := New(Options{CanonPath: canonRoot, LogDestination: blockedLog, Reviewer: failReviewer{}})
	beforeRepo, err := os.ReadFile(filepath.Join(repo, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	beforeCanon, err := os.ReadFile(filepath.Join(canonRoot, "projects", filepath.Base(repo), "local.md"))
	if err != nil {
		t.Fatal(err)
	}

	result, err := b.AuditCoverage(context.Background(), AuditCoverageRequest{RepoPath: repo, Rubric: "rubric.yaml", IncludeMap: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Project != filepath.Base(repo) || result.Rubric != "rubric" || result.FileScope != "full" {
		t.Fatalf("audit identity = %#v", result)
	}
	if got := *result.Coverage.FileCounts; got.Total != 4 || got.Excluded != 1 || got.Covered != 1 || got.Uncovered != 2 {
		t.Fatalf("audit counts = %#v", got)
	}
	if len(result.DeadPatterns) != 1 || result.DeadPatterns[0].Pattern != "cmd/**" {
		t.Fatalf("dead patterns = %#v", result.DeadPatterns)
	}
	if result.CoverageMap == nil || !mapShowsPerFileDifference(*result.CoverageMap) {
		t.Fatalf("coverage map lost per-file differences: %#v", result.CoverageMap)
	}
	reviewBroker := New(Options{CanonPath: canonRoot, LogDestination: t.TempDir(), Reviewer: dummy.New(dummy.Options{Raw: json.RawMessage(`{"summary":"clean","findings":[]}`)}), ReviewerInfo: ReviewerInfo{Name: "dummy"}})
	fullReview, err := reviewBroker.RunReview(context.Background(), StartReviewRequest{RepoPath: repo, ChangesetKind: "full"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Coverage, fullReview.Coverage) {
		t.Fatalf("audit and full review coverage differ:\n audit: %#v\n review: %#v", result.Coverage, fullReview.Coverage)
	}
	if len(b.jobs) != 0 {
		t.Fatalf("audit allocated jobs: %#v", b.jobs)
	}
	afterRepo, _ := os.ReadFile(filepath.Join(repo, "main.go"))
	afterCanon, _ := os.ReadFile(filepath.Join(canonRoot, "projects", filepath.Base(repo), "local.md"))
	if !reflect.DeepEqual(beforeRepo, afterRepo) || !reflect.DeepEqual(beforeCanon, afterCanon) {
		t.Fatal("audit modified project or canon files")
	}
	if info, err := os.Stat(blockedLog); err != nil || info.IsDir() {
		t.Fatalf("audit touched log destination: info=%v err=%v", info, err)
	}

	writeFile(t, filepath.Join(canonRoot, "projects", filepath.Base(repo), "local.md"), "---\nid: local\n---\n# local\n")
	fresh, err := b.AuditCoverage(context.Background(), AuditCoverageRequest{RepoPath: repo})
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Coverage.FileCounts.Uncovered != 0 || fresh.Coverage.FileCounts.Covered != 3 {
		t.Fatalf("fresh audit did not reload canon: %#v", fresh.Coverage.FileCounts)
	}
	if fresh.CoverageMap != nil {
		t.Fatalf("unrequested map must be omitted: %#v", fresh.CoverageMap)
	}
}

func TestAuditCoverageErrorsAreUserErrors(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	canonRoot := auditFixtureCanon(t, filepath.Base(repo))
	wrongIdentityRoot := auditFixtureCanon(t, filepath.Base(repo))
	writeFile(t, filepath.Join(wrongIdentityRoot, "projects", filepath.Base(repo), "rubric.yaml"), "project:\n  repo: other\nqualities:\n  - ref: projects/"+filepath.Base(repo)+"/local\n")

	tests := []struct {
		name string
		b    *Broker
		req  AuditCoverageRequest
	}{
		{name: "missing repo", b: New(Options{CanonPath: canonRoot}), req: AuditCoverageRequest{}},
		{name: "non repository", b: New(Options{CanonPath: canonRoot}), req: AuditCoverageRequest{RepoPath: t.TempDir()}},
		{name: "wrong project identity", b: New(Options{CanonPath: wrongIdentityRoot}), req: AuditCoverageRequest{RepoPath: repo}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.b.AuditCoverage(context.Background(), test.req)
			var classified *errs.Error
			if !errors.As(err, &classified) || classified.Code != errs.CodeUserError {
				t.Fatalf("error = %v, want user_error", err)
			}
		})
	}

	writeFile(t, filepath.Join(canonRoot, "projects", filepath.Base(repo), "rubric.yaml"), fmt.Sprintf("project:\n  repo: %q\nqualities:\n  - ref: projects/%s/local\ncoverage_exclusions:\n  - 'docs/['\n", filepath.Base(repo), filepath.Base(repo)))
	if _, err := New(Options{CanonPath: canonRoot}).AuditCoverage(context.Background(), AuditCoverageRequest{RepoPath: repo}); err == nil {
		t.Fatal("malformed exclusion must fail")
	}
	malformedQualityRoot := auditFixtureCanon(t, filepath.Base(repo))
	writeFile(t, filepath.Join(malformedQualityRoot, "projects", filepath.Base(repo), "local.md"), "---\nid: local\nterritory:\n  - 'internal/['\n---\n# local\n")
	if _, err := New(Options{CanonPath: malformedQualityRoot}).AuditCoverage(context.Background(), AuditCoverageRequest{RepoPath: repo}); err == nil {
		t.Fatal("malformed quality territory must fail")
	}
}

func auditFixtureCanon(t *testing.T, project string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "general", "style.md"), "---\nid: style\n---\n# style\n")
	writeFile(t, filepath.Join(root, "general", "dead.md"), "---\nid: dead\nterritory:\n  - cmd/**\n---\n# dead\n")
	writeFile(t, filepath.Join(root, "projects", project, "local.md"), "---\nid: local\nterritory:\n  - internal/*.go\n---\n# local\n")
	writeFile(t, filepath.Join(root, "projects", project, "rubric.yaml"), fmt.Sprintf("project:\n  repo: %q\nqualities:\n  - ref: projects/%s/local\n    blocking: true\n  - ref: general/style\n  - ref: general/dead\ncoverage_exclusions:\n  - docs/**\n", project, project))
	return root
}

func mapShowsPerFileDifference(groups []report.MapGroup) bool {
	for _, group := range groups {
		if group.Directory != "internal" || len(group.Files) != 2 {
			continue
		}
		return len(group.Files[0].Qualities) != len(group.Files[1].Qualities)
	}
	return false
}

func TestReviewAndAuditResolveRubricOnce(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "internal", "a.go"), "package internal\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	project := filepath.Base(repo)
	root := coverageFixtureCanon(t, project)
	writeFile(t, filepath.Join(root, "projects", project, "architecture.yaml"), fmt.Sprintf("project:\n  repo: %q\nqualities:\n  - ref: general/style\n", project))
	writeFile(t, filepath.Join(root, "projects", project, "architecture.yaml.yaml"), fmt.Sprintf("project:\n  repo: %q\nqualities:\n  - ref: projects/%s/local\n", project, project))
	b := New(Options{CanonPath: root, LogDestination: t.TempDir(), Reviewer: dummy.New(dummy.Options{Raw: json.RawMessage(`{"summary":"clean","findings":[]}`)})})
	tests := []struct {
		request   string
		identity  string
		selected  int
		local     int
		wantError bool
	}{
		{request: "", identity: "rubric", selected: 2, local: 1},
		{request: "  ", identity: "rubric", selected: 2, local: 1},
		{request: "architecture", identity: "architecture", selected: 1},
		{request: " architecture.yaml ", identity: "architecture", selected: 1},
		{request: "architecture.yaml.yaml", identity: "architecture.yaml", selected: 1, local: 1},
		{request: ".yaml", wantError: true},
		{request: "rubric.yaml.yaml", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.request, func(t *testing.T) {
			audit, auditErr := b.AuditCoverage(context.Background(), AuditCoverageRequest{RepoPath: repo, Rubric: test.request})
			review, reviewErr := b.RunReview(context.Background(), StartReviewRequest{RepoPath: repo, Rubric: test.request, ChangesetKind: "full"})
			if test.wantError {
				if errs.Code(auditErr) != errs.CodeUserError || errs.Code(reviewErr) != errs.CodeUserError {
					t.Fatalf("invalid or missing rubric must fail without fallback: audit=%v review=%v", auditErr, reviewErr)
				}
				return
			}
			if auditErr != nil || reviewErr != nil {
				t.Fatalf("resolve rubric: audit=%v review=%v", auditErr, reviewErr)
			}
			if audit.Rubric != test.identity || review.Rubric != test.identity {
				t.Fatalf("rubric identity: audit=%q review=%q, want %q", audit.Rubric, review.Rubric, test.identity)
			}
			if len(audit.Coverage.LocalQualities) != test.local || len(review.Coverage.LocalQualities) != test.local || review.QualitiesSelected != test.selected {
				t.Fatalf("wrong rubric loaded: audit=%#v review=%#v", audit, review)
			}
		})
	}
}
