package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/terminus/internal/errs"
	"github.com/michaelquigley/terminus/internal/report"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// tool is the transport contract for one MCP tool plus the service it wraps.
// the shared register helper turns it into a raw SDK handler that owns the
// whole dd binding path: decode and validate the input, bind it, run the
// service, then unbind the success DTO (or the error envelope) so only dd
// output crosses the SDK transport.
type tool[In any, Out any] struct {
	name        string
	description string
	annotations *mcp.ToolAnnotations
	input       *jsonschema.Schema
	// output is the success schema, or nil when the tool declares none.
	output *jsonschema.Schema
	run    func(ctx context.Context, in In) (Out, error)
}

// resolveSchema resolves a local schema once at registration. our schemas
// carry no external references, so resolution needs no network.
func resolveSchema(name string, s *jsonschema.Schema) (*jsonschema.Resolved, error) {
	r, err := s.Resolve(&jsonschema.ResolveOptions{ValidateDefaults: true})
	if err != nil {
		return nil, fmt.Errorf("%s schema: %w", name, err)
	}
	return r, nil
}

// register resolves the tool's schemas and hands a raw handler to the low-level
// AddTool. schema-construction or resolution failures surface here at setup,
// not per request.
func register[In any, Out any](server *mcp.Server, t tool[In, Out]) error {
	inResolved, err := resolveSchema(t.name+" input", t.input)
	if err != nil {
		return err
	}
	var outResolved *jsonschema.Resolved
	if t.output != nil {
		if outResolved, err = resolveSchema(t.name+" output", t.output); err != nil {
			return err
		}
	}
	mcpTool := &mcp.Tool{
		Name:        t.name,
		Description: t.description,
		Annotations: t.annotations,
		InputSchema: t.input,
	}
	if t.output != nil {
		mcpTool.OutputSchema = t.output
	}
	server.AddTool(mcpTool, newToolHandler(t.run, inResolved, outResolved))
	return nil
}

// newToolHandler builds the raw handler around the service. it is the single
// place that binds inputs and unbinds outputs, so the typed mcp.AddTool
// output-overwrite path is not used at all.
func newToolHandler[In any, Out any](
	run func(ctx context.Context, in In) (Out, error),
	inResolved, outResolved *jsonschema.Resolved,
) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := ctx.Err(); err != nil {
			return nil, protocolError(err)
		}
		args := req.Params.Arguments
		if len(args) == 0 {
			args = []byte("{}")
		}
		// decode the raw arguments to a map so validation sees an explicit
		// false or empty value rather than a zero-filled struct.
		data, err := dd.DecodeStrictJSON(args)
		if err != nil {
			return errorToolResult(errs.New(errs.CodeUserError, "malformed arguments", err, nil)), nil
		}
		if err := inResolved.Validate(data); err != nil {
			return errorToolResult(errs.New(errs.CodeUserError, "arguments failed schema validation", err, nil)), nil
		}
		in := new(In)
		if err := dd.Bind(in, data, dd.Strict()); err != nil {
			return errorToolResult(errs.New(errs.CodeUserError, "bind arguments", err, nil)), nil
		}

		out, err := run(ctx, *in)
		if err != nil {
			return serviceError(err)
		}
		return successResult(out, outResolved)
	}
}

// successResult unbinds the success DTO through the shared codec, validates the
// map against the declared success schema when there is one, and emits the same
// dd JSON as text content. only dd-produced data crosses the SDK transport.
func successResult[Out any](out Out, outResolved *jsonschema.Resolved) (*mcp.CallToolResult, error) {
	data, err := report.Unbind(out)
	if err != nil {
		return nil, protocolError(errs.New(errs.CodeInternalError, "unbind tool output", err, nil))
	}
	if outResolved != nil {
		if verr := outResolved.Validate(data); verr != nil {
			return nil, protocolError(errs.New(errs.CodeInternalError, "tool output failed schema validation", verr, nil))
		}
	}
	text, err := report.UnbindJSON(out)
	if err != nil {
		return nil, protocolError(errs.New(errs.CodeInternalError, "encode tool output", err, nil))
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(text)}},
		StructuredContent: data,
	}, nil
}

// serviceError routes a service error: classified errs become an IsError tool
// result with a dd-unbound structured envelope; anything else is a protocol
// error. classified errors never become empty success data.
func serviceError(err error) (*mcp.CallToolResult, error) {
	var e *errs.Error
	if errors.As(err, &e) {
		return errorToolResult(e), nil
	}
	return nil, protocolError(err)
}

// errorToolResult turns a classified service error into an IsError tool result.
// the structured content is the dd-unbound error envelope, so it agrees with
// the on-disk form; a defensive fallback keeps a usable envelope if unbinding
// itself ever fails.
func errorToolResult(e *errs.Error) *mcp.CallToolResult {
	eo := errorOutput(errs.NewInfo(e))
	structured, err := report.Unbind(ToolErrorOutput{Error: eo})
	if err != nil {
		structured = map[string]any{"error": map[string]any{
			"code": eo.Code, "message": eo.Message, "details": eo.Details,
		}}
	}
	text := fmt.Sprintf("%s: %s", eo.Code, eo.Message)
	if cause, ok := eo.Details["cause"].(string); ok && cause != "" {
		text += "\ncause: " + cause
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: text}},
		StructuredContent: structured,
		IsError:           true,
	}
}

// protocolError converts an unclassified failure to a JSON-RPC protocol error,
// dd-encoding any project data carried in the error's Data.
func protocolError(err error) error {
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

// wireError builds a JSON-RPC error whose Data carries the project error
// envelope encoded through the shared dd codec.
func wireError(code int64, stableCode, message string, details map[string]any, cause error) *jsonrpc.Error {
	payload := map[string]any{}
	for key, value := range details {
		payload[key] = value
	}
	if cause != nil {
		payload["cause"] = cause.Error()
	}
	raw, err := report.UnbindJSON(ToolErrorOutput{
		Error: ErrorOutput{Code: stableCode, Message: message, Details: payload},
	})
	if err != nil {
		raw = []byte(`{"error":{"code":"internal_error","message":"internal error","details":{"cause":"error payload marshal failed"}}}`)
	}
	return &jsonrpc.Error{
		Code:    code,
		Message: message,
		Data:    json.RawMessage(raw),
	}
}

// errorOutput projects a classified error into the wire error envelope. a nil
// info becomes a minimal internal-error envelope so the shape is never absent,
// and details is normalized to an object: dd emits {} for a nil map, so the
// envelope always carries details as an object, never null or absent.
func errorOutput(info *errs.Info) ErrorOutput {
	if info == nil {
		info = &errs.Info{Code: errs.CodeInternalError, Message: "internal error", Details: map[string]any{}}
	}
	details := info.Details
	if details == nil {
		details = map[string]any{}
	}
	return ErrorOutput{
		Code:    info.Code,
		Message: info.Message,
		Details: details,
		At:      info.At,
	}
}
