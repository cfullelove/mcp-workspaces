package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStdioTransport_WorkspaceCreate tests the workspace/create tool via the stdio transport.
func TestStdioTransport_WorkspaceCreate(t *testing.T) {
	// Build the binary to a temporary location to ensure we have the latest version.
	tmpDir, err := os.MkdirTemp("", "mcp-test-bin")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	binaryPath := filepath.Join(tmpDir, "mcp-workspace-manager")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	err = buildCmd.Run()
	require.NoError(t, err, "Failed to build binary for testing")

	// Create a temporary workspace root for the test.
	workspacesRoot, err := os.MkdirTemp("", "mcp-test-ws-root")
	require.NoError(t, err)
	defer os.RemoveAll(workspacesRoot)

	// Run the application as a subprocess.
	cmd := exec.Command(binaryPath, "--transport=stdio", "--workspaces-root="+workspacesRoot)

	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	defer stdin.Close()

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	defer stdout.Close()

	// Start the command.
	err = cmd.Start()
	require.NoError(t, err)
	defer cmd.Process.Kill()

	// Prepare and send the request.
	req := map[string]interface{}{
		"id":   "1",
		"tool": "workspace/create",
		"params": map[string]string{
			"name": "My Stdio Workspace",
		},
	}
	reqBytes, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = fmt.Fprintln(stdin, string(reqBytes))
	require.NoError(t, err)

	// Read the response.
	reader := bufio.NewReader(stdout)
	line, err := reader.ReadBytes('\n')
	require.NoError(t, err, "Failed to read response from stdout")

	// Unmarshal and verify the response.
	var resp map[string]interface{}
	err = json.Unmarshal(line, &resp)
	require.NoError(t, err)

	require.Nil(t, resp["error"], "Response should not contain an error")
	require.NotNil(t, resp["result"], "Response should contain a result")

	result := resp["result"].(map[string]interface{})
	require.Equal(t, "my-stdio-workspace", result["workspaceId"])

	// Verify that the workspace was actually created on disk.
	wsPath := filepath.Join(workspacesRoot, "my-stdio-workspace")
	_, err = os.Stat(wsPath)
	require.NoError(t, err, "Workspace directory should have been created")
}

func TestHttpTransport_WorkspaceCreate(t *testing.T) {
	// Build the binary
	tmpDir, err := os.MkdirTemp("", "mcp-test-bin-http")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	binaryPath := filepath.Join(tmpDir, "mcp-workspace-manager")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	err = buildCmd.Run()
	require.NoError(t, err, "Failed to build binary for testing")

	// Create a temporary workspace root for the test.
	workspacesRoot, err := os.MkdirTemp("", "mcp-test-ws-root-http")
	require.NoError(t, err)
	defer os.RemoveAll(workspacesRoot)

	// Run the application as a subprocess
	port := "8081" // Use a fixed port for simplicity in testing
	cmd := exec.Command(binaryPath, "--transport=http", "--port="+port, "--workspaces-root="+workspacesRoot)
	err = cmd.Start()
	require.NoError(t, err)
	defer cmd.Process.Kill()

	// Give the server a moment to start. In a real test suite, you'd poll the health endpoint.
	time.Sleep(1 * time.Second)

	// --- 1. Connect to the SSE stream ---
	sseResp, err := http.Get("http://localhost:" + port + "/mcp/stream")
	require.NoError(t, err)
	defer sseResp.Body.Close()
	require.Equal(t, http.StatusOK, sseResp.StatusCode)

	sseReader := bufio.NewReader(sseResp.Body)

	// --- 2. Get the Client ID from the first event ---
	line, err := sseReader.ReadString('\n') // event: connection_ready
	require.NoError(t, err)
	line, err = sseReader.ReadString('\n') // data: {"clientId":"..."}
	require.NoError(t, err)

	var connectResp map[string]string
	// The data part of the event is `{"clientId":"..."}`
	err = json.Unmarshal([]byte(line[6:]), &connectResp)
	require.NoError(t, err)
	clientID := connectResp["clientId"]
	require.NotEmpty(t, clientID)

	_, err = sseReader.ReadString('\n') // Read the trailing newline
	require.NoError(t, err)

	// --- 3. Send the command ---
	createReq := map[string]interface{}{
		"id":   "2",
		"tool": "workspace/create",
		"params": map[string]string{
			"name": "My HTTP Workspace",
		},
	}
	reqBody, err := json.Marshal(createReq)
	require.NoError(t, err)

	httpReq, err := http.NewRequest(http.MethodPost, "http://localhost:"+port+"/mcp/command", bytes.NewReader(reqBody))
	require.NoError(t, err)
	httpReq.Header.Set("X-MCP-Client-ID", clientID)
	httpReq.Header.Set("Content-Type", "application/json")

	postResp, err := http.DefaultClient.Do(httpReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, postResp.StatusCode)

	// --- 4. Read the response from the SSE stream ---
	line, err = sseReader.ReadString('\n') // data: {"id":"2", "result":{...}}
	require.NoError(t, err)

	var resp map[string]interface{}
	err = json.Unmarshal([]byte(line[5:]), &resp)
	require.NoError(t, err)

	require.Nil(t, resp["error"], "Response should not contain an error")
	result := resp["result"].(map[string]interface{})
	assert.Equal(t, "my-http-workspace", result["workspaceId"])

	// Verify that the workspace was actually created on disk.
	wsPath := filepath.Join(workspacesRoot, "my-http-workspace")
	_, err = os.Stat(wsPath)
	require.NoError(t, err, "Workspace directory should have been created")
}