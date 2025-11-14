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
	build := exec.Command("go", "build", "-tags=dev", "-o", binaryPath, ".")
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
	build := exec.Command("go", "build", "-tags=dev", "-o", binaryPath, ".")
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

// New tests ensuring .git/.gitkeep are hidden and immutable

type listDirReq struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}
type listDirResp struct {
	Entries []string `json:"entries"`
}

type readFileReq struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}
type writeFileReq struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
	Content     string `json:"content"`
}
type deleteFileReq struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}

type createDirReq struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}

type searchFilesReq struct {
	WorkspaceID     string   `json:"workspaceId"`
	Path            string   `json:"path"`
	Pattern         string   `json:"pattern"`
	ExcludePatterns []string `json:"excludePatterns,omitempty"`
}
type searchFilesResp struct {
	Matches []string `json:"matches"`
}

type treeNode struct {
	Name     string      `json:"name"`
	Type     string      `json:"type"`
	Children *[]treeNode `json:"children,omitempty"`
}
type dirTreeResp struct {
	Tree []treeNode `json:"tree"`
}

func TestHTTP_REST_FSListDirectory_HidesGitAndGitkeep(t *testing.T) {
	// Build the binary
	tmpBinDir, err := os.MkdirTemp("", "mcp-ws-bin-rest-hide")
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

	// Prepare workspace root and start server (no auth)
	wsRoot, err := os.MkdirTemp("", "mcp-ws-root-rest-hide")
	require.NoError(t, err)
	defer os.RemoveAll(wsRoot)

	host := "127.0.0.1"
	port := "18085"
	server := exec.Command(binaryPath, "--transport=http", "--host="+host, "--port="+port, "--workspaces-root="+wsRoot)
	server.Stdout = os.Stdout
	server.Stderr = os.Stderr
	require.NoError(t, server.Start())
	defer func() { _ = server.Process.Kill() }()

	// Give server time to bind
	time.Sleep(750 * time.Millisecond)

	// Create workspace
	createEP := fmt.Sprintf("http://%s:%s/api/tools/workspace_create", host, port)
	createBody, _ := json.Marshal(map[string]any{"name": "Hide Test"})
	req, err := http.NewRequest(http.MethodPost, createEP, bytes.NewReader(createBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	bs, _ := io.ReadAll(resp.Body)
	var ws wsCreateOutREST
	require.NoError(t, json.Unmarshal(bs, &ws))

	// List root directory of workspace
	listEP := fmt.Sprintf("http://%s:%s/api/tools/fs_list_directory", host, port)
	ldReq := listDirReq{WorkspaceID: ws.WorkspaceID, Path: "."}
	ldBody, _ := json.Marshal(ldReq)
	req2, _ := http.NewRequest(http.MethodPost, listEP, bytes.NewReader(ldBody))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	ldBytes, _ := io.ReadAll(resp2.Body)

	var ld listDirResp
	require.NoError(t, json.Unmarshal(ldBytes, &ld))

	// Ensure special entries are hidden
	for _, e := range ld.Entries {
		assert.NotEqual(t, "[DIR] .git", e, "'.git' must be hidden")
		assert.NotEqual(t, "[FILE] .gitkeep", e, "'.gitkeep' must be hidden")
	}
}

func TestHTTP_REST_ProtectedPathsDenied(t *testing.T) {
	// Build the binary
	tmpBinDir, err := os.MkdirTemp("", "mcp-ws-bin-rest-deny")
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

	// Prepare workspace root and start server (no auth)
	wsRoot, err := os.MkdirTemp("", "mcp-ws-root-rest-deny")
	require.NoError(t, err)
	defer os.RemoveAll(wsRoot)

	host := "127.0.0.1"
	port := "18086"
	server := exec.Command(binaryPath, "--transport=http", "--host="+host, "--port="+port, "--workspaces-root="+wsRoot)
	server.Stdout = os.Stdout
	server.Stderr = os.Stderr
	require.NoError(t, server.Start())
	defer func() { _ = server.Process.Kill() }()

	// Give server time to bind
	time.Sleep(750 * time.Millisecond)

	// Create workspace
	createEP := fmt.Sprintf("http://%s:%s/api/tools/workspace_create", host, port)
	createBody, _ := json.Marshal(map[string]any{"name": "Deny Test"})
	req, err := http.NewRequest(http.MethodPost, createEP, bytes.NewReader(createBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	bs, _ := io.ReadAll(resp.Body)
	var ws wsCreateOutREST
	require.NoError(t, json.Unmarshal(bs, &ws))

	// 1) Read .git/config -> 404
	readEP := fmt.Sprintf("http://%s:%s/api/tools/fs_read_text_file", host, port)
	rBody, _ := json.Marshal(readFileReq{WorkspaceID: ws.WorkspaceID, Path: ".git/config"})
	req1, _ := http.NewRequest(http.MethodPost, readEP, bytes.NewReader(rBody))
	req1.Header.Set("Content-Type", "application/json")
	resp1, err := http.DefaultClient.Do(req1)
	require.NoError(t, err)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusNotFound, resp1.StatusCode)

	// 2) Write .gitkeep -> 404
	writeEP := fmt.Sprintf("http://%s:%s/api/tools/fs_write_file", host, port)
	wBody, _ := json.Marshal(writeFileReq{WorkspaceID: ws.WorkspaceID, Path: ".gitkeep", Content: "x"})
	req2, _ := http.NewRequest(http.MethodPost, writeEP, bytes.NewReader(wBody))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()
	require.Equal(t, http.StatusNotFound, resp2.StatusCode)

	// 3) Delete .gitkeep -> 404
	delEP := fmt.Sprintf("http://%s:%s/api/tools/fs_delete_file", host, port)
	dBody, _ := json.Marshal(deleteFileReq{WorkspaceID: ws.WorkspaceID, Path: ".gitkeep"})
	req3, _ := http.NewRequest(http.MethodPost, delEP, bytes.NewReader(dBody))
	req3.Header.Set("Content-Type", "application/json")
	resp3, err := http.DefaultClient.Do(req3)
	require.NoError(t, err)
	defer resp3.Body.Close()
	require.Equal(t, http.StatusNotFound, resp3.StatusCode)
}

func TestHTTP_REST_SearchAndTree_SkipProtected(t *testing.T) {
	// Build the binary
	tmpBinDir, err := os.MkdirTemp("", "mcp-ws-bin-rest-search")
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

	// Prepare workspace root and start server (no auth)
	wsRoot, err := os.MkdirTemp("", "mcp-ws-root-rest-search")
	require.NoError(t, err)
	defer os.RemoveAll(wsRoot)

	host := "127.0.0.1"
	port := "18087"
	server := exec.Command(binaryPath, "--transport=http", "--host="+host, "--port="+port, "--workspaces-root="+wsRoot)
	server.Stdout = os.Stdout
	server.Stderr = os.Stderr
	require.NoError(t, server.Start())
	defer func() { _ = server.Process.Kill() }()

	// Give server time to bind
	time.Sleep(750 * time.Millisecond)

	// Create workspace
	createEP := fmt.Sprintf("http://%s:%s/api/tools/workspace_create", host, port)
	createBody, _ := json.Marshal(map[string]any{"name": "Search Test"})
	req, err := http.NewRequest(http.MethodPost, createEP, bytes.NewReader(createBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	bs, _ := io.ReadAll(resp.Body)
	var ws wsCreateOutREST
	require.NoError(t, json.Unmarshal(bs, &ws))

	// Create dir1 (will contain internal .gitkeep)
	createDirEP := fmt.Sprintf("http://%s:%s/api/tools/fs_create_directory", host, port)
	cdBody, _ := json.Marshal(createDirReq{WorkspaceID: ws.WorkspaceID, Path: "dir1"})
	reqCD, _ := http.NewRequest(http.MethodPost, createDirEP, bytes.NewReader(cdBody))
	reqCD.Header.Set("Content-Type", "application/json")
	respCD, err := http.DefaultClient.Do(reqCD)
	require.NoError(t, err)
	defer respCD.Body.Close()
	require.Equal(t, http.StatusOK, respCD.StatusCode)

	// Write a regular file
	writeEP := fmt.Sprintf("http://%s:%s/api/tools/fs_write_file", host, port)
	wBody, _ := json.Marshal(writeFileReq{WorkspaceID: ws.WorkspaceID, Path: "dir1/foo.txt", Content: "hello"})
	reqW, _ := http.NewRequest(http.MethodPost, writeEP, bytes.NewReader(wBody))
	reqW.Header.Set("Content-Type", "application/json")
	respW, err := http.DefaultClient.Do(reqW)
	require.NoError(t, err)
	defer respW.Body.Close()
	require.Equal(t, http.StatusOK, respW.StatusCode)

	// Search for *.gitkeep -> should be empty due to protection
	searchEP := fmt.Sprintf("http://%s:%s/api/tools/fs_search_files", host, port)
	sfBody, _ := json.Marshal(searchFilesReq{WorkspaceID: ws.WorkspaceID, Path: ".", Pattern: "*.gitkeep"})
	reqS1, _ := http.NewRequest(http.MethodPost, searchEP, bytes.NewReader(sfBody))
	reqS1.Header.Set("Content-Type", "application/json")
	respS1, err := http.DefaultClient.Do(reqS1)
	require.NoError(t, err)
	defer respS1.Body.Close()
	require.Equal(t, http.StatusOK, respS1.StatusCode)
	sb1, _ := io.ReadAll(respS1.Body)
	var sOut1 searchFilesResp
	require.NoError(t, json.Unmarshal(sb1, &sOut1))
	assert.Len(t, sOut1.Matches, 0, "no .gitkeep files should be returned")

	// Search for *.txt -> should include our file
	sfBody2, _ := json.Marshal(searchFilesReq{WorkspaceID: ws.WorkspaceID, Path: ".", Pattern: "*.txt"})
	reqS2, _ := http.NewRequest(http.MethodPost, searchEP, bytes.NewReader(sfBody2))
	reqS2.Header.Set("Content-Type", "application/json")
	respS2, err := http.DefaultClient.Do(reqS2)
	require.NoError(t, err)
	defer respS2.Body.Close()
	require.Equal(t, http.StatusOK, respS2.StatusCode)
	sb2, _ := io.ReadAll(respS2.Body)
	var sOut2 searchFilesResp
	require.NoError(t, json.Unmarshal(sb2, &sOut2))
	assert.Contains(t, sOut2.Matches, "dir1/foo.txt")

	// Directory tree -> should not contain .git or .gitkeep at any level
	treeEP := fmt.Sprintf("http://%s:%s/api/tools/fs_directory_tree", host, port)
	treeReqBody, _ := json.Marshal(map[string]any{"workspaceId": ws.WorkspaceID, "path": "."})
	reqT, _ := http.NewRequest(http.MethodPost, treeEP, bytes.NewReader(treeReqBody))
	reqT.Header.Set("Content-Type", "application/json")
	respT, err := http.DefaultClient.Do(reqT)
	require.NoError(t, err)
	defer respT.Body.Close()
	require.Equal(t, http.StatusOK, respT.StatusCode)
	tb, _ := io.ReadAll(respT.Body)
	var tOut dirTreeResp
	require.NoError(t, json.Unmarshal(tb, &tOut))

	var containsSpecial func(nodes []treeNode, name string) bool
	containsSpecial = func(nodes []treeNode, name string) bool {
		for _, n := range nodes {
			if n.Name == name {
				return true
			}
			if n.Children != nil && containsSpecial(*n.Children, name) {
				return true
			}
		}
		return false
	}

	assert.False(t, containsSpecial(tOut.Tree, ".git"), "tree must not include .git")
	assert.False(t, containsSpecial(tOut.Tree, ".gitkeep"), "tree must not include .gitkeep")
}
