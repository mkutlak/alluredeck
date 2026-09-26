package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
)

// connect runs a real MCP server carrying the tools register adds over an
// in-memory transport and returns a connected client session.
func connect(t *testing.T, register func(*mcpsdk.Server)) *mcpsdk.ClientSession {
	t.Helper()
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test", Version: "v0"}, nil)
	register(srv)
	st, ct := mcpsdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go srv.Run(ctx, st) //nolint:errcheck
	cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "client", Version: "v0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// setupTestServer registers every tool RegisterAll adds. The empty publicURL
// and nil signing key leave the write-confirmation gate disabled.
func setupTestServer(t *testing.T, stores *bootstrap.Stores) *mcpsdk.ClientSession {
	t.Helper()
	return connect(t, func(s *mcpsdk.Server) { tools.RegisterAll(s, stores, zap.NewNop(), "", nil) })
}

// callTool invokes a tool; only a transport error fails the test.
func callTool(t *testing.T, cs *mcpsdk.ClientSession, name string, args map[string]any) *mcpsdk.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	return res
}

// call invokes a tool that must succeed and decodes its structured output.
func call[T any](t *testing.T, cs *mcpsdk.ClientSession, name string, args map[string]any) (T, *mcpsdk.CallToolResult) {
	t.Helper()
	res := callTool(t, cs, name, args)
	if res.IsError {
		t.Fatalf("%s(%v): unexpected tool error: %s", name, args, textOf(t, res))
	}
	return decode[T](t, res), res
}

// decode JSON-round-trips a result's structured content into T.
func decode[T any](t *testing.T, res *mcpsdk.CallToolResult) T {
	t.Helper()
	var out T
	b, err := json.Marshal(res.StructuredContent)
	if err == nil {
		err = json.Unmarshal(b, &out)
	}
	if err != nil {
		t.Fatalf("decoding structured content: %v", err)
	}
	return out
}

// callErr invokes a tool that must fail and returns its error text.
func callErr(t *testing.T, cs *mcpsdk.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res := callTool(t, cs, name, args)
	if !res.IsError {
		t.Fatalf("%s(%v): want a tool error, got success", name, args)
	}
	return textOf(t, res)
}

// textOf returns the text of a result's first content block.
func textOf(t *testing.T, res *mcpsdk.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("result carries no content")
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *mcpsdk.TextContent", res.Content[0])
	}
	return tc.Text
}
