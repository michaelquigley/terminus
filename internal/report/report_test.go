package report

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/terminus/internal/coverage"
)

func assessedFixture() coverage.Assessment {
	return coverage.Assessment{
		Assessed:       true,
		FileCounts:     coverage.FileCounts{Total: 3, Excluded: 1, Covered: 1, Uncovered: 1},
		LocalQualities: []coverage.QualityRef{{ID: "config-validation", Ref: "projects/example/config-validation"}},
		UncoveredFiles: []string{"internal/gateway/handler.go"},
		Exclusions: []coverage.Exclusion{
			{Pattern: "docs/**", Files: []string{"docs/guide.md"}},
		},
	}
}

func notAssessedFixture() coverage.Assessment {
	return coverage.Assessment{Assessed: false, Reason: "ad_hoc"}
}

func unmarshalKeys(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	return out
}

func ddJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := dd.UnbindJSON(v)
	if err != nil {
		t.Fatalf("dd.UnbindJSON: %v", err)
	}
	return raw
}

// assessed output: reason and file_counts are present/omitted correctly, every
// array is an array, and dd emits the snake_case wire keys. dd output is the
// wire form; there is no separate encoding/json form to agree with.
func TestAssessedCoverageSerialization(t *testing.T) {
	cov := CoverageFrom(assessedFixture())
	keys := unmarshalKeys(t, ddJSON(t, cov))
	if keys["assessed"] != true {
		t.Fatalf("assessed = %v, want true", keys["assessed"])
	}
	if _, ok := keys["reason"]; ok {
		t.Fatalf("reason must be omitted when assessed: %s", ddJSON(t, cov))
	}
	for _, key := range []string{"file_counts", "local_qualities", "uncovered_files", "exclusions"} {
		if _, ok := keys[key]; !ok {
			t.Fatalf("dd output missing key %q: %s", key, ddJSON(t, cov))
		}
	}

	// empty arrays serialize as [] in dd, never null.
	empty := CoverageFrom(coverage.Assessment{Assessed: true, FileCounts: coverage.FileCounts{}})
	ek := unmarshalKeys(t, ddJSON(t, empty))
	for _, field := range []string{"local_qualities", "uncovered_files", "exclusions"} {
		if v, ok := ek[field].([]any); !ok || len(v) != 0 {
			t.Fatalf("%s = %#v, want empty array", field, ek[field])
		}
	}
}

// not-assessed output: explicit false assessment with the ad_hoc reason, no
// counts, and empty arrays in the dd wire form.
func TestNotAssessedCoverageSerialization(t *testing.T) {
	cov := CoverageFrom(notAssessedFixture())
	keys := unmarshalKeys(t, ddJSON(t, cov))
	if keys["assessed"] != false {
		t.Fatalf("assessed = %v, want explicit false", keys["assessed"])
	}
	if keys["reason"] != "ad_hoc" {
		t.Fatalf("reason = %v, want ad_hoc", keys["reason"])
	}
	if _, ok := keys["file_counts"]; ok {
		t.Fatalf("file_counts must be absent when not assessed: %s", ddJSON(t, cov))
	}
	for _, field := range []string{"local_qualities", "uncovered_files", "exclusions"} {
		if v, ok := keys[field].([]any); !ok || len(v) != 0 {
			t.Fatalf("%s = %#v, want empty array", field, keys[field])
		}
	}
}

// a requested-empty coverage map serializes as [] while an unrequested one is
// absent from the dd output. a nil pointer is never emitted as null.
func TestAuditMapPresenceSerialization(t *testing.T) {
	resp := AuditResponseFrom("example", "rubric", assessedFixture(), false)
	if resp.CoverageMap != nil {
		t.Fatal("unrequested map must be nil")
	}
	keys := unmarshalKeys(t, ddJSON(t, resp))
	if _, ok := keys["coverage_map"]; ok {
		t.Fatalf("unrequested map must be absent from dd: %s", ddJSON(t, resp))
	}

	empty := AuditResponseFrom("example", "rubric", coverage.Assessment{}, true)
	if empty.CoverageMap == nil || len(*empty.CoverageMap) != 0 {
		t.Fatalf("requested-empty map = %#v, want initialized empty slice", empty.CoverageMap)
	}
	dk := unmarshalKeys(t, ddJSON(t, empty))
	if v, ok := dk["coverage_map"].([]any); !ok || len(v) != 0 {
		t.Fatalf("requested-empty map dd = %#v, want []", dk["coverage_map"])
	}
}

// directory grouping keeps per-file differences within a directory and
// groups repo-root files under ".", in stable order.
func TestGroupMapFiles(t *testing.T) {
	details := []coverage.FileDetail{
		{File: "a/c/x.go", Qualities: []coverage.QualityRef{{ID: "a", Ref: "projects/p/a"}}, ExclusionPatterns: []string{}},
		{File: "a/y.go", Qualities: []coverage.QualityRef{{ID: "a", Ref: "projects/p/a"}, {ID: "b", Ref: "projects/p/b"}}, ExclusionPatterns: []string{}},
		{File: "ab/z.go", Qualities: []coverage.QualityRef{{ID: "b", Ref: "projects/p/b"}}, ExclusionPatterns: []string{}},
		{File: "root.go", Qualities: nil, ExclusionPatterns: nil},
	}
	groups := groupMapFiles(details)
	if len(groups) != 4 {
		t.Fatalf("groups = %#v", groups)
	}
	wantDirs := []string{".", "a", "a/c", "ab"}
	for i, dir := range wantDirs {
		if groups[i].Directory != dir {
			t.Fatalf("group[%d] = %q, want %q", i, groups[i].Directory, dir)
		}
	}
	aGroup := groups[1]
	if len(aGroup.Files) != 1 || aGroup.Files[0].File != "a/y.go" {
		t.Fatalf("a group = %#v", aGroup.Files)
	}
	if !reflect.DeepEqual(aGroup.Files[0].Qualities, []QualityRef{{ID: "a", Ref: "projects/p/a"}, {ID: "b", Ref: "projects/p/b"}}) {
		t.Fatalf("per-file qualities lost: %#v", aGroup.Files[0].Qualities)
	}
	rootGroup := groups[0]
	if len(rootGroup.Files) != 1 || rootGroup.Files[0].File != "root.go" {
		t.Fatalf("root group = %#v", rootGroup.Files)
	}
	if rootGroup.Files[0].Qualities == nil || rootGroup.Files[0].ExclusionPatterns == nil {
		t.Fatal("empty map members must be initialized arrays, not nil")
	}
}

// CloneCoverage copies every nested list: mutating the clone must not reach
// the source.
func TestCloneCoverageIsDeep(t *testing.T) {
	src := CoverageFrom(assessedFixture())
	clone := CloneCoverage(src)
	if !reflect.DeepEqual(clone, src) {
		t.Fatalf("clone = %#v, want copy of %#v", clone, src)
	}
	clone.UncoveredFiles = append(clone.UncoveredFiles, "injected.go")
	clone.Exclusions[0].Files = append(clone.Exclusions[0].Files, "injected.md")
	clone.LocalQualities = append(clone.LocalQualities, QualityRef{ID: "x", Ref: "projects/example/x"})
	if len(src.UncoveredFiles) != 1 || len(src.Exclusions[0].Files) != 1 || len(src.LocalQualities) != 1 {
		t.Fatal("mutating the clone altered the source")
	}
	if CloneCoverage(nil) != nil {
		t.Fatal("nil must clone to nil")
	}
}
