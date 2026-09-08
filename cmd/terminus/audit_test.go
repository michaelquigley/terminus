package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditCoverageCommandIsReadOnly(t *testing.T) {
	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	canonRoot := fixtureCanon(t, filepath.Base(repo))
	blockedLog := filepath.Join(t.TempDir(), "not-a-directory")
	writeFile(t, blockedLog, "untouched\n")
	configPath := filepath.Join(t.TempDir(), "terminus.yaml")
	writeFile(t, configPath, fmt.Sprintf("canon_path: %q\nlog_destination: %q\nreviewer:\n  name: impossible\n  impl: pi\n  binary_path: %q\n", canonRoot, blockedLog, filepath.Join(t.TempDir(), "does-not-exist")))

	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config", configPath, "audit-coverage", "--repo", repo, "--rubric", "rubric.yaml", "--include-map"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("audit command failed: %v\n%s", err, out.String())
	}
	for _, want := range []string{"project: " + filepath.Base(repo), "rubric: rubric", "scope: full", "coverage: rubric contains no project-local qualities; 1 file uncovered", "dead patterns:", "coverage map:", "main.go"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("audit output missing %q:\n%s", want, out.String())
		}
	}
	if raw, err := os.ReadFile(blockedLog); err != nil || string(raw) != "untouched\n" {
		t.Fatalf("audit touched log destination: raw=%q err=%v", raw, err)
	}
}

func TestAuditCoverageCommandRejectsArguments(t *testing.T) {
	cmd := newRootCommand()
	cmd.SetArgs([]string{"audit-coverage", "unexpected"})
	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("audit-coverage must reject positional arguments")
	}
}
