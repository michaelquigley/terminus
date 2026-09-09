package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/michaelquigley/terminus/internal/broker"
	"github.com/michaelquigley/terminus/internal/report"
)

func TestPrintCoverageSummaryNoLocalEmptyScope(t *testing.T) {
	var out bytes.Buffer
	printCoverageSummary(&out, broker.CollectReviewResponse{Coverage: &report.Coverage{
		Assessed:       true,
		FileCounts:     &report.FileCounts{},
		LocalQualities: []report.QualityRef{},
	}})
	if got, want := strings.TrimSpace(out.String()), "coverage: rubric contains no project-local qualities; no starting-point files"; got != want {
		t.Fatalf("coverage summary = %q, want %q", got, want)
	}
}

func TestPrintCoverageSummaryStates(t *testing.T) {
	tests := []struct {
		name string
		in   *report.Coverage
		want string
	}{
		{name: "unavailable", in: nil, want: "coverage: unavailable"},
		{name: "ad hoc", in: &report.Coverage{Reason: "ad_hoc"}, want: "coverage: not assessed — ad hoc review"},
		{name: "gap free", in: &report.Coverage{Assessed: true, FileCounts: &report.FileCounts{Total: 2, Covered: 2}, LocalQualities: []report.QualityRef{{Ref: "projects/p/local"}}}, want: "coverage: no uncovered files (0 excluded from gap reporting)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			printCoverageSummary(&out, broker.CollectReviewResponse{Coverage: test.in})
			if got := strings.TrimSpace(out.String()); got != test.want {
				t.Fatalf("coverage summary = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPrintCoverageSummaryOrdinaryGap(t *testing.T) {
	var out bytes.Buffer
	printCoverageSummary(&out, broker.CollectReviewResponse{Coverage: &report.Coverage{
		Assessed:       true,
		FileCounts:     &report.FileCounts{Total: 2, Covered: 1, Uncovered: 1},
		LocalQualities: []report.QualityRef{{Ref: "projects/p/local"}},
		UncoveredFiles: []string{"internal/gap.go"},
	}})
	want := "coverage: 1 file has no matching project-local quality\n  internal/gap.go"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Fatalf("coverage summary = %q, want %q", got, want)
	}
}

func TestPrintCoverageSummaryLocalAllExcluded(t *testing.T) {
	var out bytes.Buffer
	printCoverageSummary(&out, broker.CollectReviewResponse{Coverage: &report.Coverage{
		Assessed:       true,
		FileCounts:     &report.FileCounts{Total: 2, Excluded: 2},
		LocalQualities: []report.QualityRef{{Ref: "projects/p/local"}},
	}})
	want := "coverage: all 2 files excluded from gap reporting"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Fatalf("coverage summary = %q, want %q", got, want)
	}
}

func TestPrintCoverageSummaryNoLocalAllExcluded(t *testing.T) {
	var out bytes.Buffer
	printCoverageSummary(&out, broker.CollectReviewResponse{Coverage: &report.Coverage{
		Assessed:       true,
		FileCounts:     &report.FileCounts{Total: 2, Excluded: 2},
		LocalQualities: []report.QualityRef{},
	}})
	if got, want := strings.TrimSpace(out.String()), "coverage: rubric contains no project-local qualities; all 2 files excluded from gap reporting"; got != want {
		t.Fatalf("coverage summary = %q, want %q", got, want)
	}
}
