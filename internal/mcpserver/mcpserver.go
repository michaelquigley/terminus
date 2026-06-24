package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/michaelquigley/push/build"
	"github.com/michaelquigley/terminus/internal/broker"
	"github.com/michaelquigley/terminus/internal/errs"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type StartReviewInput struct {
	RepoPath      string   `json:"repo_path,omitempty"`
	ChangesetKind string   `json:"changeset_kind,omitempty"`
	Paths         []string `json:"paths,omitempty"`
	Rubric        string   `json:"rubric,omitempty"`
}

type StartReviewOutput struct {
	ReviewID       string `json:"review_id"`
	Project        string `json:"project"`
	State          string `json:"state"`
	Reviewer       string `json:"reviewer"`
	StartedAt      string `json:"started_at"`
	StatusPath     string `json:"status_path"`
	MonitorCommand string `json:"monitor_command"`
	NextAction     string `json:"next_action"`
}

type CollectReviewInput struct {
	Project  string `json:"project,omitempty"`
	ReviewID string `json:"review_id,omitempty"`
}

type CollectReviewOutput struct {
	Reviews []broker.ReviewSummary        `json:"reviews,omitempty"`
	Review  *broker.CollectReviewResponse `json:"review,omitempty"`
}

type ToolErrorOutput struct {
	Error ErrorOutput `json:"error"`
}

type ErrorOutput struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
	At      string         `json:"at,omitempty"`
}

// New wraps an already-constructed broker in an MCP server. The broker is built
// by the command layer (see wiring.NewBroker); this adapter stays out of
// composition and only registers the transport.
func New(b *broker.Broker) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "terminus", Version: build.String()}, nil)
	RegisterTools(server, b)
	return server
}

func RegisterTools(server *mcp.Server, b *broker.Broker) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "start_review",
		Description: "start one Terminus code review in the background. repo_path is required and drives project resolution from the canon. changeset_kind is working-tree, paths, or full; paths mode requires paths. rubric is the named rubric to select qualities from (defaults to the project's `rubric`); the available rubric names come from the canon's projects/<project>/ directory. use the returned monitor_command while the review runs, then call collect_review with review_id.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input StartReviewInput) (*mcp.CallToolResult, any, error) {
		response, err := b.StartReview(ctx, broker.StartReviewRequest{
			RepoPath:      input.RepoPath,
			ChangesetKind: input.ChangesetKind,
			Paths:         append([]string(nil), input.Paths...),
			Rubric:        input.Rubric,
		})
		if err != nil {
			return toolErrorResult(err)
		}
		return nil, StartReviewOutput{
			ReviewID:       response.ReviewID,
			Project:        response.Project,
			State:          response.State,
			Reviewer:       response.Reviewer,
			StartedAt:      response.StartedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			StatusPath:     response.StatusPath,
			MonitorCommand: response.MonitorCommand,
			NextAction:     response.NextAction,
		}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "collect_review",
		Description: "collect a completed Terminus review, or list known reviews when review_id is omitted. if a review is still running this returns a conflict error; monitor instead of retrying immediately. findings are triage ordered with blocking findings first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input CollectReviewInput) (*mcp.CallToolResult, any, error) {
		if input.ReviewID == "" {
			response, err := b.ListReviews(ctx, input.Project)
			if err != nil {
				return toolErrorResult(err)
			}
			return nil, CollectReviewOutput{Reviews: response.Reviews}, nil
		}
		response, err := b.CollectReview(ctx, broker.CollectReviewRequest{
			Project:  input.Project,
			ReviewID: input.ReviewID,
		})
		if err != nil {
			return toolErrorResult(err)
		}
		return nil, CollectReviewOutput{Review: &response}, nil
	})
}

func toolErrorResult(err error) (*mcp.CallToolResult, any, error) {
	var e *errs.Error
	if !errors.As(err, &e) {
		return nil, nil, rpcError(err)
	}
	output := errorOutput(errs.InfoFrom(e))
	text := fmt.Sprintf("%s: %s", output.Code, output.Message)
	if cause, ok := output.Details["cause"].(string); ok && cause != "" {
		text += "\ncause: " + cause
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
		StructuredContent: ToolErrorOutput{Error: output},
		IsError:           true,
	}, nil, nil
}

func rpcError(err error) error {
	var e *errs.Error
	if errors.As(err, &e) {
		var code int64 = jsonrpc.CodeInvalidParams
		if e.Code == errs.CodeInternalError {
			code = jsonrpc.CodeInternalError
		}
		return wireError(code, e.Code, e.Message, e.Details, e.Err)
	}
	return wireError(jsonrpc.CodeInternalError, errs.CodeInternalError, "internal error", nil, err)
}

func wireError(code int64, stableCode string, message string, details map[string]any, cause error) *jsonrpc.Error {
	payloadDetails := map[string]any{}
	for key, value := range details {
		payloadDetails[key] = value
	}
	if cause != nil {
		payloadDetails["cause"] = cause.Error()
	}
	raw, err := json.Marshal(ToolErrorOutput{
		Error: ErrorOutput{
			Code:    stableCode,
			Message: message,
			Details: payloadDetails,
		},
	})
	if err != nil {
		raw = json.RawMessage(`{"error":{"code":"internal_error","message":"internal error","details":{"cause":"error payload marshal failed"}}}`)
	}
	return &jsonrpc.Error{
		Code:    code,
		Message: message,
		Data:    raw,
	}
}

func errorOutput(info *errs.Info) ErrorOutput {
	if info == nil {
		info = &errs.Info{Code: errs.CodeInternalError, Message: "internal error", Details: map[string]any{}}
	}
	return ErrorOutput{
		Code:    info.Code,
		Message: info.Message,
		Details: info.Details,
		At:      info.At,
	}
}
