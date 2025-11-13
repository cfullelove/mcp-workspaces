package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type wsCreateOutREST struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}

func TestHTTP_REST_WorkspaceCreate_NoAuth(t *testing.T) {
	// Build the binary
	tmpBinDir, err := os.MkdirTemp("", "mcp-ws-bin-rest")
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

	// Prepare workspace root and start server (no auth)
	wsRoot, err := os.MkdirTemp("", "mcp-ws-root-rest")
	require.NoError(t, err)
	defer os.RemoveAll(wsRoot)

	host := "127.0.0.1"
	port := "18083"
	server := exec.Command(binaryPath, "--transport=http", "--host="+host, "--port="+port, "--workspaces-root="+wsRoot)
	server.Stdout = os.Stdout
	server.Stderr = os.Stderr
	require.NoError(t, server.Start())
	defer func() { _ = server.Process.Kill() }()

	// Give server time to bind
	time.Sleep(750 * time.Millisecond)

	// Call REST: POST /api/tools/workspace_create
	endpoint := fmt.Sprintf("http://%s:%s/api/tools/workspace_create", host, port)
	payload := map[string]any{"name": "My REST Workspace"}
	body, _ := json.Marshal(payload)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "expected 200 from REST tool")
	respBytes, _ := io.ReadAll(resp.Body)

	var outJSON wsCreateOutREST
	require.NoError(t, json.Unmarshal(respBytes, &outJSON))
	assert.Equal(t, "my-rest-workspace", outJSON.WorkspaceID)

	_, err = os.Stat(filepath.Join(wsRoot, "my-rest-workspace"))
	require.NoError(t, err, "workspace directory should exist")
}

func TestHTTP_REST_WorkspaceCreate_WithAuth(t *testing.T) {
	// Build the binary
	tmpBinDir, err := os.MkdirTemp("", "mcp-ws-bin-rest-auth")
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

	// Prepare workspace root and start server WITH auth token
	wsRoot, err := os.MkdirTemp("", "mcp-ws-root-rest-auth")
	require.NoError(t, err)
	defer os.RemoveAll(wsRoot)

	host := "127.0.0.1"
	port := "18084"
	token := "tokA123"
	server := exec.Command(binaryPath,
		"--transport=http",
		"--host="+host,
		"--port="+port,
		"--workspaces-root="+wsRoot,
		"--auth-tokens="+token,
	)
	server.Stdout = os.Stdout
	server.Stderr = os.Stderr
	require.NoError(t, server.Start())
	defer func() { _ = server.Process.Kill() }()

	// Give server time to bind
	time.Sleep(750 * time.Millisecond)

	endpoint := fmt.Sprintf("http://%s:%s/api/tools/workspace_create", host, port)
	payload := map[string]any{"name": "My Secured Workspace"}
	body, _ := json.Marshal(payload)

	// 1) Without Authorization header -> 401
	{
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}

	// 2) With correct Bearer token -> 200
	{
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		respBytes, _ := io.ReadAll(resp.Body)
		var outJSON wsCreateOutREST
		require.NoError(t, json.Unmarshal(respBytes, &outJSON))
		assert.Equal(t, "my-secured-workspace", outJSON.WorkspaceID)

		_, err = os.Stat(filepath.Join(wsRoot, "my-secured-workspace"))
		require.NoError(t, err, "workspace directory should exist")
	}
}
