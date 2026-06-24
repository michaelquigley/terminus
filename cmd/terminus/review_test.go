package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michaelquigley/terminus/internal/changeset"
	"github.com/michaelquigley/terminus/internal/monitor"
)

func TestReviewRequestValidation(t *testing.T) {
	if _, err := reviewRequest(".", changeset.KindWorkingTree, []string{"internal"}); err == nil {
		t.Fatal("expected working-tree path args to fail")
	}
	if _, err := reviewRequest(".", changeset.KindPaths, nil); err == nil {
		t.Fatal("expected paths without path args to fail")
	}
	if _, err := reviewRequest(".", changeset.KindFull, []string{"internal"}); err == nil {
		t.Fatal("expected full with path args to fail")
	}
	if _, err := reviewRequest(".", "unknown", nil); err == nil {
		t.Fatal("expected unknown kind to fail")
	}
	req, err := reviewRequest("/tmp/repo", changeset.KindPaths, []string{"internal"})
	if err != nil {
		t.Fatal(err)
	}
	if req.RepoPath != "/tmp/repo" || req.ChangesetKind != changeset.KindPaths || len(req.Paths) != 1 || req.Paths[0] != "internal" {
		t.Fatalf("unexpected request: %#v", req)
	}
}

func TestReviewCommandRunsForeground(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")

	canonRoot := fixtureCanon(t, filepath.Base(repo))
	logDir := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "terminus.yaml")
	writeFile(t, configPath, fmt.Sprintf(`canon_path: %q
log_destination: %q
reviewer:
  name: dummy
  impl: dummy
`, canonRoot, logDir))

	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config", configPath, "review", "--repo", repo})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("review command failed: %v\n%s", err, out.String())
	}
	text := out.String()
	for _, want := range []string{"review '", "completed", "verdict: clean", "findings: 0 blocking, 0 advisory", "log:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q\n%s", want, text)
		}
	}

	projectDir := filepath.Join(logDir, filepath.Base(repo))
	runs, err := os.ReadDir(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one review run, got %d", len(runs))
	}
	runDir := filepath.Join(projectDir, runs[0].Name())
	for _, name := range []string{"status.json", "result.json", "_prompt.md", "_findings.md"} {
		if _, err := os.Stat(filepath.Join(runDir, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
}

func TestReviewCleanTreePromotesToFull(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	// no uncommitted changes: the working tree is clean

	configPath, logDir := promoteFixtureConfig(t, repo)

	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config", configPath, "review", "--repo", repo})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("review command failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "working tree clean; reviewing full tracked repo") {
		t.Fatalf("expected promotion notice in output:\n%s", out.String())
	}

	status := readSingleStatus(t, logDir, filepath.Base(repo))
	if status.ChangesetKind != changeset.KindFull {
		t.Fatalf("expected promoted changeset kind full, got %q", status.ChangesetKind)
	}
	if len(status.Qualities) == 0 {
		t.Fatal("expected promoted full review to select qualities, got none")
	}
}

func TestReviewExplicitWorkingTreeNotPromoted(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	// clean tree, but --kind is explicit and must be honored

	configPath, logDir := promoteFixtureConfig(t, repo)

	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config", configPath, "review", "--repo", repo, "--kind", "working-tree"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("review command failed: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "working tree clean; reviewing full tracked repo") {
		t.Fatalf("explicit --kind working-tree should not be promoted:\n%s", out.String())
	}

	status := readSingleStatus(t, logDir, filepath.Base(repo))
	if status.ChangesetKind != changeset.KindWorkingTree {
		t.Fatalf("expected changeset kind working-tree, got %q", status.ChangesetKind)
	}
}

func TestReviewWithNamedRubric(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() { _ = 1 }\n")

	canonRoot := fixtureCanon(t, filepath.Base(repo))
	writeFile(t, filepath.Join(canonRoot, "projects", filepath.Base(repo), "architecture.yaml"),
		fmt.Sprintf("project:\n  repo: %q\nqualities:\n  - ref: go-conventions/df-logging\n    blocking: false\n", filepath.Base(repo)))
	logDir := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "terminus.yaml")
	writeFile(t, configPath, fmt.Sprintf(`canon_path: %q
log_destination: %q
reviewer:
  name: dummy
  impl: dummy
`, canonRoot, logDir))

	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config", configPath, "review", "--repo", repo, "--rubric", "architecture"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("review command failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "rubric: architecture") {
		t.Fatalf("expected rubric in output:\n%s", out.String())
	}
	if status := readSingleStatus(t, logDir, filepath.Base(repo)); status.Rubric != "architecture" {
		t.Fatalf("expected status rubric architecture, got %q", status.Rubric)
	}
}

func TestRubricsCommandLists(t *testing.T) {
	repo := initGitRepo(t)
	canonRoot := fixtureCanon(t, filepath.Base(repo))
	writeFile(t, filepath.Join(canonRoot, "projects", filepath.Base(repo), "architecture.yaml"),
		fmt.Sprintf("project:\n  repo: %q\nqualities:\n  - ref: go-conventions/df-logging\n", filepath.Base(repo)))
	configPath := filepath.Join(t.TempDir(), "terminus.yaml")
	writeFile(t, configPath, fmt.Sprintf("canon_path: %q\nlog_destination: %q\nreviewer:\n  name: dummy\n  impl: dummy\n", canonRoot, t.TempDir()))

	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config", configPath, "rubrics", "--repo", repo})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("rubrics command failed: %v\n%s", err, out.String())
	}
	text := out.String()
	for _, want := range []string{"architecture", "rubric (default)"} {
		if !strings.Contains(text, want) {
			t.Fatalf("rubrics output missing %q:\n%s", want, text)
		}
	}
}

// promoteFixtureConfig writes a dummy-reviewer config and returns its path along
// with the log destination, so the status helper can locate the written run.
func promoteFixtureConfig(t *testing.T, repo string) (configPath string, logDir string) {
	t.Helper()
	canonRoot := fixtureCanon(t, filepath.Base(repo))
	logDir = t.TempDir()
	configPath = filepath.Join(t.TempDir(), "terminus.yaml")
	writeFile(t, configPath, fmt.Sprintf(`canon_path: %q
log_destination: %q
reviewer:
  name: dummy
  impl: dummy
`, canonRoot, logDir))
	return configPath, logDir
}

func readSingleStatus(t *testing.T, logDir string, project string) monitor.ReviewStatus {
	t.Helper()
	projectDir := filepath.Join(logDir, project)
	runs, err := os.ReadDir(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one review run, got %d", len(runs))
	}
	status, err := monitor.ReadStatus(monitor.StatusPath(filepath.Join(projectDir, runs[0].Name())))
	if err != nil {
		t.Fatal(err)
	}
	return status
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
