package mcpsdk

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"mcp-workspace-manager/pkg/workspace"
)

var toolNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func newTool(name, description string) *sdkmcp.Tool {
	if !toolNameRegex.MatchString(name) {
		panic(fmt.Errorf("invalid tool name: %s (must match ^[a-zA-Z0-9_-]+$)", name))
	}
	return &sdkmcp.Tool{Name: name, Description: description}
}

// ===== Workspace tool types =====

type CreateWorkspaceRequest struct {
	Name string `json:"name"`
}

type CreateWorkspaceResponse struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}

// ===== FS tool types =====

type WriteFileRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
	Content     string `json:"content"`
}
type WriteFileResponse struct {
	Path         string `json:"path"`
	BytesWritten int    `json:"bytesWritten"`
	Overwritten  bool   `json:"overwritten"`
	Commit       string `json:"commit"`
}

type ReadFileRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
	Head        *int   `json:"head,omitempty"`
	Tail        *int   `json:"tail,omitempty"`
}
type ReadFileResponse struct {
	Content    string `json:"content"`
	TotalLines int    `json:"totalLines,omitempty"`
	Head       *int   `json:"head,omitempty"`
	Tail       *int   `json:"tail,omitempty"`
}

type CreateDirectoryRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}
type CreateDirectoryResponse struct {
	Path    string `json:"path"`
	Created bool   `json:"created"`
	Commit  string `json:"commit"`
}

type ListDirectoryRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}
type ListDirectoryResponse struct {
	Entries []string `json:"entries"`
}

type GetFileInfoRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}
type GetFileInfoResponse struct {
	Size        int64  `json:"size"`
	Mtime       string `json:"mtime"`
	Type        string `json:"type"`
	Permissions string `json:"permissions"`
}

type GetCommitHistoryRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path,omitempty"`
	Limit       int    `json:"limit,omitempty"`
}
type CommitLog struct {
	Commit  string `json:"commit"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Message string `json:"message"`
}
type GetCommitHistoryResponse struct {
	Log []CommitLog `json:"log"`
}

type MoveFileRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
}
type MoveFileResponse struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Commit      string `json:"commit"`
}

type Edit struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}
type EditFileRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
	Edits       []Edit `json:"edits"`
	DryRun      bool   `json:"dryRun"`
}
type EditFileDryRunResponse struct {
	DryRun  bool   `json:"dryRun"`
	Diff    string `json:"diff"`
	Matches int    `json:"matches"`
}
type EditFileResponse struct {
	DryRun       bool   `json:"dryRun"`
	Path         string `json:"path"`
	Changes      int    `json:"changes"`
	BytesWritten int    `json:"bytesWritten"`
	Commit       string `json:"commit"`
}

type ReadMultipleFilesRequest struct {
	WorkspaceID string   `json:"workspaceId"`
	Paths       []string `json:"paths"`
}
type FileReadResult struct {
	Path    string  `json:"path"`
	OK      bool    `json:"ok"`
	Content *string `json:"content,omitempty"`
	Error   *string `json:"error,omitempty"`
}
type ReadMultipleFilesResponse struct {
	Results []FileReadResult `json:"results"`
}

type ListDirectoryWithSizesRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
	SortBy      string `json:"sortBy,omitempty"` // "name" or "size"
}
type EntryInfo struct {
	Name string `json:"name"`
	Type string `json:"type"` // "file" or "directory"
	Size int64  `json:"size"`
}
type TotalsInfo struct {
	Files        int   `json:"files"`
	Directories  int   `json:"directories"`
	CombinedSize int64 `json:"combinedSize"`
}
type ListDirectoryWithSizesResponse struct {
	Entries []EntryInfo `json:"entries"`
	Totals  TotalsInfo  `json:"totals"`
}

type SearchFilesRequest struct {
	WorkspaceID     string   `json:"workspaceId"`
	Path            string   `json:"path"`
	Pattern         string   `json:"pattern"`
	ExcludePatterns []string `json:"excludePatterns,omitempty"`
}
type SearchFilesResponse struct {
	Matches []string `json:"matches"`
}

type DirectoryTreeRequest struct {
	WorkspaceID     string   `json:"workspaceId"`
	Path            string   `json:"path"`
	ExcludePatterns []string `json:"excludePatterns,omitempty"`
}
type TreeNode struct {
	Name     string     `json:"name"`
	Type     string     `json:"type"`
	Children *[]TreeNode `json:"children,omitempty"`
}
type DirectoryTreeResponse struct {
	Tree []TreeNode `json:"tree"`
}

type ReadMediaFileRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}
type ReadMediaFileResponse struct {
	MimeType string `json:"mimeType"`
	Base64   string `json:"base64"`
	Size     int64  `json:"size"`
}

// buildServer constructs an MCP SDK server and registers tools using typed handlers.
// Each tool delegates to a shared implementation in tools.go so both MCP and REST share logic.
func buildServer(wm *workspace.Manager) *sdkmcp.Server {
	impl := &sdkmcp.Implementation{
		Name:    "mcp-workspace-manager",
		Version: "0.1.0",
	}
	server := sdkmcp.NewServer(impl, nil)

	// workspace/create
	sdkmcp.AddTool[CreateWorkspaceRequest, CreateWorkspaceResponse](
		server,
		newTool("workspace_create", "Create a workspace directory under the configured root and initialize git"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, input CreateWorkspaceRequest) (*sdkmcp.CallToolResult, CreateWorkspaceResponse, error) {
			out, err := WorkspaceCreate(ctx, wm, input)
			if err != nil {
				return nil, CreateWorkspaceResponse{}, err
			}
			return nil, out, nil
		},
	)

	// fs/write_file
	sdkmcp.AddTool[WriteFileRequest, WriteFileResponse](server, newTool("fs_write_file", "Write a text file"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a WriteFileRequest) (*sdkmcp.CallToolResult, WriteFileResponse, error) {
			out, err := FSWriteFile(ctx, wm, a)
			if err != nil {
				return nil, WriteFileResponse{}, err
			}
			return nil, out, nil
		},
	)

	// fs/read_text_file
	sdkmcp.AddTool[ReadFileRequest, ReadFileResponse](server, newTool("fs_read_text_file", "Read a UTF-8 text file"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a ReadFileRequest) (*sdkmcp.CallToolResult, ReadFileResponse, error) {
			out, err := FSReadTextFile(ctx, wm, a)
			if err != nil {
				return nil, ReadFileResponse{}, err
			}
			return nil, out, nil
		},
	)

	// fs/create_directory
	sdkmcp.AddTool[CreateDirectoryRequest, CreateDirectoryResponse](server, newTool("fs_create_directory", "Create a directory (idempotent)"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a CreateDirectoryRequest) (*sdkmcp.CallToolResult, CreateDirectoryResponse, error) {
			out, err := FSCreateDirectory(ctx, wm, a)
			if err != nil {
				return nil, CreateDirectoryResponse{}, err
			}
			return nil, out, nil
		},
	)

	// fs/list_directory
	sdkmcp.AddTool[ListDirectoryRequest, ListDirectoryResponse](server, newTool("fs_list_directory", "List directory entries"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a ListDirectoryRequest) (*sdkmcp.CallToolResult, ListDirectoryResponse, error) {
			out, err := FSListDirectory(ctx, wm, a)
			if err != nil {
				return nil, ListDirectoryResponse{}, err
			}
			return nil, out, nil
		},
	)

	// fs/get_file_info
	sdkmcp.AddTool[GetFileInfoRequest, GetFileInfoResponse](server, newTool("fs_get_file_info", "Get file or directory metadata"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a GetFileInfoRequest) (*sdkmcp.CallToolResult, GetFileInfoResponse, error) {
			out, err := FSGetFileInfo(ctx, wm, a)
			if err != nil {
				return nil, GetFileInfoResponse{}, err
			}
			return nil, out, nil
		},
	)

	// fs/get_commit_history (workspace-scoped)
	sdkmcp.AddTool[GetCommitHistoryRequest, GetCommitHistoryResponse](server, newTool("fs_get_commit_history", "Get git commit history"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a GetCommitHistoryRequest) (*sdkmcp.CallToolResult, GetCommitHistoryResponse, error) {
			out, err := FSGetCommitHistory(ctx, wm, a)
			if err != nil {
				return nil, GetCommitHistoryResponse{}, err
			}
			return nil, out, nil
		},
	)

	// fs/move_file
	sdkmcp.AddTool[MoveFileRequest, MoveFileResponse](server, newTool("fs_move_file", "Move or rename a file/directory"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a MoveFileRequest) (*sdkmcp.CallToolResult, MoveFileResponse, error) {
			out, err := FSMoveFile(ctx, wm, a)
			if err != nil {
				return nil, MoveFileResponse{}, err
			}
			return nil, out, nil
		},
	)

	// fs/edit_file
	sdkmcp.AddTool[EditFileRequest, any](server, newTool("fs_edit_file", "Apply substring edits to a file"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a EditFileRequest) (*sdkmcp.CallToolResult, any, error) {
			out, err := FSEditFile(ctx, wm, a)
			if err != nil {
				return nil, nil, err
			}
			return nil, out, nil
		},
	)

	// fs/read_multiple_files
	sdkmcp.AddTool[ReadMultipleFilesRequest, ReadMultipleFilesResponse](server, newTool("fs_read_multiple_files", "Read multiple files concurrently"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a ReadMultipleFilesRequest) (*sdkmcp.CallToolResult, ReadMultipleFilesResponse, error) {
			out, err := FSReadMultipleFiles(ctx, wm, a)
			if err != nil {
				return nil, ReadMultipleFilesResponse{}, err
			}
			return nil, out, nil
		},
	)

	// fs/list_directory_with_sizes
	sdkmcp.AddTool[ListDirectoryWithSizesRequest, ListDirectoryWithSizesResponse](server, newTool("fs_list_directory_with_sizes", "List directory entries with sizes"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a ListDirectoryWithSizesRequest) (*sdkmcp.CallToolResult, ListDirectoryWithSizesResponse, error) {
			out, err := FSListDirectoryWithSizes(ctx, wm, a)
			if err != nil {
				return nil, ListDirectoryWithSizesResponse{}, err
			}
			return nil, out, nil
		},
	)

	// fs/search_files (name-glob-based per PRD prototype)
	sdkmcp.AddTool[SearchFilesRequest, SearchFilesResponse](server, newTool("fs_search_files", "Search files by glob pattern"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a SearchFilesRequest) (*sdkmcp.CallToolResult, SearchFilesResponse, error) {
			out, err := FSSearchFiles(ctx, wm, a)
			if err != nil {
				return nil, SearchFilesResponse{}, err
			}
			return nil, out, nil
		},
	)

	// fs/directory_tree
	// Note: Use 'any' for output to avoid schema inference on recursive types.
	sdkmcp.AddTool[DirectoryTreeRequest, any](server, newTool("fs_directory_tree", "Return a JSON directory tree"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a DirectoryTreeRequest) (*sdkmcp.CallToolResult, any, error) {
			out, err := FSDirectoryTree(ctx, wm, a)
			if err != nil {
				return nil, nil, err
			}
			return nil, out, nil
		},
	)

	// fs/read_media_file
	sdkmcp.AddTool[ReadMediaFileRequest, ReadMediaFileResponse](server, newTool("fs_read_media_file", "Read media file and return base64 + MIME"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a ReadMediaFileRequest) (*sdkmcp.CallToolResult, ReadMediaFileResponse, error) {
			out, err := FSReadMediaFile(ctx, wm, a)
			if err != nil {
				return nil, ReadMediaFileResponse{}, err
			}
			return nil, out, nil
		},
	)

	return server
}

// buildTree builds the directory tree respecting simple exclude patterns (name-match).
func buildTree(root string, excludePatterns []string) ([]TreeNode, error) {
	var tree []TreeNode
	files, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		// Exclude by name
		isExcluded := false
		for _, pattern := range excludePatterns {
			match, err := filepath.Match(pattern, f.Name())
			if err != nil {
				return nil, err
			}
			if match {
				isExcluded = true
				break
			}
		}
		if isExcluded {
			continue
		}
		node := TreeNode{Name: f.Name()}
		if f.IsDir() {
			node.Type = "directory"
			children, err := buildTree(filepath.Join(root, f.Name()), excludePatterns)
			if err != nil {
				return nil, err
			}
			node.Children = &children
		} else {
			node.Type = "file"
		}
		tree = append(tree, node)
	}
	return tree, nil
}

// RunStdio starts the MCP SDK server over stdio until the client disconnects or context is cancelled.
func RunStdio(wm *workspace.Manager) {
	server := buildServer(wm)
	if err := server.Run(context.Background(), &sdkmcp.StdioTransport{}); err != nil && err != io.EOF {
		slog.Error("MCP SDK stdio server exited with error", "error", err)
	}
}
