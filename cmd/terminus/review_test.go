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
