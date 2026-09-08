package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/terminus/internal/changeset"
	"github.com/michaelquigley/terminus/internal/errs"
	"github.com/michaelquigley/terminus/internal/monitor"
	"github.com/michaelquigley/terminus/internal/report"
	"github.com/michaelquigley/theharnessbody/reviewer"
	"github.com/michaelquigley/theharnessbody/reviewer/dummy"
)

func TestBrokerReviewWithBlockingFinding(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")

	canonRoot := fixtureCanon(t, filepath.Base(repo))
	raw := json.RawMessage(`{"summary":"saw logging issue","findings":[{"id":"f1","quality":"df-logging","file":"main.go","lines":"1","claim":"uses wrong logger","rationale":"logging should use df/dl","suggestion":"switch to df/dl"}]}`)
	b := New(Options{
		LogDestination: t.TempDir(),
		CanonPath:      canonRoot,
		Reviewer:       dummy.New(dummy.Options{Raw: raw}),
		ReviewerInfo:   ReviewerInfo{Name: "dummy", Impl: "dummy"},
	})

	start, err := b.StartReview(context.Background(), StartReviewRequest{
		RepoPath:      repo,
		ChangesetKind: changeset.KindWorkingTree,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := collectEventually(t, b, CollectReviewRequest{Project: start.Project, ReviewID: start.ReviewID})
	if result.Clean {
		t.Fatal("expected blocking finding to produce not clean")
	}
	if result.Verdict != VerdictNotClean {
		t.Fatalf("verdict = %q", result.Verdict)
	}
	if len(result.Findings) != 1 || !result.Findings[0].Blocking {
		t.Fatalf("unexpected findings: %#v", result.Findings)
	}
	if _, err := os.Stat(result.LogPath); err != nil {
		t.Fatalf("expected findings document: %v", err)
	}
}

func TestBrokerRunReview(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")

	canonRoot := fixtureCanon(t, filepath.Base(repo))
	raw := json.RawMessage(`{"summary":"clean","findings":[]}`)
	b := New(Options{
		LogDestination: t.TempDir(),
		CanonPath:      canonRoot,
		Reviewer:       dummy.New(dummy.Options{Raw: raw}),
		ReviewerInfo:   ReviewerInfo{Name: "dummy", Impl: "dummy"},
	})

	result, err := b.RunReview(context.Background(), StartReviewRequest{
		RepoPath:      repo,
		ChangesetKind: changeset.KindWorkingTree,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Clean || result.Verdict != VerdictClean {
		t.Fatalf("expected clean result, got %#v", result)
	}
	if _, err := os.Stat(result.LogPath); err != nil {
		t.Fatalf("expected findings document: %v", err)
	}
}

// guards the dd migration: a review written to result.json by one broker must
// collect identically from disk through a second broker (collectFromStored +
// dd.BindJSON + the json.RawMessage converter), so the round-trip preserves the
// verdict, findings, and the raw reviewer output.
func TestCollectFromDiskRoundTrip(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")

	canonRoot := fixtureCanon(t, filepath.Base(repo))
	raw := json.RawMessage(`{"summary":"needs work","findings":[{"id":"f1","quality":"df-logging","file":"main.go","lines":"1","claim":"c","rationale":"r","suggestion":"s"}]}`)
	opts := Options{
		LogDestination: t.TempDir(),
		CanonPath:      canonRoot,
		Reviewer:       dummy.New(dummy.Options{Raw: raw}),
		ReviewerInfo:   ReviewerInfo{Name: "dummy", Impl: "dummy"},
	}

	// broker A runs the review, writing status.json + result.json via dd.
	live, err := New(opts).RunReview(context.Background(), StartReviewRequest{RepoPath: repo, ChangesetKind: changeset.KindWorkingTree})
	if err != nil {
		t.Fatal(err)
	}

	// broker B has an empty in-memory job map, so collect reads from disk.
	got, err := New(opts).CollectReview(context.Background(), CollectReviewRequest{Project: live.Project, ReviewID: live.ReviewID})
	if err != nil {
		t.Fatalf("collect from disk: %v", err)
	}

	if got.Verdict != live.Verdict || got.Clean != live.Clean {
		t.Fatalf("verdict drift through disk: got %q/%v, live %q/%v", got.Verdict, got.Clean, live.Verdict, live.Clean)
	}
	if !reflect.DeepEqual(got.Findings, live.Findings) {
		t.Fatalf("findings drift through disk:\n got:  %#v\n live: %#v", got.Findings, live.Findings)
	}
	// the raw reviewer output must survive the converter round-trip (semantically;
	// disk re-serialization may reorder keys, so compare parsed JSON).
	var gotRaw, liveRaw any
	if err := json.Unmarshal(got.Raw, &gotRaw); err != nil {
		t.Fatalf("disk Raw is not valid JSON: %v (%s)", err, got.Raw)
	}
	if err := json.Unmarshal(live.Raw, &liveRaw); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotRaw, liveRaw) {
		t.Fatalf("raw drift through disk round-trip:\n disk: %s\n live: %s", got.Raw, live.Raw)
	}
}

// a canon whose rubric lists a quality whose territory reaches no
// starting-point file, so territory narrowing excludes it from the review.
func cmdScopedCanon(t *testing.T, project string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go-conventions", "df-logging.md"), `---
id: df-logging
territory:
  - "**/*.go"
---
# df logging
`)
	writeFile(t, filepath.Join(root, "go-conventions", "cmd-only.md"), `---
id: cmd-only
territory:
  - "cmd/"
---
# cmd only
`)
	writeFile(t, filepath.Join(root, "projects", project, "rubric.yaml"), fmt.Sprintf(`project:
  repo: %q
qualities:
  - ref: go-conventions/df-logging
    blocking: true
  - ref: go-conventions/cmd-only
    blocking: true
`, project))
	return root
}

// guards the territory-exclusion reporting: a rubric quality whose territory
// reaches no starting-point file is dropped from the review but named in the
// result, so a clean verdict is honest about how much of the rubric it
// covered — and the fields survive the result.json round-trip.
func TestBrokerReportsTerritoryExcludedQualities(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")

	canonRoot := cmdScopedCanon(t, filepath.Base(repo))
	opts := Options{
		LogDestination: t.TempDir(),
		CanonPath:      canonRoot,
		Reviewer:       dummy.New(dummy.Options{Raw: json.RawMessage(`{"summary":"clean","findings":[]}`)}),
		ReviewerInfo:   ReviewerInfo{Name: "dummy", Impl: "dummy"},
	}

	live, err := New(opts).RunReview(context.Background(), StartReviewRequest{RepoPath: repo, ChangesetKind: changeset.KindWorkingTree})
	if err != nil {
		t.Fatal(err)
	}
	if live.QualitiesSelected != 1 {
		t.Fatalf("expected 1 selected quality, got %d", live.QualitiesSelected)
	}
	if len(live.ExcludedQualities) != 1 || live.ExcludedQualities[0].ID != "cmd-only" || !live.ExcludedQualities[0].Blocking {
		t.Fatalf("unexpected excluded qualities: %#v", live.ExcludedQualities)
	}

	// a second broker reads from disk, so the fields must survive result.json.
	got, err := New(opts).CollectReview(context.Background(), CollectReviewRequest{Project: live.Project, ReviewID: live.ReviewID})
	if err != nil {
		t.Fatalf("collect from disk: %v", err)
	}
	if got.QualitiesSelected != live.QualitiesSelected || !reflect.DeepEqual(got.ExcludedQualities, live.ExcludedQualities) {
		t.Fatalf("exclusion fields drifted through disk:\n got:  %d %#v\n live: %d %#v", got.QualitiesSelected, got.ExcludedQualities, live.QualitiesSelected, live.ExcludedQualities)
	}
}

// an explicitly named quality bypasses territory narrowing: a human naming it
// by hand has already made the judgment the filter automates, so the review
// runs it even when its territory matches no starting-point file.
func TestBrokerAdHocBypassesTerritory(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")

	canonRoot := cmdScopedCanon(t, filepath.Base(repo))
	if err := os.Remove(filepath.Join(canonRoot, "projects", filepath.Base(repo), "rubric.yaml")); err != nil {
		t.Fatal(err)
	}
	b := New(Options{
		LogDestination: t.TempDir(),
		CanonPath:      canonRoot,
		Reviewer:       dummy.New(dummy.Options{Raw: json.RawMessage(`{"summary":"clean","findings":[]}`)}),
		ReviewerInfo:   ReviewerInfo{Name: "dummy", Impl: "dummy"},
	})

	result, err := b.RunReview(context.Background(), StartReviewRequest{
		RepoPath:      repo,
		ChangesetKind: changeset.KindWorkingTree,
		Qualities:     []string{"go-conventions/cmd-only"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.QualitiesSelected != 1 {
		t.Fatalf("expected the named quality to be selected despite territory, got %d selected", result.QualitiesSelected)
	}
	if len(result.ExcludedQualities) != 0 {
		t.Fatalf("expected no excluded qualities in an ad-hoc review, got %#v", result.ExcludedQualities)
	}
	if result.Coverage == nil || result.Coverage.Assessed || result.Coverage.Reason != "ad_hoc" {
		t.Fatalf("expected ad-hoc coverage distinction, got %#v", result.Coverage)
	}
}

type blockedReviewer struct {
	started chan struct{}
	release chan struct{}
	request reviewer.ReviewRequest
	raw     json.RawMessage
}

func (r *blockedReviewer) Review(ctx context.Context, req reviewer.ReviewRequest) (reviewer.ReviewResponse, error) {
	r.request = req
	close(r.started)
	select {
	case <-r.release:
	case <-ctx.Done():
		return reviewer.ReviewResponse{}, ctx.Err()
	}
	return reviewer.ReviewResponse{Raw: append(json.RawMessage(nil), r.raw...)}, nil
}

func TestReviewCoveragePersistsAcrossBoundaries(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n")
	writeFile(t, filepath.Join(repo, "internal", "covered.go"), "package internal\n")
	writeFile(t, filepath.Join(repo, "docs", "readme.md"), "docs\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")

	canonRoot := coverageFixtureCanon(t, filepath.Base(repo))
	logDir := t.TempDir()
	findingRaw := json.RawMessage(`{"summary":"found excluded-file issue","findings":[{"id":"f1","quality":"local","file":"docs/readme.md","lines":"1","claim":"docs issue","rationale":"local invariant applies through review tracing","suggestion":null}]}`)
	blocked := &blockedReviewer{
		started: make(chan struct{}),
		release: make(chan struct{}),
		raw:     findingRaw,
	}
	opts := Options{
		LogDestination: logDir,
		CanonPath:      canonRoot,
		Reviewer:       blocked,
		ReviewerInfo:   ReviewerInfo{Name: "dummy"},
	}
	b := New(opts)
	start, err := b.StartReview(context.Background(), StartReviewRequest{RepoPath: repo, ChangesetKind: changeset.KindFull})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-blocked.started:
	case <-time.After(2 * time.Second):
		t.Fatal("reviewer did not block")
	}
	status, err := monitor.ReadStatus(start.StatusPath)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != monitor.StateRunning || status.Coverage == nil {
		t.Fatalf("running status lost coverage: %#v", status)
	}
	initialCoverage := report.CloneCoverage(status.Coverage)
	writeFile(t, filepath.Join(canonRoot, "projects", filepath.Base(repo), "local.md"), "---\nid: local\nterritory:\n  - cmd/**\n---\n# edited local\n")
	close(blocked.release)

	live := collectEventually(t, b, CollectReviewRequest{Project: start.Project, ReviewID: start.ReviewID})
	if !reflect.DeepEqual(initialCoverage, live.Coverage) {
		t.Fatalf("live collection coverage drifted:\n initial: %#v\n live: %#v", initialCoverage, live.Coverage)
	}
	if !strings.Contains(blocked.request.Prompt, "projects/"+filepath.Base(repo)+"/local") {
		t.Fatal("prompt lost selected local quality")
	}
	assertExcludedFileFinding(t, live)

	status, err = monitor.ReadStatus(start.StatusPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(initialCoverage, status.Coverage) {
		t.Fatalf("completed status coverage drifted:\n initial: %#v\n status: %#v", initialCoverage, status.Coverage)
	}
	disk, err := New(opts).CollectReview(context.Background(), CollectReviewRequest{Project: live.Project, ReviewID: live.ReviewID})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(initialCoverage, disk.Coverage) {
		t.Fatalf("disk collection coverage drifted:\n initial: %#v\n disk: %#v", initialCoverage, disk.Coverage)
	}

	live.Coverage.UncoveredFiles[0] = "mutated.go"
	live.Coverage.Exclusions[0].Files[0] = "mutated.go"
	again, err := b.CollectReview(context.Background(), CollectReviewRequest{Project: live.Project, ReviewID: live.ReviewID})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(initialCoverage, again.Coverage) {
		t.Fatalf("same-broker coverage was mutated:\n initial: %#v\n again: %#v", initialCoverage, again.Coverage)
	}

	writeFile(t, filepath.Join(canonRoot, "projects", filepath.Base(repo), "local.md"), "---\nid: local\nterritory:\n  - internal/**\n---\n# local\n")
	writeFile(t, filepath.Join(canonRoot, "projects", filepath.Base(repo), "rubric.yaml"), fmt.Sprintf("project:\n  repo: %q\nqualities:\n  - ref: projects/%s/local\n    blocking: true\n  - ref: general/style\n    blocking: false\n", filepath.Base(repo), filepath.Base(repo)))
	unexcludedReviewer := dummy.New(dummy.Options{Raw: findingRaw})
	opts.Reviewer = unexcludedReviewer
	unexcluded, err := New(opts).RunReview(context.Background(), StartReviewRequest{RepoPath: repo, ChangesetKind: changeset.KindFull})
	if err != nil {
		t.Fatal(err)
	}
	assertExcludedFileFinding(t, unexcluded)
	if got := *unexcluded.Coverage.FileCounts; got != (report.FileCounts{Total: 3, Covered: 1, Uncovered: 2}) {
		t.Fatalf("coverage without exclusions = %#v", got)
	}
	if !strings.Contains(unexcludedReviewer.Requests()[0].Prompt, "projects/"+filepath.Base(repo)+"/local") {
		t.Fatal("unexcluded prompt lost selected local quality")
	}
}

func assertExcludedFileFinding(t *testing.T, result CollectReviewResponse) {
	t.Helper()
	if result.Verdict != VerdictNotClean || result.Clean || len(result.Findings) != 1 || result.Findings[0].File != "docs/readme.md" || !result.Findings[0].Blocking {
		t.Fatalf("coverage exclusion suppressed finding or verdict: %#v", result)
	}
	if result.QualitiesSelected != 2 || len(result.ExcludedQualities) != 0 {
		t.Fatalf("coverage changed quality selection: %#v", result)
	}
}

func TestHistoricalResultWithoutCoverageStaysUnavailable(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	canonRoot := fixtureCanon(t, filepath.Base(repo))
	opts := Options{LogDestination: t.TempDir(), CanonPath: canonRoot, Reviewer: dummy.New(dummy.Options{Raw: json.RawMessage(`{"summary":"clean","findings":[]}`)}), ReviewerInfo: ReviewerInfo{Name: "dummy"}}
	live, err := New(opts).RunReview(context.Background(), StartReviewRequest{RepoPath: repo, ChangesetKind: changeset.KindFull})
	if err != nil {
		t.Fatal(err)
	}
	legacy := reviewResultFile{ReviewID: live.ReviewID, Project: live.Project, State: monitor.StateCompleted, Verdict: VerdictClean, Clean: true}
	resultPath := filepath.Join(filepath.Dir(live.LogPath), resultFileName)
	if err := writeJSONAtomic(resultPath, legacy); err != nil {
		t.Fatal(err)
	}
	got, err := New(opts).CollectReview(context.Background(), CollectReviewRequest{Project: live.Project, ReviewID: live.ReviewID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Coverage != nil {
		t.Fatalf("historical result gained coverage: %#v", got.Coverage)
	}
}

func TestReviewFailureRetainsCoverage(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	canonRoot := coverageFixtureCanon(t, filepath.Base(repo))
	b := New(Options{LogDestination: t.TempDir(), CanonPath: canonRoot, Reviewer: dummy.New(dummy.Options{Err: errors.New("boom")}), ReviewerInfo: ReviewerInfo{Name: "dummy"}})
	start, err := b.StartReview(context.Background(), StartReviewRequest{RepoPath: repo, ChangesetKind: changeset.KindFull})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		status, readErr := monitor.ReadStatus(start.StatusPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if status.State == monitor.StateFailed {
			if status.Coverage == nil || !status.Coverage.Assessed {
				t.Fatalf("failed status lost coverage: %#v", status.Coverage)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("review did not fail")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func coverageFixtureCanon(t *testing.T, project string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "general", "style.md"), "---\nid: style\n---\n# style\n")
	writeFile(t, filepath.Join(root, "projects", project, "local.md"), "---\nid: local\nterritory:\n  - internal/**\n---\n# local\n")
	writeFile(t, filepath.Join(root, "projects", project, "rubric.yaml"), fmt.Sprintf("project:\n  repo: %q\nqualities:\n  - ref: projects/%s/local\n    blocking: true\n  - ref: general/style\n    blocking: false\ncoverage_exclusions:\n  - docs/**\n", project, project))
	return root
}

func collectEventually(t *testing.T, b *Broker, req CollectReviewRequest) CollectReviewResponse {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err := b.CollectReview(context.Background(), req)
		if err == nil {
			return resp
		}
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeConflict || time.Now().After(deadline) {
			t.Fatalf("collect failed: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func fixtureCanon(t *testing.T, project string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go-conventions", "df-logging.md"), `---
id: df-logging
territory:
  - "**/*.go"
---
# df logging
`)
	writeFile(t, filepath.Join(root, "projects", project, "rubric.yaml"), fmt.Sprintf(`project:
  repo: %q
qualities:
  - ref: go-conventions/df-logging
    blocking: true
`, project))
	return root
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init")
	git(t, dir, "config", "user.email", "test@example.com")
	git(t, dir, "config", "user.name", "Test User")
	return dir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
