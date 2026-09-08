package mcpserver

import (
	"context"

	"github.com/michaelquigley/push/build"
	"github.com/michaelquigley/terminus/internal/broker"
	"github.com/michaelquigley/terminus/internal/report"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StartReviewInput is the dd-bound input for start_review. no json tags: dd's
// snake-case default (or an explicit dd name) owns the field mapping, and the
// explicit input schema owns discovery and validation.
type StartReviewInput struct {
	RepoPath          string
	ChangesetKind     string
	Paths             []string
	Rubric            string
	Qualities         []string
	QualitiesBlocking bool
}

type StartReviewOutput struct {
	ReviewID       string
	Project        string
	State          string
	Reviewer       string
	StartedAt      string
	StatusPath     string
	MonitorCommand string
	NextAction     string
}

type CollectReviewInput struct {
	Project  string
	ReviewID string
}

type AuditCoverageInput struct {
	RepoPath   string
	Rubric     string
	IncludeMap bool
}

type CollectReviewOutput struct {
	// Reviews is the list branch (collect with no review_id); Review is the
	// single-review branch. each is omitted when its branch is not the one
	// taken, so consumers can tell the branches apart by presence.
	Reviews []broker.ReviewSummary `dd:",+omitempty"`
	Review  *broker.CollectReviewResponse
}

// ToolErrorOutput is the dd-bound error envelope returned in a tool result's
// structured content. its shape is {error: {code, message, details, at?}}.
type ToolErrorOutput struct {
	Error ErrorOutput
}

type ErrorOutput struct {
	Code    string
	Message string
	Details map[string]any
	At      string `dd:",+omitempty"`
}

// New wraps an already-constructed broker in an MCP server. the broker is built
// by the command layer (see wiring.NewBroker); this adapter stays out of
// composition and only registers the transport. a schema-construction failure
// during registration is returned rather than registered half-built.
func New(b *broker.Broker) (*mcp.Server, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: "terminus", Version: build.String()}, nil)
	if err := RegisterTools(server, b); err != nil {
		return nil, err
	}
	return server, nil
}

func RegisterTools(server *mcp.Server, b *broker.Broker) error {
	destructive := false
	if err := register(server, tool[StartReviewInput, StartReviewOutput]{
		name:        "start_review",
		description: "start one Terminus code review in the background. repo_path is required and drives project resolution from the canon. changeset_kind is working-tree, paths, or full; paths mode requires paths. rubric is the named rubric to select qualities from (defaults to the project's `rubric`); the available rubric names come from the canon's projects/<project>/ directory. rubric reviews report territory coverage for their starting-point files in status, result, and collect; coverage gaps do not change the findings verdict. use audit_coverage to investigate full-tree gaps and optionally request its per-file map. qualities is an optional list of canon quality refs to review against directly, bypassing the rubric (an ad-hoc review); when set it takes precedence over rubric, and qualities_blocking makes them blocking (they are advisory by default). use the returned monitor_command while the review runs, then call collect_review with review_id.",
		input:       startReviewInputSchema,
		output:      startReviewOutputSchema,
		run: func(ctx context.Context, in StartReviewInput) (StartReviewOutput, error) {
			response, err := b.StartReview(ctx, broker.StartReviewRequest{
				RepoPath:          in.RepoPath,
				ChangesetKind:     in.ChangesetKind,
				Paths:             append([]string(nil), in.Paths...),
				Rubric:            in.Rubric,
				Qualities:         append([]string(nil), in.Qualities...),
				QualitiesBlocking: in.QualitiesBlocking,
			})
			if err != nil {
				return StartReviewOutput{}, err
			}
			return StartReviewOutput{
				ReviewID:       response.ReviewID,
				Project:        response.Project,
				State:          response.State,
				Reviewer:       response.Reviewer,
				StartedAt:      response.StartedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
				StatusPath:     response.StatusPath,
				MonitorCommand: response.MonitorCommand,
				NextAction:     response.NextAction,
			}, nil
		},
	}); err != nil {
		return err
	}

	if err := register(server, tool[CollectReviewInput, CollectReviewOutput]{
		name:        "collect_review",
		description: "collect a completed Terminus review, or list known reviews when review_id is omitted. if a review is still running this returns a conflict error; monitor instead of retrying immediately. findings are triage ordered with blocking findings first. rubric reviews include territory coverage for their starting-point files; coverage gaps can coexist with a clean findings verdict, while ad-hoc coverage is explicitly not assessed. use audit_coverage to investigate full-tree gaps and optionally request its per-file map.",
		input:       collectReviewInputSchema,
		output:      collectReviewOutputSchema,
		run: func(ctx context.Context, in CollectReviewInput) (CollectReviewOutput, error) {
			if in.ReviewID == "" {
				response, err := b.ListReviews(ctx, in.Project)
				if err != nil {
					return CollectReviewOutput{}, err
				}
				return CollectReviewOutput{Reviews: response.Reviews}, nil
			}
			response, err := b.CollectReview(ctx, broker.CollectReviewRequest{
				Project:  in.Project,
				ReviewID: in.ReviewID,
			})
			if err != nil {
				return CollectReviewOutput{}, err
			}
			return CollectReviewOutput{Review: &response}, nil
		},
	}); err != nil {
		return err
	}

	if err := register(server, tool[AuditCoverageInput, report.AuditResponse]{
		name:        "audit_coverage",
		description: "audit one project rubric against its full tracked tree. repo_path is required; rubric defaults to `rubric`. returns uncovered files after coverage exclusions and territory patterns that match no tracked file. set include_map to true (default false) to also return a coverage map showing which project-local qualities reach each file, grouped by directory while preserving file-level differences. use the map to inspect uneven coverage even where files already match a quality. read-only: runs no reviewer, writes no review record, and makes no canon edits. use the evidence to propose canon changes, then rerun the audit to check their effect.",
		annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &destructive},
		input:       auditInputSchema,
		output:      auditResponseSchema,
		run: func(ctx context.Context, in AuditCoverageInput) (report.AuditResponse, error) {
			return b.AuditCoverage(ctx, broker.AuditCoverageRequest{RepoPath: in.RepoPath, Rubric: in.Rubric, IncludeMap: in.IncludeMap})
		},
	}); err != nil {
		return err
	}
	return nil
}
