package mcpserver

import (
	"github.com/google/jsonschema-go/jsonschema"
)

// explicit transport schemas for the MCP tools. these are declarations that
// must agree with the dd output of the project DTOs; they drive input
// validation, success validation, and discovery. they are not a second binder
// — binding stays entirely in dd.
//
// the building blocks are functions, not shared variables: the resolver
// requires each schema to form a tree, and a node shared across two property
// positions is not a tree. calling a constructor yields a fresh node, so a
// root that embeds several blocks (audit embeds coverage and dead patterns,
// both of which embed quality refs) stays a valid tree. the per-tool schemas
// below are built once from those constructors.

func strSchema() *jsonschema.Schema { return &jsonschema.Schema{Type: "string"} }

func boolSchema() *jsonschema.Schema { return &jsonschema.Schema{Type: "boolean"} }

// anySchema matches any JSON value; used for the raw reviewer output, which is
// opaque project data we never reinterpret.
func anySchema() *jsonschema.Schema { return &jsonschema.Schema{} }

func nonNegInt() *jsonschema.Schema {
	min := float64(0)
	return &jsonschema.Schema{Type: "integer", Minimum: &min}
}

// objSchema builds an object schema that disallows unknown properties (the
// same posture dd.Strict binding enforces) and names the required members.
func objSchema(props map[string]*jsonschema.Schema, required ...string) *jsonschema.Schema {
	s := &jsonschema.Schema{
		Type:                 "object",
		Properties:           props,
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
	if len(required) > 0 {
		s.Required = required
	}
	return s
}

func arraySchema(items *jsonschema.Schema) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "array", Items: items}
}

// shared building blocks; the coverage block is the single definition used by
// review status/result/collect and by audit.

func qualityRefSchema() *jsonschema.Schema {
	return objSchema(
		map[string]*jsonschema.Schema{"id": strSchema(), "ref": strSchema()},
		"id", "ref",
	)
}

func fileCountsSchema() *jsonschema.Schema {
	return objSchema(
		map[string]*jsonschema.Schema{
			"total":     nonNegInt(),
			"excluded":  nonNegInt(),
			"covered":   nonNegInt(),
			"uncovered": nonNegInt(),
		},
		"total", "excluded", "covered", "uncovered",
	)
}

func exclusionSchema() *jsonschema.Schema {
	return objSchema(
		map[string]*jsonschema.Schema{"pattern": strSchema(), "files": arraySchema(strSchema())},
		"pattern", "files",
	)
}

func coverageSchema() *jsonschema.Schema {
	return objSchema(
		map[string]*jsonschema.Schema{
			"assessed":        boolSchema(),
			"reason":          strSchema(),
			"file_counts":     fileCountsSchema(),
			"local_qualities": arraySchema(qualityRefSchema()),
			"uncovered_files": arraySchema(strSchema()),
			"exclusions":      arraySchema(exclusionSchema()),
		},
		"assessed", "local_qualities", "uncovered_files", "exclusions",
	)
}

func deadPatternSchema() *jsonschema.Schema {
	return objSchema(
		map[string]*jsonschema.Schema{"quality": qualityRefSchema(), "pattern": strSchema()},
		"quality", "pattern",
	)
}

func mapFileSchema() *jsonschema.Schema {
	return objSchema(
		map[string]*jsonschema.Schema{
			"file":               strSchema(),
			"qualities":          arraySchema(qualityRefSchema()),
			"exclusion_patterns": arraySchema(strSchema()),
		},
		"file", "qualities", "exclusion_patterns",
	)
}

func mapGroupSchema() *jsonschema.Schema {
	return objSchema(
		map[string]*jsonschema.Schema{"directory": strSchema(), "files": arraySchema(mapFileSchema())},
		"directory", "files",
	)
}

func errorSchema() *jsonschema.Schema {
	return objSchema(
		map[string]*jsonschema.Schema{
			"code":    strSchema(),
			"message": strSchema(),
			"details": &jsonschema.Schema{Type: "object"},
			"at":      strSchema(),
		},
		"code", "message", "details",
	)
}

func toolErrorSchema() *jsonschema.Schema {
	return objSchema(map[string]*jsonschema.Schema{"error": errorSchema()}, "error")
}

func excludedQualitySchema() *jsonschema.Schema {
	return objSchema(
		map[string]*jsonschema.Schema{"id": strSchema(), "ref": strSchema(), "blocking": boolSchema()},
		"id", "ref", "blocking",
	)
}

func triageFindingSchema() *jsonschema.Schema {
	return objSchema(
		map[string]*jsonschema.Schema{
			"id":         strSchema(),
			"quality":    strSchema(),
			"file":       strSchema(),
			"lines":      strSchema(),
			"claim":      strSchema(),
			"rationale":  strSchema(),
			"suggestion": strSchema(),
			"blocking":   boolSchema(),
		},
		"id", "quality", "file", "lines", "claim", "rationale", "blocking",
	)
}

func reviewSummarySchema() *jsonschema.Schema {
	return objSchema(
		map[string]*jsonschema.Schema{
			"review_id":      strSchema(),
			"project":        strSchema(),
			"rubric":         strSchema(),
			"state":          strSchema(),
			"changeset_kind": strSchema(),
			"started_at":     strSchema(),
			"completed_at":   strSchema(),
			"status_path":    strSchema(),
			"log_path":       strSchema(),
		},
		"review_id", "project", "state", "changeset_kind", "started_at", "status_path",
	)
}

func collectResponseSchema() *jsonschema.Schema {
	return objSchema(
		map[string]*jsonschema.Schema{
			"review_id":          strSchema(),
			"project":            strSchema(),
			"rubric":             strSchema(),
			"qualities_selected": nonNegInt(),
			"excluded_qualities": arraySchema(excludedQualitySchema()),
			"state":              strSchema(),
			"verdict":            strSchema(),
			"clean":              boolSchema(),
			"summary":            strSchema(),
			"log_path":           strSchema(),
			"prompt_path":        strSchema(),
			"reviewer_name":      strSchema(),
			"raw":                anySchema(),
			"findings":           arraySchema(triageFindingSchema()),
			"next_finding":       triageFindingSchema(),
			"guidance":           strSchema(),
			"coverage":           coverageSchema(),
		},
		"review_id", "project", "qualities_selected", "state", "verdict", "clean",
		"summary", "log_path", "prompt_path", "reviewer_name", "findings", "guidance",
	)
}

// per-tool input and output schemas, each built once from the shared
// constructors so its tree is internally consistent.

var startReviewInputSchema = objSchema(
	map[string]*jsonschema.Schema{
		"repo_path": strSchema(),
		"changeset_kind": &jsonschema.Schema{
			Type:        "string",
			Description: "working-tree, paths, or full; defaults to working-tree. paths mode requires paths.",
		},
		"paths":              arraySchema(strSchema()),
		"rubric":             stringDesc("named rubric to select qualities from; defaults to the project's `rubric`."),
		"qualities":          arraySchema(strSchema()),
		"qualities_blocking": boolDesc("treat qualities entries as blocking (advisory by default); ad-hoc reviews only."),
	},
)

var startReviewOutputSchema = objSchema(
	map[string]*jsonschema.Schema{
		"review_id":       strSchema(),
		"project":         strSchema(),
		"state":           strSchema(),
		"reviewer":        strSchema(),
		"started_at":      strSchema(),
		"status_path":     strSchema(),
		"monitor_command": strSchema(),
		"next_action":     strSchema(),
	},
	"review_id", "project", "state", "reviewer", "started_at", "status_path", "monitor_command", "next_action",
)

var collectReviewInputSchema = objSchema(
	map[string]*jsonschema.Schema{
		"project":   stringDesc("project name; when omitted the review is located by review_id alone."),
		"review_id": stringDesc("review id to collect; when omitted, known review runs are listed instead."),
	},
)

var collectReviewOutputSchema = objSchema(
	map[string]*jsonschema.Schema{
		"reviews": arraySchema(reviewSummarySchema()),
		"review":  collectResponseSchema(),
	},
)

// audit schemas are shared: stage 1 registers a test-only tool over them to pin
// the requested-empty versus omitted coverage_map behavior, and stage 3
// registers the real audit_coverage tool using these same definitions.

var auditInputSchema = objSchema(
	map[string]*jsonschema.Schema{
		"repo_path":   strSchema(),
		"rubric":      stringDesc("named rubric to audit; defaults to the project's `rubric`."),
		"include_map": boolDesc("default false. when true also returns coverage_map: which project-local qualities reach each file, grouped by directory while preserving per-file differences."),
	},
	"repo_path",
)

var auditResponseSchema = objSchema(
	map[string]*jsonschema.Schema{
		"project":       strSchema(),
		"rubric":        strSchema(),
		"file_scope":    strSchema(),
		"coverage":      coverageSchema(),
		"dead_patterns": arraySchema(deadPatternSchema()),
		"coverage_map":  arraySchema(mapGroupSchema()),
	},
	"project", "rubric", "file_scope", "coverage", "dead_patterns",
)

func stringDesc(desc string) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Description: desc}
}

func boolDesc(desc string) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "boolean", Description: desc}
}
