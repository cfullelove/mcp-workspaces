package tool

import (
	"encoding/json"
	"mcp-workspace-manager/pkg/mcp"
	"mcp-workspace-manager/pkg/workspace"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestEnvironment creates a temporary workspace root and initializes the necessary managers and registries.
func setupTestEnvironment(t *testing.T) (*workspace.Manager, *Registry, string) {
	tmpDir, err := os.MkdirTemp("", "mcp-workspaces-test")
	require.NoError(t, err)

	manager, err := workspace.NewManager(tmpDir)
	require.NoError(t, err)

	registry := NewRegistry()
	RegisterWorkspaceTools(registry, manager)
	RegisterFSTools(registry, manager)

	return manager, registry, tmpDir
}

func TestWorkspaceCreate(t *testing.T) {
	_, registry, tmpDir := setupTestEnvironment(t)
	defer os.RemoveAll(tmpDir)

	// --- Test Request ---
	createReq := CreateWorkspaceRequest{Name: "My Test Workspace"}
	params, err := json.Marshal(createReq)
	require.NoError(t, err)

	req := &mcp.Request{
		ID:     json.RawMessage(`"1"`),
		Tool:   "workspace/create",
		Params: params,
	}

	// --- Dispatch and Assert ---
	resp := registry.Dispatch(req)
	require.Nil(t, resp.Error)
	require.NotNil(t, resp.Result)

	var createResp CreateWorkspaceResponse
	err = json.Unmarshal(resp.Result, &createResp)
	require.NoError(t, err)

	assert.Equal(t, "my-test-workspace", createResp.WorkspaceID)
	expectedPath := filepath.Join(tmpDir, "my-test-workspace")
	assert.Equal(t, expectedPath, createResp.Path)

	// Verify that the directory and git repo exist
	_, err = os.Stat(expectedPath)
	assert.NoError(t, err, "workspace directory should exist")

	_, err = git.PlainOpen(expectedPath)
	assert.NoError(t, err, ".git directory should exist")
}

func TestWriteAndReadFile(t *testing.T) {
	_, registry, tmpDir := setupTestEnvironment(t)
	defer os.RemoveAll(tmpDir)

	// First, create a workspace
	workspaceID := "test-ws"
	wsPath := filepath.Join(tmpDir, workspaceID)
	os.Mkdir(wsPath, 0755)
	_, err := git.PlainInit(wsPath, false)
	require.NoError(t, err)

	// --- Test Write File ---
	writeFileReq := WriteFileRequest{
		WorkspaceID: workspaceID,
		Path:        "hello.txt",
		Content:     "Hello, World!",
	}
	writeParams, err := json.Marshal(writeFileReq)
	require.NoError(t, err)

	writeReq := &mcp.Request{
		Tool:   "fs/write_file",
		Params: writeParams,
	}
	writeResp := registry.Dispatch(writeReq)
	require.Nil(t, writeResp.Error)

	var writeFileResp WriteFileResponse
	err = json.Unmarshal(writeResp.Result, &writeFileResp)
	require.NoError(t, err)

	assert.Equal(t, "hello.txt", writeFileResp.Path)
	assert.Equal(t, 13, writeFileResp.BytesWritten)
	assert.NotEmpty(t, writeFileResp.Commit)

	// --- Test Read File ---
	readFileReq := ReadFileRequest{
		WorkspaceID: workspaceID,
		Path:        "hello.txt",
	}
	readParams, err := json.Marshal(readFileReq)
	require.NoError(t, err)

	readReq := &mcp.Request{
		Tool:   "fs/read_text_file",
		Params: readParams,
	}
	readResp := registry.Dispatch(readReq)
	require.Nil(t, readResp.Error)

	var readFileResp ReadFileResponse
	err = json.Unmarshal(readResp.Result, &readFileResp)
	require.NoError(t, err)

	assert.Equal(t, "Hello, World!", readFileResp.Content)
}

func TestCreateListAndGetInfo(t *testing.T) {
	_, registry, tmpDir := setupTestEnvironment(t)
	defer os.RemoveAll(tmpDir)

	// First, create a workspace
	workspaceID := "test-ws"
	wsPath := filepath.Join(tmpDir, workspaceID)
	os.Mkdir(wsPath, 0755)
	_, err := git.PlainInit(wsPath, false)
	require.NoError(t, err)

	// --- Test Create Directory ---
	createDirReq := CreateDirectoryRequest{
		WorkspaceID: workspaceID,
		Path:        "new-dir/subdir",
	}
	createDirParams, err := json.Marshal(createDirReq)
	require.NoError(t, err)

	createDirResp := registry.Dispatch(&mcp.Request{Tool: "fs/create_directory", Params: createDirParams})
	require.Nil(t, createDirResp.Error)

	var createDirJSON CreateDirectoryResponse
	err = json.Unmarshal(createDirResp.Result, &createDirJSON)
	require.NoError(t, err)
	assert.True(t, createDirJSON.Created)
	assert.NotEmpty(t, createDirJSON.Commit)
	assert.DirExists(t, filepath.Join(wsPath, "new-dir/subdir"))

	// --- Test List Directory ---
	listDirReq := ListDirectoryRequest{
		WorkspaceID: workspaceID,
		Path:        "new-dir",
	}
	listDirParams, err := json.Marshal(listDirReq)
	require.NoError(t, err)

	listDirResp := registry.Dispatch(&mcp.Request{Tool: "fs/list_directory", Params: listDirParams})
	require.Nil(t, listDirResp.Error)

	var listDirJSON ListDirectoryResponse
	err = json.Unmarshal(listDirResp.Result, &listDirJSON)
	require.NoError(t, err)
	assert.Contains(t, listDirJSON.Entries, "[DIR]  subdir")

	// --- Test Get File Info ---
	getFileInfoReq := GetFileInfoRequest{
		WorkspaceID: workspaceID,
		Path:        "new-dir/subdir",
	}
	getFileInfoParams, err := json.Marshal(getFileInfoReq)
	require.NoError(t, err)

	getFileInfoResp := registry.Dispatch(&mcp.Request{Tool: "fs/get_file_info", Params: getFileInfoParams})
	require.Nil(t, getFileInfoResp.Error)

	var getFileInfoJSON GetFileInfoResponse
	err = json.Unmarshal(getFileInfoResp.Result, &getFileInfoJSON)
	require.NoError(t, err)

	assert.Equal(t, "directory", getFileInfoJSON.Type)
	assert.NotEmpty(t, getFileInfoJSON.Mtime)
}

func TestMoveFileAndGetHistory(t *testing.T) {
	_, registry, tmpDir := setupTestEnvironment(t)
	defer os.RemoveAll(tmpDir)

	// Setup: Create a workspace and a file
	workspaceID := "test-ws"
	wsPath := filepath.Join(tmpDir, workspaceID)
	os.Mkdir(wsPath, 0755)
	_, err := git.PlainInit(wsPath, false)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(wsPath, "original.txt"), []byte("move me"), 0644)
	require.NoError(t, err)

	// --- Test Move File ---
	moveReq := MoveFileRequest{
		WorkspaceID: workspaceID,
		Source:      "original.txt",
		Destination: "moved.txt",
	}
	moveParams, err := json.Marshal(moveReq)
	require.NoError(t, err)

	moveResp := registry.Dispatch(&mcp.Request{Tool: "fs/move_file", Params: moveParams})
	require.Nil(t, moveResp.Error)

	var moveJSON MoveFileResponse
	err = json.Unmarshal(moveResp.Result, &moveJSON)
	require.NoError(t, err)

	assert.NotEmpty(t, moveJSON.Commit)
	assert.FileExists(t, filepath.Join(wsPath, "moved.txt"))
	assert.NoFileExists(t, filepath.Join(wsPath, "original.txt"))

	// --- Test Get Commit History ---
	historyReq := GetCommitHistoryRequest{WorkspaceID: workspaceID}
	historyParams, err := json.Marshal(historyReq)
	require.NoError(t, err)

	historyResp := registry.Dispatch(&mcp.Request{Tool: "fs/get_commit_history", Params: historyParams})
	require.Nil(t, historyResp.Error)

	var historyJSON GetCommitHistoryResponse
	err = json.Unmarshal(historyResp.Result, &historyJSON)
	require.NoError(t, err)

	// We expect 2 commits: the initial one from setup and the move commit.
	// Note: The test setup doesn't create an initial commit, so we might only have 1.
	// Let's check for at least one meaningful commit.
	assert.GreaterOrEqual(t, len(historyJSON.Log), 1)
	assert.Contains(t, historyJSON.Log[0].Message, "mcp/fs/move_file: Move original.txt to moved.txt")
}

func TestEditFile(t *testing.T) {
	manager, registry, tmpDir := setupTestEnvironment(t)
	defer os.RemoveAll(tmpDir)

	// Setup: Create a workspace and a file
	workspaceID := "edit-ws"
	wsPath := filepath.Join(tmpDir, workspaceID)
	os.Mkdir(wsPath, 0755)
	_, err := git.PlainInit(wsPath, false)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(wsPath, "config.txt"), []byte("version=1.0\nhost=localhost"), 0644)
	require.NoError(t, err)
	_, err = manager.Commit(workspaceID, "Initial config", "test")
	require.NoError(t, err)

	// --- Test Dry Run ---
	editDryRunReq := EditFileRequest{
		WorkspaceID: workspaceID,
		Path:        "config.txt",
		Edits:       []Edit{{OldText: "localhost", NewText: "example.com"}},
		DryRun:      true,
	}
	dryRunParams, err := json.Marshal(editDryRunReq)
	require.NoError(t, err)

	dryRunResp := registry.Dispatch(&mcp.Request{Tool: "fs/edit_file", Params: dryRunParams})
	require.Nil(t, dryRunResp.Error)

	var dryRunJSON EditFileDryRunResponse
	err = json.Unmarshal(dryRunResp.Result, &dryRunJSON)
	require.NoError(t, err)
	assert.True(t, dryRunJSON.DryRun)
	assert.Equal(t, 1, dryRunJSON.Matches)

	// --- Test Apply ---
	editReq := EditFileRequest{
		WorkspaceID: workspaceID,
		Path:        "config.txt",
		Edits:       []Edit{{OldText: "version=1.0", NewText: "version=1.1"}},
		DryRun:      false,
	}
	applyParams, err := json.Marshal(editReq)
	require.NoError(t, err)

	applyResp := registry.Dispatch(&mcp.Request{Tool: "fs/edit_file", Params: applyParams})
	require.Nil(t, applyResp.Error)

	var applyJSON EditFileResponse
	err = json.Unmarshal(applyResp.Result, &applyJSON)
	require.NoError(t, err)
	assert.False(t, applyJSON.DryRun)
	assert.NotEmpty(t, applyJSON.Commit)

	content, err := os.ReadFile(filepath.Join(wsPath, "config.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "version=1.1")
}

func TestSearchAndTree(t *testing.T) {
	manager, registry, tmpDir := setupTestEnvironment(t)
	defer os.RemoveAll(tmpDir)

	// Setup
	workspaceID := "search-ws"
	wsPath := filepath.Join(tmpDir, workspaceID)
	os.MkdirAll(filepath.Join(wsPath, "src/app"), 0755)
	os.MkdirAll(filepath.Join(wsPath, "data"), 0755)
	os.WriteFile(filepath.Join(wsPath, "src/app/main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(wsPath, "src/app/main_test.go"), []byte("package main_test"), 0644)
	os.WriteFile(filepath.Join(wsPath, "README.md"), []byte("# Test"), 0644)
	_, err := git.PlainInit(wsPath, false)
	require.NoError(t, err)
	_, err = manager.Commit(workspaceID, "Initial project structure", "test")
	require.NoError(t, err)

	// --- Test Search ---
	searchReq := SearchFilesRequest{
		WorkspaceID:     workspaceID,
		Path:            "src",
		Pattern:         "*.go",
		ExcludePatterns: []string{"*_test.go"},
	}
	searchParams, err := json.Marshal(searchReq)
	require.NoError(t, err)

	searchResp := registry.Dispatch(&mcp.Request{Tool: "fs/search_files", Params: searchParams})
	require.Nil(t, searchResp.Error)

	var searchJSON SearchFilesResponse
	err = json.Unmarshal(searchResp.Result, &searchJSON)
	require.NoError(t, err)
	require.Len(t, searchJSON.Matches, 1)
	// The path should be relative to the workspace root, not include the workspace ID.
	assert.Equal(t, filepath.ToSlash(filepath.Join("src", "app", "main.go")), searchJSON.Matches[0])

	// --- Test Directory Tree ---
	treeReq := DirectoryTreeRequest{
		WorkspaceID:     workspaceID,
		Path:            ".",
		ExcludePatterns: []string{".git"},
	}
	treeParams, err := json.Marshal(treeReq)
	require.NoError(t, err)

	treeResp := registry.Dispatch(&mcp.Request{Tool: "fs/directory_tree", Params: treeParams})
	require.Nil(t, treeResp.Error)

	var treeJSON DirectoryTreeResponse
	err = json.Unmarshal(treeResp.Result, &treeJSON)
	require.NoError(t, err)
	assert.Len(t, treeJSON.Tree, 3) // src, data, README.md
}