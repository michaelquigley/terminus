package monitor

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/terminus/internal/errs"
	"github.com/michaelquigley/terminus/internal/report"
)

// guards the dd migration of status.json: a fully-populated ReviewStatus must
// survive WriteStatus -> ReadStatus unchanged, including the nested reviewer
// info, error detail, file list, selected qualities, and excluded qualities.
func TestStatusRoundTrip(t *testing.T) {
	path := StatusPath(t.TempDir())
	in := ReviewStatus{
		ReviewID:          "abc123",
		Project:           "demo",
		Rubric:            "rubric",
		State:             StateCompleted,
		ChangesetKind:     "full",
		Reviewer:          ReviewerInfo{Name: "pi", Impl: "pi", Model: "m"},
		StartedAt:         "2026-06-30T00:00:00Z",
		UpdatedAt:         "2026-06-30T00:00:05Z",
		CompletedAt:       "2026-06-30T00:00:05Z",
		StatusPath:        path,
		LogPath:           "log.md",
		Error:             &errs.Info{Code: "user_error", Message: "bad input", Details: map[string]any{"key": "value"}, At: "2026-06-30T00:00:01Z"},
		Files:             []string{"main.go", "internal/x.go"},
		Qualities:         []QualityInfo{{ID: "df-binding", Ref: "go-conventions/df-binding", Blocking: true}},
		ExcludedQualities: []QualityInfo{{ID: "cmd-only", Ref: "go-conventions/cmd-only", Blocking: true}},
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

// a status carrying a coverage object round-trips through WriteStatus ->
// ReadStatus, in both the dd on-disk form and the JSON wire form.
func TestStatusCoverageRoundTrip(t *testing.T) {
	cov := &report.Coverage{
		Assessed:       true,
		FileCounts:     &report.FileCounts{Total: 2, Excluded: 1, Covered: 1},
		LocalQualities: []report.QualityRef{{ID: "df-logging", Ref: "projects/demo/df-logging"}},
		UncoveredFiles: []string{"internal/gateway/handler.go"},
		Exclusions:     []report.Exclusion{{Pattern: "docs/**", Files: []string{"docs/guide.md"}}},
	}
	path := StatusPath(t.TempDir())
	in := ReviewStatus{ReviewID: "abc123", Project: "demo", State: StateRunning, StartedAt: "2026-09-08T00:00:00Z", UpdatedAt: "2026-09-08T00:00:00Z", StatusPath: path, Coverage: cov}
	if err := WriteStatus(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadStatus(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in.Coverage, out.Coverage) {
		t.Fatalf("coverage drifted through status:\n in:  %#v\n out: %#v", in.Coverage, out.Coverage)
	}

	raw, err := dd.UnbindJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]any
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	if _, ok := keys["coverage"]; !ok {
		t.Fatalf("dd status output missing coverage: %s", raw)
	}
}

// historical statuses without a coverage key bind to a nil pointer and re-emit
// without one, so unavailable coverage is never presented as an assessed
// empty gap list.
func TestStatusNilCoverageStaysUnavailable(t *testing.T) {
	path := StatusPath(t.TempDir())
	in := ReviewStatus{ReviewID: "old1", Project: "demo", State: StateCompleted, StartedAt: "2026-08-01T00:00:00Z", UpdatedAt: "2026-08-01T00:00:00Z", StatusPath: path}
	if err := WriteStatus(path, in); err != nil {
		t.Fatal(err)
	}
	raw, err := dd.UnbindJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]any
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	if _, ok := keys["coverage"]; ok {
		t.Fatalf("nil coverage must be absent from dd output: %s", raw)
	}
	out, err := ReadStatus(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.Coverage != nil {
		t.Fatalf("historical status must bind to nil coverage, got %#v", out.Coverage)
	}
	// and re-emission keeps it absent.
	if err := WriteStatus(path, out); err != nil {
		t.Fatal(err)
	}
	reread, err := ReadStatus(path)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Coverage != nil {
		t.Fatalf("re-emitted status must stay without coverage, got %#v", reread.Coverage)
	}
}
