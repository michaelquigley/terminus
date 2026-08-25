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
	"testing"
	"time"

	"github.com/michaelquigley/terminus/internal/changeset"
	"github.com/michaelquigley/terminus/internal/errs"
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
