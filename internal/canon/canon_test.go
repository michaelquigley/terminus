package canon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseQualityStrictHead(t *testing.T) {
	_, err := ParseQuality([]byte(`---
id: df-logging
teritory:
  - "**/*.go"
---
# body
`))
	if err == nil || !strings.Contains(err.Error(), "teritory") {
		t.Fatalf("expected unknown key error naming teritory, got %v", err)
	}
}

func TestComposeAndNarrow(t *testing.T) {
	root := fixtureCanon(t)
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	rubric, err := LoadRubric(store, "sample")
	if err != nil {
		t.Fatal(err)
	}
	composed, err := Compose(store, rubric)
	if err != nil {
		t.Fatal(err)
	}

	selected := Narrow(composed, []string{"main.go"})
	assertIDs(t, selected, "df-logging", "project-wide")

	selected = Narrow(composed, []string{"internal/foo/bar.go"})
	assertIDs(t, selected, "df-logging", "project-wide")

	selected = Narrow(composed, []string{"cmd/main.go"})
	assertIDs(t, selected, "df-logging", "cli-quality", "project-wide")
}

func TestComposeDuplicateIDFails(t *testing.T) {
	root := fixtureCanon(t)
	writeFile(t, filepath.Join(root, "go-conventions", "dupe.md"), `---
id: df-logging
territory:
  - "**/*.go"
---
# duplicate
`)
	writeFile(t, filepath.Join(root, "projects", "sample", "rubric.yaml"), `project:
  repo: sample
qualities:
  - ref: go-conventions/df-logging
  - ref: go-conventions/dupe
`)
	store, _ := NewStore(root)
	rubric, err := LoadRubric(store, "sample")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Compose(store, rubric)
	if err == nil || !strings.Contains(err.Error(), "duplicate quality id") {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
}

func TestCleanRefRejectsEscapes(t *testing.T) {
	for _, ref := range []string{"../x", "/tmp/x", "a/../../b"} {
		if _, err := CleanRef(ref); err == nil {
			t.Fatalf("expected ref %q to be rejected", ref)
		}
	}
}

func TestLoadProjectRubricValidatesRepoIdentity(t *testing.T) {
	root := fixtureCanon(t)
	writeFile(t, filepath.Join(root, "projects", "sample", "rubric.yaml"), `project:
  repo: other
qualities:
  - ref: go-conventions/df-logging
`)
	store, _ := NewStore(root)
	_, _, err := LoadProjectRubric(store, filepath.Join(t.TempDir(), "sample"))
	if err == nil || !strings.Contains(err.Error(), "project.repo mismatch") {
		t.Fatalf("expected project.repo mismatch, got %v", err)
	}
}

func TestValidateTerritoryRejectsMalformedGlob(t *testing.T) {
	err := ValidateTerritory("[bad")
	if err == nil {
		t.Fatal("expected malformed glob error")
	}
	err = ValidateTerritory("foo**bar")
	if err == nil || !strings.Contains(err.Error(), "**") {
		t.Fatalf("expected malformed ** error, got %v", err)
	}
}

func fixtureCanon(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go-conventions", "df-logging.md"), `---
id: df-logging
territory:
  - "**/*.go"
---
# df logging
`)
	writeFile(t, filepath.Join(root, "go-conventions", "cli-quality.md"), `---
id: cli-quality
territory:
  - "cmd/"
---
# cli
`)
	writeFile(t, filepath.Join(root, "go-conventions", "project-wide.md"), `---
id: project-wide
---
# project wide
`)
	writeFile(t, filepath.Join(root, "projects", "sample", "rubric.yaml"), `project:
  name: sample
  repo: sample
qualities:
  - ref: go-conventions/df-logging
    blocking: true
  - ref: go-conventions/cli-quality
  - ref: go-conventions/project-wide
`)
	return root
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

func assertIDs(t *testing.T, selected []Selected, ids ...string) {
	t.Helper()
	if len(selected) != len(ids) {
		t.Fatalf("expected %d selected, got %d: %#v", len(ids), len(selected), selected)
	}
	for i, id := range ids {
		if selected[i].Quality.Head.ID != id {
			t.Fatalf("selected[%d] = %q, expected %q", i, selected[i].Quality.Head.ID, id)
		}
	}
}
