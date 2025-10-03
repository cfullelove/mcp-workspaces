package mcpsdk

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sergi/go-diff/diffmatchpatch"

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
			if input.Name == "" {
				return nil, CreateWorkspaceResponse{}, fmt.Errorf("invalid input: 'name' is required")
			}
			id, path, err := wm.Create(input.Name)
			if err != nil {
				return nil, CreateWorkspaceResponse{}, err
			}
			return nil, CreateWorkspaceResponse{WorkspaceID: id, Path: path}, nil
		},
	)

	// fs/write_file
	sdkmcp.AddTool[WriteFileRequest, WriteFileResponse](server, newTool("fs_write_file", "Write a text file"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a WriteFileRequest) (*sdkmcp.CallToolResult, WriteFileResponse, error) {
			if a.WorkspaceID == "" || a.Path == "" {
				return nil, WriteFileResponse{}, fmt.Errorf("INVALID_INPUT: 'workspaceId' and 'path' are required")
			}
			absPath, err := wm.SafePath(a.WorkspaceID, a.Path)
			if err != nil {
				return nil, WriteFileResponse{}, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
			}
			_, statErr := os.Stat(absPath)
			overwritten := !os.IsNotExist(statErr)

			if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
				return nil, WriteFileResponse{}, fmt.Errorf("INTERNAL: failed to create parent directories: %v", err)
			}
			contentBytes := []byte(a.Content)
			if err := os.WriteFile(absPath, contentBytes, 0644); err != nil {
				return nil, WriteFileResponse{}, fmt.Errorf("INTERNAL: failed to write file: %v", err)
			}
			commit, err := wm.Commit(a.WorkspaceID, fmt.Sprintf("mcp/fs_write_file: Write %s", a.Path), "mcp-client")
			if err != nil {
				return nil, WriteFileResponse{}, fmt.Errorf("INTERNAL: failed to commit changes: %v", err)
			}
			return nil, WriteFileResponse{Path: a.Path, BytesWritten: len(contentBytes), Overwritten: overwritten, Commit: commit}, nil
		},
	)

	// fs/read_text_file
	sdkmcp.AddTool[ReadFileRequest, ReadFileResponse](server, newTool("fs_read_text_file", "Read a UTF-8 text file"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a ReadFileRequest) (*sdkmcp.CallToolResult, ReadFileResponse, error) {
			if a.WorkspaceID == "" || a.Path == "" {
				return nil, ReadFileResponse{}, fmt.Errorf("INVALID_INPUT: 'workspaceId' and 'path' are required")
			}
			if a.Head != nil && a.Tail != nil {
				return nil, ReadFileResponse{}, fmt.Errorf("INVALID_INPUT: cannot specify both 'head' and 'tail'")
			}
			absPath, err := wm.SafePath(a.WorkspaceID, a.Path)
			if err != nil {
				return nil, ReadFileResponse{}, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
			}
			contentBytes, err := os.ReadFile(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					return nil, ReadFileResponse{}, fmt.Errorf("NOT_FOUND: file not found")
				}
				return nil, ReadFileResponse{}, fmt.Errorf("INTERNAL: failed to read file: %v", err)
			}
			content := string(contentBytes)
			lines := strings.Split(content, "\n")
			total := len(lines)
			resp := ReadFileResponse{TotalLines: total}
			if a.Head != nil {
				h := *a.Head
				if h > total {
					h = total
				}
				resp.Content = strings.Join(lines[:h], "\n")
				resp.Head = &h
			} else if a.Tail != nil {
				t := *a.Tail
				if t > total {
					t = total
				}
				resp.Content = strings.Join(lines[total-t:], "\n")
				resp.Tail = &t
			} else {
				resp.Content = content
			}
			return nil, resp, nil
		},
	)

	// fs/create_directory
	sdkmcp.AddTool[CreateDirectoryRequest, CreateDirectoryResponse](server, newTool("fs_create_directory", "Create a directory (idempotent)"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a CreateDirectoryRequest) (*sdkmcp.CallToolResult, CreateDirectoryResponse, error) {
			absPath, err := wm.SafePath(a.WorkspaceID, a.Path)
			if err != nil {
				return nil, CreateDirectoryResponse{}, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
			}
			_, statErr := os.Stat(absPath)
			created := os.IsNotExist(statErr)
			if err := os.MkdirAll(absPath, 0755); err != nil {
				return nil, CreateDirectoryResponse{}, fmt.Errorf("INTERNAL: failed to create directory: %v", err)
			}
			// Ensure tracking empty folders
			gk := filepath.Join(absPath, ".gitkeep")
			if _, err := os.Stat(gk); os.IsNotExist(err) {
				if f, e := os.Create(gk); e == nil {
					f.Close()
				}
			}
			commit, err := wm.Commit(a.WorkspaceID, fmt.Sprintf("mcp/fs_create_directory: Create %s", a.Path), "mcp-client")
			if err != nil {
				return nil, CreateDirectoryResponse{}, fmt.Errorf("INTERNAL: failed to commit changes: %v", err)
			}
			return nil, CreateDirectoryResponse{Path: a.Path, Created: created, Commit: commit}, nil
		},
	)

	// fs/list_directory
	sdkmcp.AddTool[ListDirectoryRequest, ListDirectoryResponse](server, newTool("fs_list_directory", "List directory entries"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a ListDirectoryRequest) (*sdkmcp.CallToolResult, ListDirectoryResponse, error) {
			absPath, err := wm.SafePath(a.WorkspaceID, a.Path)
			if err != nil {
				return nil, ListDirectoryResponse{}, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
			}
			files, err := os.ReadDir(absPath)
			if err != nil {
				return nil, ListDirectoryResponse{}, fmt.Errorf("INTERNAL: failed to list directory: %v", err)
			}
			var entries []string
			for _, f := range files {
				prefix := "[FILE]"
				if f.IsDir() {
					prefix = "[DIR]"
				}
				entries = append(entries, prefix+" "+f.Name())
			}
			return nil, ListDirectoryResponse{Entries: entries}, nil
		},
	)

	// fs/get_file_info
	sdkmcp.AddTool[GetFileInfoRequest, GetFileInfoResponse](server, newTool("fs_get_file_info", "Get file or directory metadata"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a GetFileInfoRequest) (*sdkmcp.CallToolResult, GetFileInfoResponse, error) {
			absPath, err := wm.SafePath(a.WorkspaceID, a.Path)
			if err != nil {
				return nil, GetFileInfoResponse{}, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
			}
			info, err := os.Stat(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					return nil, GetFileInfoResponse{}, fmt.Errorf("NOT_FOUND: file or directory not found")
				}
				return nil, GetFileInfoResponse{}, fmt.Errorf("INTERNAL: failed to get file info: %v", err)
			}
			ftype := "file"
			if info.IsDir() {
				ftype = "directory"
			}
			out := GetFileInfoResponse{
				Size:        info.Size(),
				Mtime:       info.ModTime().UTC().Format(time.RFC3339),
				Type:        ftype,
				Permissions: info.Mode().String(),
			}
			return nil, out, nil
		},
	)

	// fs/get_commit_history (workspace-scoped; ignores path for now to match current implementation)
	sdkmcp.AddTool[GetCommitHistoryRequest, GetCommitHistoryResponse](server, newTool("fs_get_commit_history", "Get git commit history"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a GetCommitHistoryRequest) (*sdkmcp.CallToolResult, GetCommitHistoryResponse, error) {
			if a.WorkspaceID == "" {
				return nil, GetCommitHistoryResponse{}, fmt.Errorf("INVALID_INPUT: 'workspaceId' is required")
			}
			limit := 20
			if a.Limit > 0 {
				limit = a.Limit
			}
			commits, err := wm.GetCommitHistory(a.WorkspaceID, limit)
			if err != nil {
				return nil, GetCommitHistoryResponse{}, fmt.Errorf("INTERNAL: failed to get commit history: %v", err)
			}
			var log []CommitLog
			for _, c := range commits {
				log = append(log, CommitLog{
					Commit:  c.Hash.String(),
					Author:  c.Author.String(),
					Date:    c.Author.When.UTC().Format(time.RFC3339),
					Message: c.Message,
				})
			}
			return nil, GetCommitHistoryResponse{Log: log}, nil
		},
	)

	// fs/move_file
	sdkmcp.AddTool[MoveFileRequest, MoveFileResponse](server, newTool("fs_move_file", "Move or rename a file/directory"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a MoveFileRequest) (*sdkmcp.CallToolResult, MoveFileResponse, error) {
			src, err := wm.SafePath(a.WorkspaceID, a.Source)
			if err != nil {
				return nil, MoveFileResponse{}, fmt.Errorf("OUT_OF_BOUNDS: source path invalid: %v", err)
			}
			dst, err := wm.SafePath(a.WorkspaceID, a.Destination)
			if err != nil {
				return nil, MoveFileResponse{}, fmt.Errorf("OUT_OF_BOUNDS: destination path invalid: %v", err)
			}
			if _, err := os.Stat(dst); !os.IsNotExist(err) {
				return nil, MoveFileResponse{}, fmt.Errorf("ALREADY_EXISTS: destination exists")
			}
			if err := os.Rename(src, dst); err != nil {
				return nil, MoveFileResponse{}, fmt.Errorf("INTERNAL: move failed: %v", err)
			}
			commit, err := wm.Commit(a.WorkspaceID, fmt.Sprintf("mcp/fs_move_file: Move %s to %s", a.Source, a.Destination), "mcp-client")
			if err != nil {
				return nil, MoveFileResponse{}, fmt.Errorf("INTERNAL: commit failed: %v", err)
			}
			return nil, MoveFileResponse{Source: a.Source, Destination: a.Destination, Commit: commit}, nil
		},
	)

	// fs/edit_file
	sdkmcp.AddTool[EditFileRequest, any](server, newTool("fs_edit_file", "Apply substring edits to a file"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a EditFileRequest) (*sdkmcp.CallToolResult, any, error) {
			if a.WorkspaceID == "" || a.Path == "" || len(a.Edits) == 0 {
				return nil, nil, fmt.Errorf("INVALID_INPUT: 'workspaceId', 'path', and 'edits' are required")
			}
			absPath, err := wm.SafePath(a.WorkspaceID, a.Path)
			if err != nil {
				return nil, nil, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
			}
			orig, err := os.ReadFile(absPath)
			if err != nil {
				return nil, nil, fmt.Errorf("NOT_FOUND: file not found")
			}
			newContent := string(orig)
			matches := 0
			for _, e := range a.Edits {
				matches += strings.Count(newContent, e.OldText)
				newContent = strings.ReplaceAll(newContent, e.OldText, e.NewText)
			}

			if a.DryRun {
				dmp := diffmatchpatch.New()
				diffs := dmp.DiffMain(string(orig), newContent, true)
				out := EditFileDryRunResponse{DryRun: true, Diff: dmp.DiffPrettyText(diffs), Matches: matches}
				return nil, out, nil
			}

			contentBytes := []byte(newContent)
			if err := os.WriteFile(absPath, contentBytes, 0644); err != nil {
				return nil, nil, fmt.Errorf("INTERNAL: failed to write edited file: %v", err)
			}
			commit, err := wm.Commit(a.WorkspaceID, fmt.Sprintf("mcp/fs_edit_file: Edit %s", a.Path), "mcp-client")
			if err != nil {
				return nil, nil, fmt.Errorf("INTERNAL: failed to commit changes: %v", err)
			}
			out := EditFileResponse{DryRun: false, Path: a.Path, Changes: len(a.Edits), BytesWritten: len(contentBytes), Commit: commit}
			return nil, out, nil
		},
	)

	// fs/read_multiple_files
	sdkmcp.AddTool[ReadMultipleFilesRequest, ReadMultipleFilesResponse](server, newTool("fs_read_multiple_files", "Read multiple files concurrently"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a ReadMultipleFilesRequest) (*sdkmcp.CallToolResult, ReadMultipleFilesResponse, error) {
			if a.WorkspaceID == "" || len(a.Paths) == 0 {
				return nil, ReadMultipleFilesResponse{}, fmt.Errorf("INVALID_INPUT: 'workspaceId' and 'paths' are required")
			}
			numJobs := len(a.Paths)
			jobs := make(chan string, numJobs)
			results := make(chan FileReadResult, numJobs)
			numWorkers := 4
			if numJobs < numWorkers {
				numWorkers = numJobs
			}
			for w := 0; w < numWorkers; w++ {
				go func() {
					for p := range jobs {
						abs, err := wm.SafePath(a.WorkspaceID, p)
						if err != nil {
							errStr := err.Error()
							results <- FileReadResult{Path: p, OK: false, Error: &errStr}
							continue
						}
						contentBytes, err := os.ReadFile(abs)
						if err != nil {
							errStr := err.Error()
							results <- FileReadResult{Path: p, OK: false, Error: &errStr}
							continue
						}
						content := string(contentBytes)
						results <- FileReadResult{Path: p, OK: true, Content: &content}
					}
				}()
			}
			for _, p := range a.Paths {
				jobs <- p
			}
			close(jobs)
			var out []FileReadResult
			for i := 0; i < numJobs; i++ {
				out = append(out, <-results)
			}
			return nil, ReadMultipleFilesResponse{Results: out}, nil
		},
	)

	// fs/list_directory_with_sizes
	sdkmcp.AddTool[ListDirectoryWithSizesRequest, ListDirectoryWithSizesResponse](server, newTool("fs_list_directory_with_sizes", "List directory entries with sizes"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a ListDirectoryWithSizesRequest) (*sdkmcp.CallToolResult, ListDirectoryWithSizesResponse, error) {
			abs, err := wm.SafePath(a.WorkspaceID, a.Path)
			if err != nil {
				return nil, ListDirectoryWithSizesResponse{}, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
			}
			files, err := os.ReadDir(abs)
			if err != nil {
				return nil, ListDirectoryWithSizesResponse{}, fmt.Errorf("INTERNAL: failed to list directory: %v", err)
			}
			var entries []EntryInfo
			var totals TotalsInfo
			for _, f := range files {
				info, err := f.Info()
				if err != nil {
					continue
				}
				e := EntryInfo{Name: f.Name()}
				if f.IsDir() {
					e.Type = "directory"
					e.Size = 0
					totals.Directories++
				} else {
					e.Type = "file"
					e.Size = info.Size()
					totals.Files++
					totals.CombinedSize += info.Size()
				}
				entries = append(entries, e)
			}
			if a.SortBy == "size" {
				sort.Slice(entries, func(i, j int) bool { return entries[i].Size > entries[j].Size })
			} else {
				sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
			}
			return nil, ListDirectoryWithSizesResponse{Entries: entries, Totals: totals}, nil
		},
	)

	// fs/search_files (name-glob-based per PRD prototype)
	sdkmcp.AddTool[SearchFilesRequest, SearchFilesResponse](server, newTool("fs_search_files", "Search files by glob pattern"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a SearchFilesRequest) (*sdkmcp.CallToolResult, SearchFilesResponse, error) {
			if a.WorkspaceID == "" || a.Pattern == "" {
				return nil, SearchFilesResponse{}, fmt.Errorf("INVALID_INPUT: 'workspaceId' and 'pattern' are required")
			}
			start, err := wm.SafePath(a.WorkspaceID, a.Path)
			if err != nil {
				return nil, SearchFilesResponse{}, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
			}
			var matches []string
			err = filepath.WalkDir(start, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				// Check main pattern against filename
				matched, err := filepath.Match(a.Pattern, d.Name())
				if err != nil {
					return err
				}
				if matched {
					// Apply excludes
					excluded := false
					for _, ex := range a.ExcludePatterns {
						ok, err := filepath.Match(ex, d.Name())
						if err != nil {
							return err
						}
						if ok {
							excluded = true
							break
						}
					}
					if !excluded {
						wsRoot, err := wm.SafePath(a.WorkspaceID, ".")
						if err != nil {
							return err
						}
						if rel, err := filepath.Rel(wsRoot, path); err == nil {
							matches = append(matches, rel)
						}
					}
				}
				return nil
			})
			if err != nil {
				return nil, SearchFilesResponse{}, fmt.Errorf("INTERNAL: search failed: %v", err)
			}
			return nil, SearchFilesResponse{Matches: matches}, nil
		},
	)

	// fs/directory_tree
	// Note: Use 'any' for output to avoid schema inference on recursive types.
	sdkmcp.AddTool[DirectoryTreeRequest, any](server, newTool("fs_directory_tree", "Return a JSON directory tree"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a DirectoryTreeRequest) (*sdkmcp.CallToolResult, any, error) {
			start, err := wm.SafePath(a.WorkspaceID, a.Path)
			if err != nil {
				return nil, nil, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
			}
			tree, err := buildTree(start, a.ExcludePatterns)
			if err != nil {
				return nil, nil, fmt.Errorf("INTERNAL: failed to build directory tree: %v", err)
			}
			return nil, DirectoryTreeResponse{Tree: tree}, nil
		},
	)

	// fs/read_media_file
	sdkmcp.AddTool[ReadMediaFileRequest, ReadMediaFileResponse](server, newTool("fs_read_media_file", "Read media file and return base64 + MIME"),
		func(ctx context.Context, req *sdkmcp.CallToolRequest, a ReadMediaFileRequest) (*sdkmcp.CallToolResult, ReadMediaFileResponse, error) {
			abs, err := wm.SafePath(a.WorkspaceID, a.Path)
			if err != nil {
				return nil, ReadMediaFileResponse{}, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
			}
			content, err := os.ReadFile(abs)
			if err != nil {
				if os.IsNotExist(err) {
					return nil, ReadMediaFileResponse{}, fmt.Errorf("NOT_FOUND: file not found")
				}
				return nil, ReadMediaFileResponse{}, fmt.Errorf("INTERNAL: failed to read media file: %v", err)
			}
			const maxMediaFileSize = 10 * 1024 * 1024
			if len(content) > maxMediaFileSize {
				return nil, ReadMediaFileResponse{}, fmt.Errorf("UNSUPPORTED: media file too large (max 10MB)")
			}
			mimeType := http.DetectContentType(content)
			encoded := base64.StdEncoding.EncodeToString(content)
			return nil, ReadMediaFileResponse{
				MimeType: mimeType,
				Base64:   encoded,
				Size:     int64(len(content)),
			}, nil
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
