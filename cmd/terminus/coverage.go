package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/michaelquigley/terminus/internal/broker"
	"github.com/michaelquigley/terminus/internal/report"
)

// printCoverageSummary renders the review's stored assessment without
// recalculating territory reach. missing historical data stays distinct from
// an explicit ad-hoc or assessed result.
func printCoverageSummary(out io.Writer, result broker.CollectReviewResponse) {
	fmt.Fprintln(out, coverageSummary(result.Coverage))
	coverage := result.Coverage
	if coverage == nil || !coverage.Assessed || coverage.FileCounts == nil || len(coverage.LocalQualities) == 0 {
		return
	}
	if coverage.FileCounts.Uncovered > 0 {
		for _, file := range coverage.UncoveredFiles {
			fmt.Fprintf(out, "  %s\n", file)
		}
	}
}

func coverageSummary(coverage *report.Coverage) string {
	if coverage == nil {
		return "coverage: unavailable"
	}
	if !coverage.Assessed {
		return fmt.Sprintf("coverage: not assessed — %s review", strings.ReplaceAll(coverage.Reason, "_", " "))
	}
	if coverage.FileCounts == nil {
		return "coverage: unavailable"
	}
	counts := coverage.FileCounts
	if len(coverage.LocalQualities) == 0 {
		line := "coverage: rubric contains no project-local qualities"
		switch {
		case counts.Total == 0:
			line += "; no starting-point files"
		case counts.Excluded == counts.Total:
			line += fmt.Sprintf("; all %d files excluded from gap reporting", counts.Total)
		default:
			word := "files"
			if counts.Uncovered == 1 {
				word = "file"
			}
			line += fmt.Sprintf("; %d %s uncovered", counts.Uncovered, word)
		}
		return line
	}
	switch {
	case counts.Total == 0:
		return "coverage: no starting-point files"
	case counts.Excluded == counts.Total:
		return fmt.Sprintf("coverage: all %d files excluded from gap reporting", counts.Total)
	case counts.Uncovered > 0:
		word := "files"
		if counts.Uncovered == 1 {
			word = "file"
		}
		return fmt.Sprintf("coverage: %d %s has no matching project-local quality", counts.Uncovered, word)
	default:
		return fmt.Sprintf("coverage: no uncovered files (%d excluded from gap reporting)", counts.Excluded)
	}
}
