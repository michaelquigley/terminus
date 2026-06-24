package prompt

import (
	"strings"
	"testing"

	"github.com/michaelquigley/terminus/internal/canon"
	"github.com/michaelquigley/terminus/internal/changeset"
)

func TestBuildContainsSelectedQualityAndSchema(t *testing.T) {
	text := Build(Request{
		RepoPath: "/tmp/repo",
		Selected: []canon.Selected{{
			Quality: canon.Quality{
				Head: canon.Head{ID: "df-logging"},
				Ref:  "go-conventions/df-logging",
				Body: "# df logging\n\nuse df/dl",
			},
			Blocking: true,
		}},
		Changeset: changeset.Changeset{
			Kind:  changeset.KindWorkingTree,
			Files: []string{"main.go"},
			Diff:  "diff --git a/main.go b/main.go",
		},
	})
	for _, want := range []string{
		"df-logging",
		"go-conventions/df-logging",
		"blocking: `true`",
		"diff --git",
		"Respond with a single JSON object only",
		"quality",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("prompt did not contain %q\n%s", want, text)
		}
	}
}
