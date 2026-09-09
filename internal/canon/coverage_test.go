package canon

import (
	"strings"
	"testing"
)

// the old validator matched one dummy path, so an early segment that failed
// to match hid invalid syntax in later segments. validation must visit every
// normalized segment.
func TestValidateTerritoryVisitsAllSegments(t *testing.T) {
	for _, pattern := range []string{
		"**",
		"*",
		"?.go",
		"*.go",
		"internal/**",
		"internal/**/x",
		"**/x",
		"docs/",
		"projects/sample/rubric.yaml",
	} {
		if err := ValidateTerritory(pattern); err != nil {
			t.Fatalf("ValidateTerritory(%q) = %v, want valid", pattern, err)
		}
	}
	for _, pattern := range []string{
		"",
		"a**b",
		"internal/a**b",
		"zzz/[",      // earlier segment cannot match; late syntax error must still fail
		"internal/[", // work-order case: late unterminated character class
		"internal/**/[",
		"[bad",
	} {
		err := ValidateTerritory(pattern)
		if err == nil {
			t.Fatalf("ValidateTerritory(%q) = nil, want syntax error", pattern)
		}
	}
}

// ** may only occupy an entire segment, and the error must name the rule.
func TestValidateTerritoryWholeSegmentOnly(t *testing.T) {
	err := ValidateTerritory("foo**bar")
	if err == nil || !strings.Contains(err.Error(), "**") {
		t.Fatalf("expected ** whole-segment error, got %v", err)
	}
}

func TestParseRubricCoverageExclusions(t *testing.T) {
	r, err := ParseRubric([]byte(`project:
  repo: sample
qualities:
  - ref: go-conventions/df-logging
coverage_exclusions:
  - docs/**
  - demo/**
`))
	if err != nil {
		t.Fatalf("parse rubric with exclusions: %v", err)
	}
	if len(r.CoverageExclusions) != 2 || r.CoverageExclusions[0] != "docs/**" || r.CoverageExclusions[1] != "demo/**" {
		t.Fatalf("declared exclusion spelling not preserved: %#v", r.CoverageExclusions)
	}
}

// an invalid exclusion reports the rubric field index and the offending
// pattern, whether the syntax error sits in an early or a late segment.
func TestParseRubricRejectsInvalidExclusions(t *testing.T) {
	for _, pattern := range []string{`internal/[`, `zzz/[`, `a**b`, ``} {
		raw := "project:\n  repo: sample\nqualities:\n  - ref: go-conventions/df-logging\ncoverage_exclusions:\n  - "
		if pattern == "" {
			// a bare dash parses as a YAML null, which dd rejects; quote the
			// empty string explicitly to reach the pattern validator.
			raw += "''"
		} else {
			raw += pattern
		}
		raw += "\n"
		_, err := ParseRubric([]byte(raw))
		if err == nil {
			t.Fatalf("exclusion %q accepted, want validation error", pattern)
		}
		if !strings.Contains(err.Error(), "coverage_exclusions[0]") {
			t.Fatalf("error does not name the field index: %v", err)
		}
		if pattern != "" && !strings.Contains(err.Error(), pattern) {
			t.Fatalf("error does not name the offending pattern: %v", err)
		}
	}
}

// unknown rubric fields stay rejected; the unsupported `coverage` key in
// particular must not be confused with coverage_exclusions.
func TestParseRubricStillRejectsUnknownFields(t *testing.T) {
	_, err := ParseRubric([]byte(`project:
  repo: sample
qualities:
  - ref: go-conventions/df-logging
coverage:
  - docs/**
`))
	if err == nil || !strings.Contains(err.Error(), "coverage") {
		t.Fatalf("expected unknown key error for coverage, got %v", err)
	}
}
