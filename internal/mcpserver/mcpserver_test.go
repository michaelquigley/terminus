package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/michaelquigley/terminus/internal/broker"
	"github.com/michaelquigley/terminus/internal/errs"
	"github.com/michaelquigley/theharnessbody/reviewer/dummy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func dummyBroker(t *testing.T) *broker.Broker {
	t.Helper()
	return broker.New(broker.Options{
		LogDestination: t.TempDir(),
		Reviewer:       dummy.New(dummy.Options{Raw: json.RawMessage(`{"summary":"x","findings":[]}`)}),
		ReviewerInfo:   broker.ReviewerInfo{Name: "dummy", Impl: "dummy"},
	})
}

func dialMCP(t *testing.T, b *broker.Broker) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := New(b).Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// collect_review with no review_id lists known runs; exercises the full MCP
// dispatch path (New -> RegisterTools -> handler -> output) against an empty store.
func TestCollectReviewListThroughMCP(t *testing.T) {
	cs := dialMCP(t, dummyBroker(t))
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "collect_review"})
	if err != nil {
		t.Fatalf("call_tool transport error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %+v", res)
	}
}

// an unknown review_id flows a CodeNotFound errs.Error back through the error
// mapping as an IsError tool result (not a transport error).
func TestCollectReviewNotFoundThroughMCP(t *testing.T) {
	cs := dialMCP(t, dummyBroker(t))
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "collect_review",
		Arguments: map[string]any{"review_id": "nope"},
	})
	if err != nil {
		t.Fatalf("call_tool transport error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected IsError result for unknown review_id")
	}
}

// direct coverage of the error mapping: errs.Error becomes an IsError result with
// structured content; a plain error becomes a transport-level rpc error.
func TestToolErrorMapping(t *testing.T) {
	res, _, err := toolErrorResult(errs.New(errs.CodeUserError, "bad input", nil, nil))
	if err != nil {
		t.Fatalf("errs.Error should not be a transport error, got %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("expected IsError result for errs.Error")
	}
	out, ok := res.StructuredContent.(ToolErrorOutput)
	if !ok || out.Error.Code != errs.CodeUserError || out.Error.Message != "bad input" {
		t.Fatalf("unexpected structured error: %+v", res.StructuredContent)
	}

	if _, _, err := toolErrorResult(errors.New("boom")); err == nil {
		t.Fatal("expected a transport rpc error for a non-errs error")
	}
}
