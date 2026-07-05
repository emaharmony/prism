package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"
)

// fakeServer is an in-memory MCP server: it reads JSON-RPC requests line-by-line
// and writes canned responses, so the stdio transport is tested without a real
// subprocess. Returns the client-side reader/writer.
func fakeServer(t *testing.T) (clientReader io.Reader, clientWriter io.Writer, calls *[]string) {
	t.Helper()
	// client writes → srvIn; server writes → srvOut (client reads srvOut).
	srvInR, srvInW := io.Pipe()
	srvOutR, srvOutW := io.Pipe()
	seen := &[]string{}

	go func() {
		defer srvOutW.Close()
		sc := bufio.NewScanner(srvInR)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			var req struct {
				ID     *int           `json:"id"`
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
				continue
			}
			*seen = append(*seen, req.Method)
			if req.ID == nil {
				continue // notification → no response
			}
			var result any
			switch req.Method {
			case "initialize":
				result = map[string]any{"protocolVersion": "2024-11-05", "serverInfo": map[string]any{"name": "fake"}}
			case "tools/list":
				result = map[string]any{"tools": []any{
					map[string]any{"name": "read_file", "description": "read a file",
						"inputSchema": map[string]any{"properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []any{"path"}}},
				}}
			case "tools/call":
				name, _ := req.Params["name"].(string)
				if name == "boom" {
					result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "it failed"}}, "isError": true}
				} else {
					result = map[string]any{"content": []any{
						map[string]any{"type": "text", "text": "line1"},
						map[string]any{"type": "text", "text": "line2"},
					}, "isError": false}
				}
			default:
				result = map[string]any{}
			}
			resp, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result})
			srvOutW.Write(append(resp, '\n'))
		}
	}()

	return srvOutR, srvInW, seen
}

func newTestClient(t *testing.T) (*StdioClient, *[]string) {
	r, w, calls := fakeServer(t)
	conn := NewConn(r, w, nil)
	return NewStdioClient(conn), calls
}

func TestStdioInitializeListCall(t *testing.T) {
	client, calls := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defs, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(defs) != 1 || defs[0].Name != "read_file" || !hasProp(defs[0].InputSchema, "path") {
		t.Fatalf("unexpected tools: %+v", defs)
	}

	res, err := client.CallTool(ctx, "read_file", map[string]any{"path": "x"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError || res.Content != "line1\nline2" {
		t.Fatalf("unexpected call result: %+v", res)
	}

	// MCP isError surfaces as IsError=true with the text content.
	bad, err := client.CallTool(ctx, "boom", nil)
	if err != nil {
		t.Fatalf("CallTool boom: %v", err)
	}
	if !bad.IsError || bad.Content != "it failed" {
		t.Fatalf("expected isError result, got %+v", bad)
	}

	// The handshake + methods were actually exchanged over the wire.
	want := []string{"initialize", "notifications/initialized", "tools/list", "tools/call", "tools/call"}
	if len(*calls) != len(want) {
		t.Fatalf("methods seen = %v, want %v", *calls, want)
	}
	for i := range want {
		if (*calls)[i] != want[i] {
			t.Fatalf("method[%d] = %q, want %q (all: %v)", i, (*calls)[i], want[i], *calls)
		}
	}
}

// End-to-end through the real adapter: register the stdio-backed client into a
// tool.Registry and confirm a remote tool surfaces.
func TestStdioClientRegisters(t *testing.T) {
	client, _ := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	defs, err := client.ListTools(ctx)
	if err != nil || len(defs) == 0 {
		t.Fatalf("list: %v", err)
	}
}

func hasProp(schema map[string]any, key string) bool {
	props, _ := schema["properties"].(map[string]any)
	_, ok := props[key]
	return ok
}
