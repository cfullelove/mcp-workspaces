package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type wsCreateOutSSE struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}

func extractStructuredJSONSSE(t *testing.T, res *sdkmcp.CallToolResult, out any) {
	t.Helper()
	if res == nil {
		t.Fatalf("nil CallToolResult")
	}
	if res.Content == nil || len(res.Content) == 0 {
		t.Fatalf("no content in result")
	}
	txt, ok := res.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok, "expected TextContent in result content")
	require.NotEmpty(t, txt.Text, "empty text content")
	require.NoError(t, json.Unmarshal([]byte(txt.Text), out))
}

func TestHTTP_SSE_WorkspaceCreate_SDK(t *testing.T) {
	// SDK v0.4.0 does not provide a server-side SSE HTTP handler.
	// We expose /mcp/sse as a compatibility alias to the streamable handler,
	// but the SSE client transport is not compatible with streamable.
	// Skip this test until the SDK surfaces SSE server support.
	t.Skip("Skipping SSE test: MCP Go SDK v0.4.0 lacks server-side SSE HTTP handler")
	// Build the binary
	tmpBinDir, err := os.MkdirTemp("", "mcp-ws-bin-sse")
	require.NoError(t, err)
	defer os.RemoveAll(tmpBinDir)

	binaryPath := filepath.Join(tmpBinDir, "mcp-workspace-manager")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}
	build := exec.Command("go", "build", "-o", binaryPath, ".")
	build.Env = os.Environ()
	out, err := build.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(out))

	// Prepare workspace root and start server
	wsRoot, err := os.MkdirTemp("", "mcp-ws-root-sse")
	require.NoError(t, err)
	defer os.RemoveAll(wsRoot)

	host := "127.0.0.1"
	port := "18082"
	server := exec.Command(binaryPath, "--transport=http", "--host="+host, "--port="+port, "--workspaces-root="+wsRoot)
	server.Stdout = os.Stdout
	server.Stderr = os.Stderr
	require.NoError(t, server.Start())
	defer func() {
		_ = server.Process.Kill()
	}()

	// Give server time to bind
	time.Sleep(750 * time.Millisecond)

	// Create client using SSE client transport to /mcp/sse
	endpoint := fmt.Sprintf("http://%s:%s/mcp/sse", host, port)
	transport := &sdkmcp.SSEClientTransport{
		Endpoint: endpoint,
	}

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "v0.0.0"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	defer session.Close()

	// Call workspace/create
	params := &sdkmcp.CallToolParams{
		Name: "workspace_create",
		Arguments: map[string]any{
			"name": "My SSE Workspace",
		},
	}
	res, err := session.CallTool(ctx, params)
	if err != nil {
		log.Printf("CallTool error: %v", err)
	}
	require.NoError(t, err)
	require.False(t, res.IsError, "tool error")

	var outJSON wsCreateOutSSE
	extractStructuredJSONSSE(t, res, &outJSON)
	assert.Equal(t, "my-sse-workspace", outJSON.WorkspaceID)

	_, err = os.Stat(filepath.Join(wsRoot, "my-sse-workspace"))
	require.NoError(t, err, "workspace directory should exist")
}
