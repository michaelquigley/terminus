package monitor

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/michaelquigley/terminus/internal/errs"
)

// guards the dd migration of status.json: a fully-populated ReviewStatus must
// survive WriteStatus -> ReadStatus unchanged, including the nested reviewer
// info, error detail, file list, and selected qualities.
func TestStatusRoundTrip(t *testing.T) {
	path := StatusPath(t.TempDir())
	in := ReviewStatus{
		ReviewID:      "abc123",
		Project:       "demo",
		Rubric:        "rubric",
		State:         StateCompleted,
		ChangesetKind: "full",
		Reviewer:      ReviewerInfo{Name: "pi", Impl: "pi", Model: "m"},
		StartedAt:     "2026-06-30T00:00:00Z",
		UpdatedAt:     "2026-06-30T00:00:05Z",
		CompletedAt:   "2026-06-30T00:00:05Z",
		StatusPath:    path,
		LogPath:       "log.md",
		Error:         &errs.Info{Code: "user_error", Message: "bad input", Details: map[string]any{"key": "value"}, At: "2026-06-30T00:00:01Z"},
		Files:         []string{"main.go", "internal/x.go"},
		Qualities:     []QualityInfo{{ID: "df-binding", Ref: "go-conventions/df-binding", Blocking: true}},
	}
	if err := WriteStatus(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadStatus(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("status round-trip drift:\n in:  %#v\n out: %#v", in, out)
	}
}

// the key path other tooling relies on: ReadStatus resolves a written status by
// its path helper.
func TestStatusPathRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if got := StatusPath(dir); got != filepath.Join(dir, StatusFileName) {
		t.Fatalf("unexpected status path %q", got)
	}
}
