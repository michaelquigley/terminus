package changeset

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestWorkingTreeIncludesModifiedDeletedAndUntracked(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n")
	writeFile(t, filepath.Join(repo, "old.go"), "package main\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")

	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")
	if err := os.Remove(filepath.Join(repo, "old.go")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "new.go"), "package main\n")

	cs, err := WorkingTree(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"main.go", "old.go", "new.go"} {
		if !slices.Contains(cs.Files, file) {
			t.Fatalf("expected %s in files: %#v", file, cs.Files)
		}
	}
	if !strings.Contains(cs.Diff, "main.go") || !strings.Contains(cs.Diff, "old.go") {
		t.Fatalf("expected diff to include tracked changes:\n%s", cs.Diff)
	}
}

func TestPathsAndFull(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "cmd", "main.go"), "package main\n")
	writeFile(t, filepath.Join(repo, "internal", "x.go"), "package internal\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")

	paths, err := Paths(context.Background(), repo, []string{"cmd/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths.Files) != 1 || paths.Files[0] != "cmd/main.go" {
		t.Fatalf("unexpected paths files: %#v", paths.Files)
	}

	full, err := Full(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"cmd/main.go", "internal/x.go"} {
		if !slices.Contains(full.Files, file) {
			t.Fatalf("expected %s in full files: %#v", file, full.Files)
		}
	}
}

func TestGitStagingMatrix(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*testing.T, string)
		workingWant []string
		fullWant    []string
	}{
		{name: "untracked add", mutate: func(t *testing.T, repo string) { writeFile(t, filepath.Join(repo, "new.go"), "package main\n") }, workingWant: []string{"new.go"}, fullWant: []string{"old.go"}},
		{name: "staged add", mutate: func(t *testing.T, repo string) {
			writeFile(t, filepath.Join(repo, "new.go"), "package main\n")
			git(t, repo, "add", "new.go")
		}, workingWant: []string{"new.go"}, fullWant: []string{"new.go", "old.go"}},
		{name: "unstaged delete", mutate: func(t *testing.T, repo string) {
			if err := os.Remove(filepath.Join(repo, "old.go")); err != nil {
				t.Fatal(err)
			}
		}, workingWant: []string{"old.go"}, fullWant: []string{"old.go"}},
		{name: "staged delete", mutate: func(t *testing.T, repo string) { git(t, repo, "rm", "old.go") }, workingWant: []string{"old.go"}, fullWant: []string{}},
		{name: "unstaged rename", mutate: func(t *testing.T, repo string) {
			if err := os.Rename(filepath.Join(repo, "old.go"), filepath.Join(repo, "new.go")); err != nil {
				t.Fatal(err)
			}
		}, workingWant: []string{"new.go", "old.go"}, fullWant: []string{"old.go"}},
		{name: "staged rename", mutate: func(t *testing.T, repo string) { git(t, repo, "mv", "old.go", "new.go") }, workingWant: []string{"new.go"}, fullWant: []string{"new.go"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := initGitRepo(t)
			writeFile(t, filepath.Join(repo, "old.go"), "package main\n")
			git(t, repo, "add", ".")
			git(t, repo, "commit", "-m", "initial")
			test.mutate(t, repo)
			working, err := WorkingTree(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			full, err := Full(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			slices.Sort(working.Files)
			slices.Sort(full.Files)
			if !slices.Equal(working.Files, test.workingWant) || !slices.Equal(full.Files, test.fullWant) {
				t.Fatalf("working=%#v full=%#v; want working=%#v full=%#v", working.Files, full.Files, test.workingWant, test.fullWant)
			}
		})
	}
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
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false"}, args...)...)
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
