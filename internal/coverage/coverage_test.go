package coverage

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/michaelquigley/terminus/internal/canon"
)

func sel(id, ref string, territories ...string) canon.Selected {
	return canon.Selected{Quality: canon.Quality{Head: canon.Head{ID: id, Territory: territories}, Ref: ref}}
}

func mustAssess(t *testing.T, project string, composed []canon.Selected, files []string, exclusions []string, opts Options) Assessment {
	t.Helper()
	a, err := Assess(context.Background(), project, composed, files, exclusions, opts)
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	return a
}

// foreign and general qualities never supply local matches, whatever their
// territories reach.
func TestProjectLocalPrefixMembership(t *testing.T) {
	composed := []canon.Selected{
		sel("local-a", "projects/sample/local-a", "internal/a.go"),
		sel("foreign", "projects/other/reaches-b", "internal/b.go"),
		sel("general", "general/reaches-all", "internal/**"),
	}
	a := mustAssess(t, "sample", composed, []string{"internal/a.go", "internal/b.go"}, nil, Options{})
	if !a.Assessed {
		t.Fatal("expected assessed")
	}
	if got := a.FileCounts; got != (FileCounts{Total: 2, Covered: 1, Uncovered: 1}) {
		t.Fatalf("counts = %+v", got)
	}
	if !reflect.DeepEqual(a.LocalQualities, []QualityRef{{ID: "local-a", Ref: "projects/sample/local-a"}}) {
		t.Fatalf("local qualities = %#v", a.LocalQualities)
	}
	if !reflect.DeepEqual(a.UncoveredFiles, []string{"internal/b.go"}) {
		t.Fatalf("uncovered = %#v (foreign and general reaches must not count)", a.UncoveredFiles)
	}
}

// a local quality with no territory reaches every input file and eliminates
// gap reports for that rubric.
func TestTerritoryFreeLocalQualityCoversAll(t *testing.T) {
	composed := []canon.Selected{
		sel("project-wide", "projects/sample/project-wide"),
		sel("scoped", "projects/sample/scoped", "cmd/**"),
	}
	a := mustAssess(t, "sample", composed, []string{"internal/a.go", "cmd/x.go", "docs/note.md"}, nil, Options{})
	if a.FileCounts != (FileCounts{Total: 3, Covered: 3}) {
		t.Fatalf("counts = %+v", a.FileCounts)
	}
	if len(a.UncoveredFiles) != 0 {
		t.Fatalf("uncovered = %#v", a.UncoveredFiles)
	}
}

func TestMixedCoveredUncovered(t *testing.T) {
	composed := []canon.Selected{sel("cmd-scope", "projects/sample/cmd-scope", "cmd/**")}
	a := mustAssess(t, "sample", composed, []string{"cmd/main.go", "internal/y.go"}, nil, Options{})
	if a.FileCounts != (FileCounts{Total: 2, Covered: 1, Uncovered: 1}) {
		t.Fatalf("counts = %+v", a.FileCounts)
	}
	if !reflect.DeepEqual(a.UncoveredFiles, []string{"internal/y.go"}) {
		t.Fatalf("uncovered = %#v", a.UncoveredFiles)
	}
}

// an exclusion suppresses the complaint even when a local quality reaches
// the file, and the map keeps the actual match.
func TestExclusionOverlapsCoverage(t *testing.T) {
	composed := []canon.Selected{sel("docs-scope", "projects/sample/docs-scope", "**/*.md")}
	a := mustAssess(t, "sample", composed,
		[]string{"docs/a.md", "internal/b.go"}, []string{"docs/**"}, Options{WithMap: true})
	// docs/a.md is both excluded and covered: the exclusion wins the count
	// classification; b.go matches neither the local quality (.md only) nor
	// an exclusion, so it stays uncovered.
	if a.FileCounts != (FileCounts{Total: 2, Excluded: 1, Uncovered: 1}) {
		t.Fatalf("counts = %+v", a.FileCounts)
	}
	if !reflect.DeepEqual(a.UncoveredFiles, []string{"internal/b.go"}) {
		t.Fatalf("uncovered = %#v", a.UncoveredFiles)
	}
	detail := a.FileDetails[0] // docs/a.md sorts first
	if !reflect.DeepEqual(detail.Qualities, []QualityRef{{ID: "docs-scope", Ref: "projects/sample/docs-scope"}}) {
		t.Fatalf("excluded file must keep its quality match in the map, got %#v", detail.Qualities)
	}
	if !reflect.DeepEqual(detail.ExclusionPatterns, []string{"docs/**"}) {
		t.Fatalf("exclusion patterns = %#v", detail.ExclusionPatterns)
	}
}

// overlapping exclusions count a file once, and duplicate declared strings
// collapse to one diagnostic entry.
func TestExclusionsOverlapEachOther(t *testing.T) {
	composed := []canon.Selected{sel("none", "projects/sample/none", "zzz/**")}
	a := mustAssess(t, "sample", composed,
		[]string{"docs/a.md", "demo/x.go", "src/y.go"},
		[]string{"docs/**", "docs/a.md", "docs/**"}, Options{})
	if a.FileCounts != (FileCounts{Total: 3, Excluded: 1, Uncovered: 2}) {
		t.Fatalf("counts = %+v", a.FileCounts)
	}
	if len(a.Exclusions) != 2 {
		t.Fatalf("expected distinct patterns only, got %#v", a.Exclusions)
	}
	if !reflect.DeepEqual(a.Exclusions[0].Files, []string{"docs/a.md"}) || a.Exclusions[0].Pattern != "docs/**" {
		t.Fatalf("exclusion entries = %#v", a.Exclusions)
	}
	if !reflect.DeepEqual(a.UncoveredFiles, []string{"demo/x.go", "src/y.go"}) {
		t.Fatalf("uncovered = %#v", a.UncoveredFiles)
	}
}

func TestEmptyScope(t *testing.T) {
	composed := []canon.Selected{sel("a", "projects/sample/a", "internal/**")}
	a := mustAssess(t, "sample", composed, nil, []string{"docs/**"}, Options{})
	if a.FileCounts != (FileCounts{}) {
		t.Fatalf("empty scope counts = %+v", a.FileCounts)
	}
	if len(a.Exclusions) != 1 || a.Exclusions[0].Pattern != "docs/**" || len(a.Exclusions[0].Files) != 0 {
		t.Fatalf("pattern with no matches must be kept with empty files, got %#v", a.Exclusions)
	}
}

func TestAllFilesExcluded(t *testing.T) {
	composed := []canon.Selected{sel("a", "projects/sample/a", "docs/**")}
	a := mustAssess(t, "sample", composed,
		[]string{"docs/a.md", "docs/b.md"}, []string{"docs/**"}, Options{})
	if a.FileCounts != (FileCounts{Total: 2, Excluded: 2}) {
		t.Fatalf("counts = %+v (all-excluded must differ from empty scope)", a.FileCounts)
	}
	if len(a.UncoveredFiles) != 0 {
		t.Fatalf("uncovered = %#v", a.UncoveredFiles)
	}
}

// a rubric with no project-local tier still gets an assessment retaining
// every uncovered, non-excluded path.
func TestNoLocalTier(t *testing.T) {
	composed := []canon.Selected{sel("general", "general/x", "**")}
	a := mustAssess(t, "sample", composed,
		[]string{"a.go", "docs/b.md"}, []string{"docs/**"}, Options{})
	if len(a.LocalQualities) != 0 {
		t.Fatalf("local tier = %#v", a.LocalQualities)
	}
	if a.FileCounts != (FileCounts{Total: 2, Excluded: 1, Uncovered: 1}) {
		t.Fatalf("counts = %+v", a.FileCounts)
	}
	if !reflect.DeepEqual(a.UncoveredFiles, []string{"a.go"}) {
		t.Fatalf("uncovered = %#v", a.UncoveredFiles)
	}
}

// a matcher error propagates instead of reading as no match, and validation
// runs even for an empty file set.
func TestMalformedLateSegmentPropagates(t *testing.T) {
	composed := []canon.Selected{sel("a", "projects/sample/a", "zzz/[")}
	if _, err := Assess(context.Background(), "sample", composed, nil, nil, Options{}); err == nil {
		t.Fatal("expected matcher validation error for empty file set")
	}
	if _, err := Assess(context.Background(), "sample", composed, []string{"a.go"}, nil, Options{}); err == nil {
		t.Fatal("expected matcher validation error")
	}
	// a malformed exclusion fails too.
	if _, err := Assess(context.Background(), "sample", nil, []string{"a.go"}, []string{"internal/["}, Options{}); err == nil {
		t.Fatal("expected exclusion validation error")
	}
}

// dead patterns span every tier, a pattern whose only matches sit in an
// excluded tree is not dead, and duplicate ref/pattern pairs collapse.
func TestDeadPatternsAcrossTiers(t *testing.T) {
	composed := []canon.Selected{
		sel("local-live", "projects/sample/local-live", "internal/**"),
		sel("local-excluded-only", "projects/sample/local-excluded-only", "docs/**"),
		sel("local-dead", "projects/sample/local-dead", "gone/**", "also/gone/**"),
		sel("general-dead", "general/old", "legacy/**"),
		sel("foreign-dead", "projects/other/old", "stale/**"),
		sel("territory-free", "projects/sample/wide"),
	}
	files := []string{"internal/a.go", "docs/b.md"}
	a := mustAssess(t, "sample", composed, files, []string{"docs/**"}, Options{WithDeadPatterns: true})
	want := []DeadPattern{
		{Quality: QualityRef{ID: "general-dead", Ref: "general/old"}, Pattern: "legacy/**"},
		{Quality: QualityRef{ID: "foreign-dead", Ref: "projects/other/old"}, Pattern: "stale/**"},
		{Quality: QualityRef{ID: "local-dead", Ref: "projects/sample/local-dead"}, Pattern: "also/gone/**"},
		{Quality: QualityRef{ID: "local-dead", Ref: "projects/sample/local-dead"}, Pattern: "gone/**"},
	}
	if !reflect.DeepEqual(a.DeadPatterns, want) {
		t.Fatalf("dead patterns =\n%#v\nwant\n%#v", a.DeadPatterns, want)
	}
}

// requesting the map changes nothing about the assessment or dead patterns,
// and per-file detail keeps differing matches within one directory.
func TestMapDoesNotChangeAssessment(t *testing.T) {
	composed := []canon.Selected{
		sel("a", "projects/sample/a", "pkg/alpha.go"),
		sel("both", "projects/sample/both", "pkg/**"),
	}
	files := []string{"pkg/alpha.go", "pkg/beta.go"}
	bare := mustAssess(t, "sample", composed, files, nil, Options{WithDeadPatterns: true})
	withMap := mustAssess(t, "sample", composed, files, nil, Options{WithDeadPatterns: true, WithMap: true})
	details := withMap.FileDetails

	bare.FileDetails = nil
	withMap.FileDetails = nil
	if !reflect.DeepEqual(bare, withMap) {
		t.Fatalf("map request changed the assessment:\n%#v\n%#v", bare, withMap)
	}

	if len(details) != 2 {
		t.Fatalf("file details = %#v", details)
	}
	alpha := details[0]
	if !reflect.DeepEqual(alpha.Qualities, []QualityRef{
		{ID: "a", Ref: "projects/sample/a"},
		{ID: "both", Ref: "projects/sample/both"},
	}) {
		t.Fatalf("alpha qualities = %#v", alpha.Qualities)
	}
	beta := details[1]
	if !reflect.DeepEqual(beta.Qualities, []QualityRef{{ID: "both", Ref: "projects/sample/both"}}) {
		t.Fatalf("beta qualities = %#v (per-file differences must survive)", beta.Qualities)
	}
}

func TestCanceledContextStopsEvaluation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	files := make([]string, 100)
	for i := range files {
		files[i] = "internal/f" + string(rune('a'+i%26)) + ".go"
	}
	if _, err := Assess(ctx, "sample", nil, files, nil, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// regression for the empty-input hole: with no files the per-file loops never
// run, so cancellation must be caught at entry, in the validation loops, and
// in dead-pattern detection over an empty tree alike.
func TestCanceledContextEmptyInput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// entry: nothing to validate or match, yet a canceled context must still
	// stop the calculation.
	if _, err := Assess(ctx, "sample", nil, nil, nil, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("empty everything: err = %v, want context.Canceled", err)
	}
	// validation loop: territories are validated before any file loop.
	composed := []canon.Selected{
		sel("a", "projects/sample/a", "internal/**"),
		sel("g", "general/g", "**"),
	}
	if _, err := Assess(ctx, "sample", composed, nil, nil, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("empty files, composed rubric: err = %v, want context.Canceled", err)
	}
	if _, err := Assess(ctx, "sample", nil, nil, []string{"docs/**"}, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("empty files, exclusions: err = %v, want context.Canceled", err)
	}
	// dead-pattern detection over an empty tree: the per-file loop inside
	// patternMatchesAny never runs, so its entry check must catch it.
	if _, err := Assess(ctx, "sample", composed, nil, nil, Options{WithDeadPatterns: true}); !errors.Is(err, context.Canceled) {
		t.Fatalf("dead patterns, empty tree: err = %v, want context.Canceled", err)
	}
}
