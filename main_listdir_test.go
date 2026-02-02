// go:build dev

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

func TestHTTP_REST_FSListDirectory_NonRecursive(t *testing.T) {
	// Build the binary
	tmpBinDir, err := os.MkdirTemp("", "mcp-listdir-bin")
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
	wsRoot, err := os.MkdirTemp("", "mcp-listdir-root")
	require.NoError(t, err)
	defer os.RemoveAll(wsRoot)

	host := "127.0.0.1"
	port := "19988"
	server := exec.Command(binaryPath, "--transport=http", "--host="+host, "--port="+port, "--workspaces-root="+wsRoot)
	server.Stdout = os.Stdout
	server.Stderr = os.Stderr
	require.NoError(t, server.Start())
	defer func() { _ = server.Process.Kill() }()

	time.Sleep(750 * time.Millisecond)

	baseURL := fmt.Sprintf("http://%s:%s", host, port)

	// Create workspace
	wsID := createWorkspace(t, baseURL, "test-ws")

	// Create nested directory structure
	createDir(t, baseURL, wsID, "dir1")
	createDir(t, baseURL, wsID, "dir1/subdir1")
	createFile(t, baseURL, wsID, "file1.txt", "content1")
	createFile(t, baseURL, wsID, "dir1/file2.txt", "content2")
	createFile(t, baseURL, wsID, "dir1/subdir1/file3.txt", "content3")

	// Note: .gitkeep files are automatically created by createDir
	// and should be filtered out by the listing

	// List root directory (non-recursive)
	resp := listDirectory(t, baseURL, wsID, ".", false, nil)

	// Should only contain immediate children
	assert.Contains(t, resp.Entries, "[DIR] dir1")
	assert.Contains(t, resp.Entries, "[FILE] file1.txt")

	// Verify .gitkeep is filtered out
	for _, entry := range resp.Entries {
		assert.NotContains(t, entry, ".gitkeep", "Protected .gitkeep should not appear in listing")
	}

	// Should not contain nested files
	assert.NotContains(t, resp.Entries, "[DIR] dir1/subdir1")
	assert.NotContains(t, resp.Entries, "[FILE] dir1/file2.txt")
}

func TestHTTP_REST_FSListDirectory_Recursive(t *testing.T) {
	// Build the binary
	tmpBinDir, err := os.MkdirTemp("", "mcp-listdir-bin")
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
	wsRoot, err := os.MkdirTemp("", "mcp-listdir-root")
	require.NoError(t, err)
	defer os.RemoveAll(wsRoot)

	host := "127.0.0.1"
	port := "19989"
	server := exec.Command(binaryPath, "--transport=http", "--host="+host, "--port="+port, "--workspaces-root="+wsRoot)
	server.Stdout = os.Stdout
	server.Stderr = os.Stderr
	require.NoError(t, server.Start())
	defer func() { _ = server.Process.Kill() }()

	time.Sleep(750 * time.Millisecond)

	baseURL := fmt.Sprintf("http://%s:%s", host, port)

	// Create workspace
	wsID := createWorkspace(t, baseURL, "test-ws")

	// Create nested structure (3+ levels deep)
	createDir(t, baseURL, wsID, "level1")
	createDir(t, baseURL, wsID, "level1/level2")
	createDir(t, baseURL, wsID, "level1/level2/level3")
	createFile(t, baseURL, wsID, "root.txt", "root")
	createFile(t, baseURL, wsID, "level1/file1.txt", "content1")
	createFile(t, baseURL, wsID, "level1/level2/file2.txt", "content2")
	createFile(t, baseURL, wsID, "level1/level2/level3/file3.txt", "content3")

	// Note: .gitkeep files are automatically created by createDir at all levels
	// and should be filtered out by the recursive listing

	// List recursively
	resp := listDirectory(t, baseURL, wsID, ".", true, nil)

	// Should contain all files and directories
	assert.Contains(t, resp.Entries, "[FILE] root.txt")
	assert.Contains(t, resp.Entries, "[DIR] level1")
	assert.Contains(t, resp.Entries, "[FILE] level1/file1.txt")
	assert.Contains(t, resp.Entries, "[DIR] level1/level2")
	assert.Contains(t, resp.Entries, "[FILE] level1/level2/file2.txt")
	assert.Contains(t, resp.Entries, "[DIR] level1/level2/level3")
	assert.Contains(t, resp.Entries, "[FILE] level1/level2/level3/file3.txt")

	// Should not contain protected names at any level
	for _, entry := range resp.Entries {
		assert.NotContains(t, entry, ".gitkeep", "Protected .gitkeep should not appear in recursive listing")
	}
}

func TestHTTP_REST_FSListDirectory_RecursiveWithMaxDepth(t *testing.T) {
	// Build the binary
	tmpBinDir, err := os.MkdirTemp("", "mcp-listdir-bin")
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
	wsRoot, err := os.MkdirTemp("", "mcp-listdir-root")
	require.NoError(t, err)
	defer os.RemoveAll(wsRoot)

	host := "127.0.0.1"
	port := "19990"
	server := exec.Command(binaryPath, "--transport=http", "--host="+host, "--port="+port, "--workspaces-root="+wsRoot)
	server.Stdout = os.Stdout
	server.Stderr = os.Stderr
	require.NoError(t, server.Start())
	defer func() { _ = server.Process.Kill() }()

	time.Sleep(750 * time.Millisecond)

	baseURL := fmt.Sprintf("http://%s:%s", host, port)

	// Create workspace
	wsID := createWorkspace(t, baseURL, "test-ws")

	// Create 4+ levels of nesting
	createDir(t, baseURL, wsID, "l1")
	createDir(t, baseURL, wsID, "l1/l2")
	createDir(t, baseURL, wsID, "l1/l2/l3")
	createDir(t, baseURL, wsID, "l1/l2/l3/l4")
	createFile(t, baseURL, wsID, "root.txt", "root")
	createFile(t, baseURL, wsID, "l1/f1.txt", "f1")
	createFile(t, baseURL, wsID, "l1/l2/f2.txt", "f2")
	createFile(t, baseURL, wsID, "l1/l2/l3/f3.txt", "f3")
	createFile(t, baseURL, wsID, "l1/l2/l3/l4/f4.txt", "f4")

	// Test maxDepth: 0 (immediate children only)
	resp := listDirectory(t, baseURL, wsID, ".", true, intPtr(0))
	assert.Contains(t, resp.Entries, "[FILE] root.txt")
	assert.Contains(t, resp.Entries, "[DIR] l1")
	assert.NotContains(t, resp.Entries, "[FILE] l1/f1.txt")
	assert.NotContains(t, resp.Entries, "[DIR] l1/l2")

	// Test maxDepth: 1
	resp = listDirectory(t, baseURL, wsID, ".", true, intPtr(1))
	assert.Contains(t, resp.Entries, "[FILE] root.txt")
	assert.Contains(t, resp.Entries, "[DIR] l1")
	assert.Contains(t, resp.Entries, "[FILE] l1/f1.txt")
	assert.Contains(t, resp.Entries, "[DIR] l1/l2")
	assert.NotContains(t, resp.Entries, "[FILE] l1/l2/f2.txt")
	assert.NotContains(t, resp.Entries, "[DIR] l1/l2/l3")

	// Test maxDepth: 2
	resp = listDirectory(t, baseURL, wsID, ".", true, intPtr(2))
	assert.Contains(t, resp.Entries, "[FILE] root.txt")
	assert.Contains(t, resp.Entries, "[DIR] l1")
	assert.Contains(t, resp.Entries, "[FILE] l1/f1.txt")
	assert.Contains(t, resp.Entries, "[DIR] l1/l2")
	assert.Contains(t, resp.Entries, "[FILE] l1/l2/f2.txt")
	assert.Contains(t, resp.Entries, "[DIR] l1/l2/l3")
	assert.NotContains(t, resp.Entries, "[FILE] l1/l2/l3/f3.txt")
	assert.NotContains(t, resp.Entries, "[DIR] l1/l2/l3/l4")
}

func TestHTTP_REST_FSListDirectory_RecursiveFiltersProtected(t *testing.T) {
	// Build the binary
	tmpBinDir, err := os.MkdirTemp("", "mcp-listdir-bin")
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
	wsRoot, err := os.MkdirTemp("", "mcp-listdir-root")
	require.NoError(t, err)
	defer os.RemoveAll(wsRoot)

	host := "127.0.0.1"
	port := "19991"
	server := exec.Command(binaryPath, "--transport=http", "--host="+host, "--port="+port, "--workspaces-root="+wsRoot)
	server.Stdout = os.Stdout
	server.Stderr = os.Stderr
	require.NoError(t, server.Start())
	defer func() { _ = server.Process.Kill() }()

	time.Sleep(750 * time.Millisecond)

	baseURL := fmt.Sprintf("http://%s:%s", host, port)

	// Create workspace
	wsID := createWorkspace(t, baseURL, "test-ws")

	// Create normal directory structure
	// Note: .gitkeep files are automatically created by createDir
	createDir(t, baseURL, wsID, "src")
	createFile(t, baseURL, wsID, "src/main.go", "package main")
	createFile(t, baseURL, wsID, "src/util.go", "package main")

	// List recursively
	resp := listDirectory(t, baseURL, wsID, ".", true, nil)

	// Should contain normal files
	assert.Contains(t, resp.Entries, "[DIR] src")
	assert.Contains(t, resp.Entries, "[FILE] src/main.go")
	assert.Contains(t, resp.Entries, "[FILE] src/util.go")

	// Should not contain any .gitkeep files (they are automatically created but should be filtered)
	for _, entry := range resp.Entries {
		assert.NotContains(t, entry, ".gitkeep", "Protected .gitkeep should not appear in listing")
	}
}

// Helper functions

func listDirectory(t *testing.T, baseURL, wsID, path string, recursive bool, maxDepth *int) struct {
	Entries []string `json:"entries"`
} {
	reqBody := map[string]interface{}{
		"workspaceId": wsID,
		"path":        path,
	}
	if recursive {
		reqBody["recursive"] = recursive
	}
	if maxDepth != nil {
		reqBody["maxDepth"] = *maxDepth
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/tools/fs_list_directory", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result struct {
		Entries []string `json:"entries"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	return result
}

func createDir(t *testing.T, baseURL, wsID, path string) {
	reqBody := map[string]string{
		"workspaceId": wsID,
		"path":        path,
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	t.Logf("Creating directory: wsID=%s, path=%s, reqBody=%s", wsID, path, string(body))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/tools/fs_create_directory", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("createDir failed: status=%d, body=%s, url=%s", resp.StatusCode, string(bodyBytes), baseURL+"/api/tools/fs_create_directory")
	}
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func createFile(t *testing.T, baseURL, wsID, path, content string) {
	reqBody := map[string]string{
		"workspaceId": wsID,
		"path":        path,
		"content":     content,
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/tools/fs_write_file", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func createWorkspace(t *testing.T, baseURL, name string) string {
	reqBody := map[string]string{"name": name}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/tools/workspace_create", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result struct {
		WorkspaceID string `json:"workspaceId"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	return result.WorkspaceID
}

func intPtr(i int) *int {
	return &i
}
