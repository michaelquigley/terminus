// Package coverage computes territory-coverage assessments: for a set of
// starting-point files, which files no project-local quality of the composed
// rubric reaches, which rubric exclusion patterns suppress, and — on request
// — which territory patterns are dead and which local qualities reach each
// file.
//
// review-time reporting and the full-tree audit both run this one
// calculation, so the two surfaces can never disagree about a file's
// coverage. the package owns plain domain types only: it performs no
// filesystem access and carries no JSON, YAML, MCP, or CLI knowledge. the
// report package converts Assessment to the wire shapes; CLI renderers
// consume report data and may group or format it but never recalculate
// coverage.
package coverage

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/michaelquigley/terminus/internal/canon"
)

// QualityRef identifies a quality by its head id and its canon-relative ref.
type QualityRef struct {
	ID  string
	Ref string
}

// FileCounts classifies the input files disjointly: a file matching any
// exclusion counts once as excluded, else a file reached by a local quality
// is covered, else it is uncovered. these are file counts, not a coverage
// score.
type FileCounts struct {
	Total     int
	Excluded  int
	Covered   int
	Uncovered int
}

// Exclusion pairs a declared exclusion pattern with all input files it
// matches, including files also reached by local qualities.
type Exclusion struct {
	Pattern string
	Files   []string
}

// FileDetail records, for one input file, the local qualities and exclusion
// patterns that reach it. files remain visible whether covered, uncovered,
// or excluded.
type FileDetail struct {
	File              string
	Qualities         []QualityRef
	ExclusionPatterns []string
}

// DeadPattern is a territory pattern that matches no input file. a pattern
// whose only matches fall in an excluded tree is not dead.
type DeadPattern struct {
	Quality QualityRef
	Pattern string
}

// Assessment is the result of one coverage calculation.
type Assessment struct {
	// Assessed is true for a rubric assessment, including the empty-input and
	// no-local-tier cases.
	Assessed bool
	// Reason explains a not-assessed result, such as an ad-hoc review. it is
	// empty when assessed.
	Reason     string
	FileCounts FileCounts
	// LocalQualities are all local qualities in the composed rubric, sorted
	// by ref. it is empty for a rubric with no local tier.
	LocalQualities []QualityRef
	// UncoveredFiles are the reportable uncovered paths, sorted: files no
	// local quality reaches and no exclusion suppresses.
	UncoveredFiles []string
	// Exclusions are the rubric's distinct declared exclusion patterns,
	// sorted, each with the input files it matches.
	Exclusions []Exclusion
	// FileDetails hold one entry per input file, sorted by path; set only
	// when Options.WithMap is set.
	FileDetails []FileDetail
	// DeadPatterns are sorted by ref then pattern; set only when
	// Options.WithDeadPatterns is set.
	DeadPatterns []DeadPattern
}

// Options requests the audit-only detail beyond the core assessment.
// review-time evaluation passes neither flag: a review must never call a
// pattern dead merely because it misses the changeset.
type Options struct {
	WithDeadPatterns bool
	WithMap          bool
}

// Assess computes how the project-local qualities in composed reach the
// normalized input files, with exclusions suppressing uncovered-file
// complaints. project is the resolved project identity; a quality is local
// when its canon ref sits beneath projects/<project>/. a local quality with
// no territory reaches every input file. general and foreign-project
// qualities never supply local matches, but their explicit territory
// patterns participate in dead-pattern detection.
//
// every quality territory and exclusion pattern is validated before the
// assessment is returned, even for an empty file set, and matcher errors
// propagate instead of being treated as absence of a match. a canceled
// context stops evaluation promptly.
func Assess(ctx context.Context, project string, composed []canon.Selected, files []string, exclusions []string, opts Options) (Assessment, error) {
	// check at entry as well as in the loops: with an empty file set the loop
	// checks below never run, and a canceled context must still stop the
	// calculation rather than return a success.
	if err := ctx.Err(); err != nil {
		return Assessment{}, err
	}
	for _, s := range composed {
		for _, territory := range s.Quality.Head.Territory {
			if err := ctx.Err(); err != nil {
				return Assessment{}, err
			}
			if err := canon.ValidateTerritory(territory); err != nil {
				return Assessment{}, fmt.Errorf("quality %q territory %q: %w", s.Quality.Ref, territory, err)
			}
		}
	}
	for _, pattern := range exclusions {
		if err := ctx.Err(); err != nil {
			return Assessment{}, err
		}
		if err := canon.ValidateTerritory(pattern); err != nil {
			return Assessment{}, fmt.Errorf("coverage exclusion %q: %w", pattern, err)
		}
	}

	paths := canon.NormalizeFiles(files)
	exclusionPatterns := distinctSorted(exclusions)
	prefix := "projects/" + project + "/"

	local := make([]canon.Selected, 0, len(composed))
	a := Assessment{
		Assessed:       true,
		LocalQualities: []QualityRef{},
		UncoveredFiles: []string{},
		Exclusions:     []Exclusion{},
	}
	seenLocal := map[string]bool{}
	for _, s := range composed {
		if !strings.HasPrefix(s.Quality.Ref, prefix) {
			continue
		}
		if seenLocal[s.Quality.Ref] {
			continue
		}
		seenLocal[s.Quality.Ref] = true
		a.LocalQualities = append(a.LocalQualities, QualityRef{ID: s.Quality.Head.ID, Ref: s.Quality.Ref})
		local = append(local, s)
	}
	sort.Slice(a.LocalQualities, func(i, j int) bool {
		return a.LocalQualities[i].Ref < a.LocalQualities[j].Ref
	})

	perFile := make([]fileInfo, len(paths))
	perPattern := map[string][]string{}
	for i, file := range paths {
		if err := ctx.Err(); err != nil {
			return Assessment{}, err
		}
		info := &perFile[i]
		for _, s := range local {
			matched, err := qualityMatchesFile(s, file)
			if err != nil {
				return Assessment{}, fmt.Errorf("quality %q: %w", s.Quality.Ref, err)
			}
			if !matched {
				continue
			}
			info.covered = true
			info.qualities = append(info.qualities, QualityRef{ID: s.Quality.Head.ID, Ref: s.Quality.Ref})
		}
		for _, pattern := range exclusionPatterns {
			matched, err := canon.MatchTerritory(pattern, file)
			if err != nil {
				return Assessment{}, fmt.Errorf("coverage exclusion %q: %w", pattern, err)
			}
			if !matched {
				continue
			}
			info.excluded = true
			info.exclusionPatterns = append(info.exclusionPatterns, pattern)
			perPattern[pattern] = append(perPattern[pattern], file)
		}
	}

	counts := FileCounts{Total: len(paths)}
	for i, info := range perFile {
		switch {
		case info.excluded:
			counts.Excluded++
		case info.covered:
			counts.Covered++
		default:
			counts.Uncovered++
			a.UncoveredFiles = append(a.UncoveredFiles, paths[i])
		}
	}
	a.FileCounts = counts

	for _, pattern := range exclusionPatterns {
		files := perPattern[pattern]
		if files == nil {
			files = []string{}
		}
		a.Exclusions = append(a.Exclusions, Exclusion{Pattern: pattern, Files: files})
	}

	if opts.WithMap {
		a.FileDetails = make([]FileDetail, 0, len(paths))
		for i, file := range paths {
			info := perFile[i]
			qualities := append([]QualityRef(nil), info.qualities...)
			sort.Slice(qualities, func(x, y int) bool { return qualities[x].Ref < qualities[y].Ref })
			if qualities == nil {
				qualities = []QualityRef{}
			}
			patterns := append([]string(nil), info.exclusionPatterns...)
			if patterns == nil {
				patterns = []string{}
			}
			a.FileDetails = append(a.FileDetails, FileDetail{File: file, Qualities: qualities, ExclusionPatterns: patterns})
		}
	}

	if opts.WithDeadPatterns {
		dead, err := deadPatterns(ctx, composed, paths)
		if err != nil {
			return Assessment{}, err
		}
		a.DeadPatterns = dead
	}

	return a, nil
}

// fileInfo is the per-file working state built while classifying inputs.
type fileInfo struct {
	covered           bool
	excluded          bool
	qualities         []QualityRef
	exclusionPatterns []string
}

// qualityMatchesFile reports whether a quality reaches one file: a quality
// with no territory always applies, else any of its territory patterns
// matching suffices.
func qualityMatchesFile(s canon.Selected, file string) (bool, error) {
	if len(s.Quality.Head.Territory) == 0 {
		return true, nil
	}
	for _, territory := range s.Quality.Head.Territory {
		matched, err := canon.MatchTerritory(territory, file)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

// deadPatterns tests every explicit territory pattern of the composed
// qualities — local, general, and foreign-project tiers alike — against all
// input files, collapsing duplicate ref/pattern pairs.
func deadPatterns(ctx context.Context, composed []canon.Selected, paths []string) ([]DeadPattern, error) {
	type key struct {
		ref     string
		id      string
		pattern string
	}
	seen := map[key]bool{}
	var out []DeadPattern
	for _, s := range composed {
		for _, pattern := range s.Quality.Head.Territory {
			live, err := patternMatchesAny(ctx, pattern, paths)
			if err != nil {
				return nil, fmt.Errorf("quality %q pattern %q: %w", s.Quality.Ref, pattern, err)
			}
			if live {
				continue
			}
			k := key{ref: s.Quality.Ref, id: s.Quality.Head.ID, pattern: pattern}
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, DeadPattern{Quality: QualityRef{ID: s.Quality.Head.ID, Ref: s.Quality.Ref}, Pattern: pattern})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Quality.Ref != out[j].Quality.Ref {
			return out[i].Quality.Ref < out[j].Quality.Ref
		}
		return out[i].Pattern < out[j].Pattern
	})
	if out == nil {
		out = []DeadPattern{}
	}
	return out, nil
}

func patternMatchesAny(ctx context.Context, pattern string, files []string) (bool, error) {
	// also at entry: with no input files the per-file loop below never runs,
	// so dead-pattern detection over an empty tree must still honor
	// cancellation.
	if err := ctx.Err(); err != nil {
		return false, err
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		matched, err := canon.MatchTerritory(pattern, file)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

func distinctSorted(patterns []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, p := range patterns {
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
