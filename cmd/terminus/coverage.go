package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/michaelquigley/terminus/internal/broker"
)

// printCoverageSummary renders the review's stored assessment without
// recalculating territory reach. missing historical data stays distinct from
// an explicit ad-hoc or assessed result.
func printCoverageSummary(out io.Writer, result broker.CollectReviewResponse) {
	coverage := result.Coverage
	if coverage == nil {
		fmt.Fprintln(out, "coverage: unavailable")
		return
	}
	if !coverage.Assessed {
		fmt.Fprintf(out, "coverage: not assessed — %s review\n", strings.ReplaceAll(coverage.Reason, "_", " "))
		return
	}
	if coverage.FileCounts == nil {
		fmt.Fprintln(out, "coverage: unavailable")
		return
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
			line += fmt.Sprintf("; %d files uncovered", counts.Uncovered)
		}
		fmt.Fprintln(out, line)
		return
	}
	switch {
	case counts.Total == 0:
		fmt.Fprintln(out, "coverage: no starting-point files")
	case counts.Excluded == counts.Total:
		fmt.Fprintf(out, "coverage: all %d files excluded from gap reporting\n", counts.Total)
	case counts.Uncovered > 0:
		word := "files"
		if counts.Uncovered == 1 {
			word = "file"
		}
		fmt.Fprintf(out, "coverage: %d %s has no matching project-local quality\n", counts.Uncovered, word)
		for _, file := range coverage.UncoveredFiles {
			fmt.Fprintf(out, "  %s\n", file)
		}
	default:
		fmt.Fprintf(out, "coverage: no uncovered files (%d excluded from gap reporting)\n", counts.Excluded)
	}
}
