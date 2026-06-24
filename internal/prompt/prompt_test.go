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
		// rewritten instruction sections
		"## How to Review",
		"## Output Constraints",
		"discrimination",
		"Prefer fewer, more material findings",
		"is unique within this review",
		"never the empty string",
		// entry-points / unfenced framing
		"## Starting Points",
		"the review is not limited to them",
		"wherever the problem actually lives",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("prompt did not contain %q\n%s", want, text)
		}
	}

	for _, absent := range []string{
		// the convention provenance field was dropped; the prompt must not render it.
		"- convention:",
		// findings are no longer fenced to the changeset.
		"file findings only against the files under review",
		"Every `file` is a file under review",
	} {
		if strings.Contains(text, absent) {
			t.Fatalf("prompt still contains stale text %q\n%s", absent, text)
		}
	}
}
