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

type wsCreateOut struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}

// helper: extract structured JSON from CallToolResult.Content (TextContent JSON) into out.
func extractStructuredJSON(t *testing.T, res *sdkmcp.CallToolResult, out any) {
	t.Helper()
	if res == nil {
		t.Fatalf("nil CallToolResult")
	}
	// Prefer Content as Text JSON (server sets StructuredContent and mirrors to TextContent if Content unset)
	if res.Content == nil || len(res.Content) == 0 {
		t.Fatalf("no content in result")
	}
	// First content item should be *mcp.TextContent with JSON string
	txt, ok := res.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok, "expected TextContent in result content")
	require.NotEmpty(t, txt.Text, "empty text content")
	require.NoError(t, json.Unmarshal([]byte(txt.Text), out))
}

func TestStdio_WorkspaceCreate_SDK(t *testing.T) {
	// Build the binary
	tmpBinDir, err := os.MkdirTemp("", "mcp-ws-bin-stdio")
	require.NoError(t, err)
	defer os.RemoveAll(tmpBinDir)

	binaryPath := filepath.Join(tmpBinDir, "mcp-workspace-manager")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}
	build := exec.Command("go", "build", "-tags=dev", "-o", binaryPath, ".")
	build.Env = os.Environ()
	out, err := build.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(out))

	// Prepare workspace root
	wsRoot, err := os.MkdirTemp("", "mcp-ws-root-stdio")
	require.NoError(t, err)
	defer os.RemoveAll(wsRoot)

	// Prepare client using CommandTransport to invoke the binary with stdio transport
	cmd := exec.Command(binaryPath, "--transport=stdio", "--workspaces-root="+wsRoot)
	transport := &sdkmcp.CommandTransport{Command: cmd}

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "v0.0.0"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err)
	defer session.Close()

	// Call workspace/create
	params := &sdkmcp.CallToolParams{
		Name: "workspace_create",
		Arguments: map[string]any{
			"name": "My Stdio Workspace",
		},
	}
	res, err := session.CallTool(ctx, params)
	require.NoError(t, err)
	require.False(t, res.IsError, "tool error")

	var outJSON wsCreateOut
	extractStructuredJSON(t, res, &outJSON)
	assert.Equal(t, "my-stdio-workspace", outJSON.WorkspaceID)

	// Verify created dir
	_, err = os.Stat(filepath.Join(wsRoot, "my-stdio-workspace"))
	require.NoError(t, err, "workspace directory should exist")
}

func TestHTTP_Streamable_WorkspaceCreate_SDK(t *testing.T) {
	// Build the binary
	tmpBinDir, err := os.MkdirTemp("", "mcp-ws-bin-http")
	require.NoError(t, err)
	defer os.RemoveAll(tmpBinDir)

	binaryPath := filepath.Join(tmpBinDir, "mcp-workspace-manager")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}
	build := exec.Command("go", "build", "-tags=dev", "-o", binaryPath, ".")
	build.Env = os.Environ()
	out, err := build.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(out))

	// Prepare workspace root and start server
	wsRoot, err := os.MkdirTemp("", "mcp-ws-root-http")
	require.NoError(t, err)
	defer os.RemoveAll(wsRoot)

	host := "127.0.0.1"
	port := "18081"
	server := exec.Command(binaryPath, "--transport=http", "--host="+host, "--port="+port, "--workspaces-root="+wsRoot)
	server.Stdout = os.Stdout
	server.Stderr = os.Stderr
	require.NoError(t, server.Start())
	defer func() {
		_ = server.Process.Kill()
	}()

	// Give server time to bind
	time.Sleep(750 * time.Millisecond)

	// Create client using Streamable client transport to /mcp
	endpoint := fmt.Sprintf("http://%s:%s/mcp", host, port)
	transport := &sdkmcp.StreamableClientTransport{
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
			"name": "My HTTP Workspace",
		},
	}
	res, err := session.CallTool(ctx, params)
	if err != nil {
		log.Printf("CallTool error: %v", err)
	}
	require.NoError(t, err)
	require.False(t, res.IsError, "tool error")

	var outJSON wsCreateOut
	extractStructuredJSON(t, res, &outJSON)
	assert.Equal(t, "my-http-workspace", outJSON.WorkspaceID)

	_, err = os.Stat(filepath.Join(wsRoot, "my-http-workspace"))
	require.NoError(t, err, "workspace directory should exist")
}
