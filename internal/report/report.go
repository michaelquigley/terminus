// Package report defines the transport and persistence shapes for territory
// coverage and converts coverage domain results into them. the broker and the
// monitor both import this package without an import cycle: coverage carries
// no schema knowledge, report carries no calculation. one conversion defines
// the public coverage shape used by review status, results, collect, and
// audit. CLI renderers consume this data and may group or format it but never
// recalculate coverage.
//
// report also owns the shared dd codec. dd is the single binding substrate
// for every project payload — on-disk records, and the MCP tool inputs,
// results, and errors. project-owned structs carry no json tags; the raw
// reviewer JSON passes through unbound as its nested value so disk and MCP
// agree on it. schema knowledge stays out of this package; the explicit MCP
// schemas live in internal/mcpserver.
package report

import (
	"sort"
	"strings"

	"github.com/michaelquigley/terminus/internal/coverage"
)

// FileCounts classifies the input files disjointly: excluded first, then
// covered, then uncovered. present only on assessed output.
type FileCounts struct {
	Total     int
	Excluded  int
	Covered   int
	Uncovered int
}

// QualityRef identifies a quality by its head id and its canon-relative ref.
type QualityRef struct {
	ID  string
	Ref string
}

// Exclusion pairs a declared exclusion pattern with all input files it
// matches, including files also reached by local qualities.
type Exclusion struct {
	Pattern string
	Files   []string
}

// Coverage is the wire object carried by every new review status, completed
// result, collect response, and audit result.
//
// Assessed is true for a rubric assessment, including empty input and
// no-local-tier cases. Reason is omitted when assessed and carries ad_hoc
// otherwise. FileCounts is present only when assessed. array members are
// always arrays, never null: empty for not-assessed output, and empty for
// assessed output with nothing to report. a nil *Coverage on historical
// status and result DTOs means coverage information is unavailable; it must
// never be filled in from the current canon.
type Coverage struct {
	Assessed       bool
	Reason         string `dd:",+omitempty"`
	FileCounts     *FileCounts
	LocalQualities []QualityRef
	UncoveredFiles []string
	Exclusions     []Exclusion
}

// DeadPattern is a territory pattern that matches no file in the audited
// tree, attributed to the quality that declares it.
type DeadPattern struct {
	Quality QualityRef
	Pattern string
}

// MapFile is one file's coverage map entry: the local qualities and
// exclusion patterns that reach it.
type MapFile struct {
	File              string
	Qualities         []QualityRef
	ExclusionPatterns []string
}

// MapGroup groups coverage map entries by immediate parent directory, with
// "." for repo-root files. file-level differences within a directory are
// preserved.
type MapGroup struct {
	Directory string
	Files     []MapFile
}

// FileScopeFull identifies the audit's scope: the full tracked tree.
const FileScopeFull = "full"

// AuditResponse is the audit_coverage result: the assessed coverage of the
// project rubric over its full tracked tree, the dead territory patterns,
// and the optional per-file coverage map.
type AuditResponse struct {
	Project   string
	Rubric    string
	FileScope string
	Coverage  *Coverage
	// DeadPatterns are ordered by ref then declared pattern string.
	DeadPatterns []DeadPattern
	// CoverageMap is nil when the map was not requested and a pointer to an
	// initialized (possibly empty) slice when it was, so requested-empty
	// survives serialization as [] while omitted stays absent through dd.
	CoverageMap *[]MapGroup
}

// CoverageFrom converts a domain assessment to the wire coverage object. it
// initializes every array to an empty slice so serialization emits [] rather
// than null, and deep-copies nested lists so mutating the result cannot
// reach the assessment it came from.
func CoverageFrom(a coverage.Assessment) *Coverage {
	out := &Coverage{
		Assessed:       a.Assessed,
		Reason:         a.Reason,
		LocalQualities: []QualityRef{},
		UncoveredFiles: []string{},
		Exclusions:     []Exclusion{},
	}
	if !a.Assessed {
		return out
	}
	out.FileCounts = &FileCounts{
		Total:     a.FileCounts.Total,
		Excluded:  a.FileCounts.Excluded,
		Covered:   a.FileCounts.Covered,
		Uncovered: a.FileCounts.Uncovered,
	}
	out.LocalQualities = append(out.LocalQualities, qualityRefs(a.LocalQualities)...)
	out.UncoveredFiles = append(out.UncoveredFiles, a.UncoveredFiles...)
	for _, e := range a.Exclusions {
		out.Exclusions = append(out.Exclusions, Exclusion{
			Pattern: e.Pattern,
			Files:   append([]string(nil), e.Files...),
		})
	}
	return out
}

// AuditResponseFrom converts an assessment for project and rubric into the
// audit wire object. includeMap selects the per-file coverage map: a
// requested map is always present (possibly []), an unrequested one nil.
func AuditResponseFrom(project, rubric string, a coverage.Assessment, includeMap bool) *AuditResponse {
	out := &AuditResponse{
		Project:      project,
		Rubric:       rubric,
		FileScope:    FileScopeFull,
		Coverage:     CoverageFrom(a),
		DeadPatterns: []DeadPattern{},
	}
	for _, dp := range a.DeadPatterns {
		out.DeadPatterns = append(out.DeadPatterns, DeadPattern{
			Quality: QualityRef{ID: dp.Quality.ID, Ref: dp.Quality.Ref},
			Pattern: dp.Pattern,
		})
	}
	if includeMap {
		groups := groupMapFiles(a.FileDetails)
		out.CoverageMap = &groups
	}
	return out
}

// CloneCoverage deep-copies a wire coverage object, including the nested
// exclusion file lists and local-quality lists, so crossing a mutable result
// boundary cannot alter the source. a nil source stays nil.
func CloneCoverage(in *Coverage) *Coverage {
	if in == nil {
		return nil
	}
	out := &Coverage{
		Assessed:       in.Assessed,
		Reason:         in.Reason,
		LocalQualities: append([]QualityRef(nil), in.LocalQualities...),
		UncoveredFiles: append([]string(nil), in.UncoveredFiles...),
		Exclusions:     make([]Exclusion, 0, len(in.Exclusions)),
	}
	if in.FileCounts != nil {
		counts := *in.FileCounts
		out.FileCounts = &counts
	}
	for _, e := range in.Exclusions {
		out.Exclusions = append(out.Exclusions, Exclusion{
			Pattern: e.Pattern,
			Files:   append([]string(nil), e.Files...),
		})
	}
	return out
}

func groupMapFiles(details []coverage.FileDetail) []MapGroup {
	idx := map[string]int{}
	groups := []MapGroup{}
	for _, d := range details {
		mf := MapFile{
			File:              d.File,
			Qualities:         qualityRefs(d.Qualities),
			ExclusionPatterns: append([]string(nil), d.ExclusionPatterns...),
		}
		if mf.Qualities == nil {
			mf.Qualities = []QualityRef{}
		}
		if mf.ExclusionPatterns == nil {
			mf.ExclusionPatterns = []string{}
		}
		dir := "."
		if i := strings.LastIndex(d.File, "/"); i >= 0 {
			dir = d.File[:i]
		}
		if i, ok := idx[dir]; ok {
			groups[i].Files = append(groups[i].Files, mf)
			continue
		}
		idx[dir] = len(groups)
		groups = append(groups, MapGroup{Directory: dir, Files: []MapFile{mf}})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Directory < groups[j].Directory })
	return groups
}

func qualityRefs(in []coverage.QualityRef) []QualityRef {
	out := make([]QualityRef, 0, len(in))
	for _, q := range in {
		out = append(out, QualityRef{ID: q.ID, Ref: q.Ref})
	}
	return out
}
