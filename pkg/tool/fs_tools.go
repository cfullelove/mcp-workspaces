package tool

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mcp-workspace-manager/pkg/mcp"
	"mcp-workspace-manager/pkg/workspace"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// FSRequest is a generic struct for file system operations that require a workspace and a path.
type FSRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}

// RegisterFSTools registers all file system-related tools with the given registry.
func RegisterFSTools(registry *Registry, manager *workspace.Manager) {
	registry.Register("fs/write_file", makeWriteFileHandler(manager))
	registry.Register("fs/read_text_file", makeReadFileHandler(manager))
	registry.Register("fs/create_directory", makeCreateDirectoryHandler(manager))
	registry.Register("fs/list_directory", makeListDirectoryHandler(manager))
	registry.Register("fs/get_file_info", makeGetFileInfoHandler(manager))
	registry.Register("fs/get_commit_history", makeGetCommitHistoryHandler(manager))
	registry.Register("fs/move_file", makeMoveFileHandler(manager))
	registry.Register("fs/edit_file", makeEditFileHandler(manager))
	registry.Register("fs/read_multiple_files", makeReadMultipleFilesHandler(manager))
	registry.Register("fs/list_directory_with_sizes", makeListDirectoryWithSizesHandler(manager))
	registry.Register("fs/search_files", makeSearchFilesHandler(manager))
	registry.Register("fs/directory_tree", makeDirectoryTreeHandler(manager))
	registry.Register("fs/read_media_file", makeReadMediaFileHandler(manager))
}

// --- Read Media File ---

type ReadMediaFileRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}

type ReadMediaFileResponse struct {
	MimeType string `json:"mimeType"`
	Base64   string `json:"base64"`
	Size     int64  `json:"size"`
}

func makeReadMediaFileHandler(manager *workspace.Manager) HandlerFunc {
	return func(params []byte) (interface{}, *mcp.Error) {
		var req ReadMediaFileRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, mcp.NewError("INVALID_INPUT", "Failed to parse parameters", err.Error())
		}

		absPath, err := manager.SafePath(req.WorkspaceID, req.Path)
		if err != nil {
			return nil, mcp.NewError("OUT_OF_BOUNDS", err.Error(), nil)
		}

		content, err := os.ReadFile(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, mcp.NewError("NOT_FOUND", "File not found", req.Path)
			}
			return nil, mcp.NewError("INTERNAL", "Failed to read media file", err.Error())
		}

		// As per the PRD, we'll return an error if the file is too large for memory.
		// For a prototype, we can set a reasonable limit, e.g., 10MB.
		const maxMediaFileSize = 10 * 1024 * 1024
		if len(content) > maxMediaFileSize {
			return nil, mcp.NewError("UNSUPPORTED", "Media file is too large for prototype", "Max size: 10MB")
		}

		// Detect MIME type
		mimeType := http.DetectContentType(content)

		// Encode to base64
		encodedContent := base64.StdEncoding.EncodeToString(content)

		return &ReadMediaFileResponse{
			MimeType: mimeType,
			Base64:   encodedContent,
			Size:     int64(len(content)),
		}, nil
	}
}

// --- Directory Tree ---

type DirectoryTreeRequest struct {
	WorkspaceID     string   `json:"workspaceId"`
	Path            string   `json:"path"`
	ExcludePatterns []string `json:"excludePatterns,omitempty"`
}

type TreeNode struct {
	Name     string      `json:"name"`
	Type     string      `json:"type"`
	Children *[]TreeNode `json:"children,omitempty"`
}

type DirectoryTreeResponse struct {
	Tree []TreeNode `json:"tree"`
}

func makeDirectoryTreeHandler(manager *workspace.Manager) HandlerFunc {
	return func(params []byte) (interface{}, *mcp.Error) {
		var req DirectoryTreeRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, mcp.NewError("INVALID_INPUT", "Failed to parse parameters", err.Error())
		}

		startPath, err := manager.SafePath(req.WorkspaceID, req.Path)
		if err != nil {
			return nil, mcp.NewError("OUT_OF_BOUNDS", err.Error(), nil)
		}

		tree, err := buildTree(startPath, req.ExcludePatterns)
		if err != nil {
			return nil, mcp.NewError("INTERNAL", "Failed to build directory tree", err)
		}

		return &DirectoryTreeResponse{Tree: tree}, nil
	}
}

func buildTree(root string, excludePatterns []string) ([]TreeNode, error) {
	var tree []TreeNode
	files, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		// Check against exclude patterns
		isExcluded := false
		for _, pattern := range excludePatterns {
			excluded, err := filepath.Match(pattern, file.Name())
			if err != nil {
				return nil, err
			}
			if excluded {
				isExcluded = true
				break
			}
		}
		if isExcluded {
			continue
		}

		node := TreeNode{
			Name: file.Name(),
		}

		if file.IsDir() {
			node.Type = "directory"
			children, err := buildTree(filepath.Join(root, file.Name()), excludePatterns)
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

// --- Search Files ---

type SearchFilesRequest struct {
	WorkspaceID     string   `json:"workspaceId"`
	Path            string   `json:"path"`
	Pattern         string   `json:"pattern"`
	ExcludePatterns []string `json:"excludePatterns,omitempty"`
}

type SearchFilesResponse struct {
	Matches []string `json:"matches"`
}

func makeSearchFilesHandler(manager *workspace.Manager) HandlerFunc {
	return func(params []byte) (interface{}, *mcp.Error) {
		var req SearchFilesRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, mcp.NewError("INVALID_INPUT", "Failed to parse parameters", err.Error())
		}

		if req.WorkspaceID == "" || req.Pattern == "" {
			return nil, mcp.NewError("INVALID_INPUT", "'workspaceId' and 'pattern' are required", nil)
		}

		startPath, err := manager.SafePath(req.WorkspaceID, req.Path)
		if err != nil {
			return nil, mcp.NewError("OUT_OF_BOUNDS", err.Error(), nil)
		}

		var matches []string
		err = filepath.WalkDir(startPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil // Don't match directories
			}

			// Check against the main pattern
			matched, err := filepath.Match(req.Pattern, d.Name())
			if err != nil {
				return err // Propagate glob compilation errors
			}

			if matched {
				// Check against exclude patterns
				isExcluded := false
				for _, excludePattern := range req.ExcludePatterns {
					excluded, err := filepath.Match(excludePattern, d.Name())
					if err != nil {
						return err
					}
					if excluded {
						isExcluded = true
						break
					}
				}

				if !isExcluded {
					// Convert back to a workspace-relative path for the response
					workspaceRoot, err := manager.SafePath(req.WorkspaceID, ".")
					if err != nil {
						return err // Should not happen if the workspace exists
					}
					relPath, err := filepath.Rel(workspaceRoot, path)
					if err == nil {
						matches = append(matches, relPath)
					}
				}
			}
			return nil
		})

		if err != nil {
			return nil, mcp.NewError("INTERNAL", "Failed during file search", err.Error())
		}

		return &SearchFilesResponse{Matches: matches}, nil
	}
}

// --- List Directory With Sizes ---

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

func makeListDirectoryWithSizesHandler(manager *workspace.Manager) HandlerFunc {
	return func(params []byte) (interface{}, *mcp.Error) {
		var req ListDirectoryWithSizesRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, mcp.NewError("INVALID_INPUT", "Failed to parse parameters", err.Error())
		}

		absPath, err := manager.SafePath(req.WorkspaceID, req.Path)
		if err != nil {
			return nil, mcp.NewError("OUT_OF_BOUNDS", err.Error(), nil)
		}

		files, err := os.ReadDir(absPath)
		if err != nil {
			return nil, mcp.NewError("INTERNAL", "Failed to list directory", err.Error())
		}

		var entries []EntryInfo
		var totals TotalsInfo
		for _, file := range files {
			info, err := file.Info()
			if err != nil {
				continue // Skip files we can't get info for
			}

			entry := EntryInfo{
				Name: file.Name(),
			}

			if file.IsDir() {
				entry.Type = "directory"
				entry.Size = 0 // Per PRD, directory size is simplified for the prototype
				totals.Directories++
			} else {
				entry.Type = "file"
				entry.Size = info.Size()
				totals.Files++
				totals.CombinedSize += info.Size()
			}
			entries = append(entries, entry)
		}

		// Sorting logic
		if req.SortBy == "size" {
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].Size > entries[j].Size // Descending
			})
		} else { // Default to sorting by name
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].Name < entries[j].Name // Ascending
			})
		}

		return &ListDirectoryWithSizesResponse{
			Entries: entries,
			Totals:  totals,
		}, nil
	}
}

// --- Read Multiple Files ---

type ReadMultipleFilesRequest struct {
	WorkspaceID string   `json:"workspaceId"`
	Paths       []string `json:"paths"`
}

type FileReadResult struct {
	Path    string `json:"path"`
	OK      bool   `json:"ok"`
	Content *string `json:"content,omitempty"`
	Error   *string `json:"error,omitempty"`
}

type ReadMultipleFilesResponse struct {
	Results []FileReadResult `json:"results"`
}

func makeReadMultipleFilesHandler(manager *workspace.Manager) HandlerFunc {
	return func(params []byte) (interface{}, *mcp.Error) {
		var req ReadMultipleFilesRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, mcp.NewError("INVALID_INPUT", "Failed to parse parameters", err.Error())
		}
		if req.WorkspaceID == "" || len(req.Paths) == 0 {
			return nil, mcp.NewError("INVALID_INPUT", "'workspaceId' and 'paths' are required", nil)
		}

		// Bounded worker pool to limit concurrency
		numJobs := len(req.Paths)
		jobs := make(chan string, numJobs)
		results := make(chan FileReadResult, numJobs)
		numWorkers := 4 // A sensible default for a prototype
		if numJobs < numWorkers {
			numWorkers = numJobs
		}

		for w := 0; w < numWorkers; w++ {
			go func() {
				for path := range jobs {
					absPath, err := manager.SafePath(req.WorkspaceID, path)
					if err != nil {
						errStr := err.Error()
						results <- FileReadResult{Path: path, OK: false, Error: &errStr}
						continue
					}
					contentBytes, err := os.ReadFile(absPath)
					if err != nil {
						errStr := err.Error()
						results <- FileReadResult{Path: path, OK: false, Error: &errStr}
						continue
					}
					content := string(contentBytes)
					results <- FileReadResult{Path: path, OK: true, Content: &content}
				}
			}()
		}

		for _, path := range req.Paths {
			jobs <- path
		}
		close(jobs)

		var responseResults []FileReadResult
		for i := 0; i < numJobs; i++ {
			responseResults = append(responseResults, <-results)
		}

		return &ReadMultipleFilesResponse{Results: responseResults}, nil
	}
}

// --- Edit File ---

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

func makeEditFileHandler(manager *workspace.Manager) HandlerFunc {
	return func(params []byte) (interface{}, *mcp.Error) {
		var req EditFileRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, mcp.NewError("INVALID_INPUT", "Failed to parse parameters", err.Error())
		}
		if req.WorkspaceID == "" || req.Path == "" || len(req.Edits) == 0 {
			return nil, mcp.NewError("INVALID_INPUT", "'workspaceId', 'path', and 'edits' are required", nil)
		}

		absPath, err := manager.SafePath(req.WorkspaceID, req.Path)
		if err != nil {
			return nil, mcp.NewError("OUT_OF_BOUNDS", err.Error(), nil)
		}

		originalContent, err := os.ReadFile(absPath)
		if err != nil {
			return nil, mcp.NewError("NOT_FOUND", "File not found", err.Error())
		}

		newContent := string(originalContent)
		matches := 0
		for _, edit := range req.Edits {
			// Count matches before replacing, as the new text might also contain the old text.
			matches += strings.Count(newContent, edit.OldText)
			newContent = strings.Replace(newContent, edit.OldText, edit.NewText, -1)
		}

		if req.DryRun {
			dmp := diffmatchpatch.New()
			diffs := dmp.DiffMain(string(originalContent), newContent, true)
			return &EditFileDryRunResponse{
				DryRun:  true,
				Diff:    dmp.DiffPrettyText(diffs),
				Matches: matches,
			}, nil
		}

		// Apply changes
		contentBytes := []byte(newContent)
		if err := os.WriteFile(absPath, contentBytes, 0644); err != nil {
			return nil, mcp.NewError("INTERNAL", "Failed to write edited file", err.Error())
		}

		commitHash, err := manager.Commit(req.WorkspaceID, fmt.Sprintf("mcp/fs/edit_file: Edit %s", req.Path), "mcp-client")
		if err != nil {
			return nil, mcp.NewError("INTERNAL", "Failed to commit changes", err.Error())
		}

		return &EditFileResponse{
			DryRun:       false,
			Path:         req.Path,
			Changes:      len(req.Edits),
			BytesWritten: len(contentBytes),
			Commit:       commitHash,
		}, nil
	}
}

// --- Move File ---

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

func makeMoveFileHandler(manager *workspace.Manager) HandlerFunc {
	return func(params []byte) (interface{}, *mcp.Error) {
		var req MoveFileRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, mcp.NewError("INVALID_INPUT", "Failed to parse parameters", err.Error())
		}

		sourceAbsPath, err := manager.SafePath(req.WorkspaceID, req.Source)
		if err != nil {
			return nil, mcp.NewError("OUT_OF_BOUNDS", "Source path is invalid", err.Error())
		}
		destAbsPath, err := manager.SafePath(req.WorkspaceID, req.Destination)
		if err != nil {
			return nil, mcp.NewError("OUT_OF_BOUNDS", "Destination path is invalid", err.Error())
		}

		if _, err := os.Stat(destAbsPath); !os.IsNotExist(err) {
			return nil, mcp.NewError("ALREADY_EXISTS", "Destination path already exists", req.Destination)
		}

		if err := os.Rename(sourceAbsPath, destAbsPath); err != nil {
			return nil, mcp.NewError("INTERNAL", "Failed to move file/directory", err.Error())
		}

		commitHash, err := manager.Commit(req.WorkspaceID, fmt.Sprintf("mcp/fs/move_file: Move %s to %s", req.Source, req.Destination), "mcp-client")
		if err != nil {
			return nil, mcp.NewError("INTERNAL", "Failed to commit changes", err.Error())
		}

		return &MoveFileResponse{
			Source:      req.Source,
			Destination: req.Destination,
			Commit:      commitHash,
		}, nil
	}
}

// --- Get Commit History ---

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

func makeGetCommitHistoryHandler(manager *workspace.Manager) HandlerFunc {
	return func(params []byte) (interface{}, *mcp.Error) {
		var req GetCommitHistoryRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, mcp.NewError("INVALID_INPUT", "Failed to parse parameters", err.Error())
		}

		if req.WorkspaceID == "" {
			return nil, mcp.NewError("INVALID_INPUT", "'workspaceId' is required", nil)
		}

		limit := 20 // Default limit
		if req.Limit > 0 {
			limit = req.Limit
		}

		// Note: The 'path' parameter is not yet implemented in the manager.
		// This implementation returns the history for the entire workspace.
		commits, err := manager.GetCommitHistory(req.WorkspaceID, limit)
		if err != nil {
			return nil, mcp.NewError("INTERNAL", "Failed to get commit history", err.Error())
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

		return &GetCommitHistoryResponse{Log: log}, nil
	}
}

// --- Create Directory ---

type CreateDirectoryRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}

type CreateDirectoryResponse struct {
	Path    string `json:"path"`
	Created bool   `json:"created"`
	Commit  string `json:"commit"`
}

func makeCreateDirectoryHandler(manager *workspace.Manager) HandlerFunc {
	return func(params []byte) (interface{}, *mcp.Error) {
		var req CreateDirectoryRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, mcp.NewError("INVALID_INPUT", "Failed to parse parameters", err.Error())
		}

		absPath, err := manager.SafePath(req.WorkspaceID, req.Path)
		if err != nil {
			return nil, mcp.NewError("OUT_OF_BOUNDS", err.Error(), nil)
		}

		_, err = os.Stat(absPath)
		created := os.IsNotExist(err)

		if err := os.MkdirAll(absPath, 0755); err != nil {
			return nil, mcp.NewError("INTERNAL", "Failed to create directory", err.Error())
		}

		// To ensure empty directories are committed, we can add a .gitkeep file.
		gitkeepPath := filepath.Join(absPath, ".gitkeep")
		if _, err := os.Stat(gitkeepPath); os.IsNotExist(err) {
			if f, err := os.Create(gitkeepPath); err == nil {
				f.Close()
			}
		}

		commitHash, err := manager.Commit(req.WorkspaceID, fmt.Sprintf("mcp/fs/create_directory: Create %s", req.Path), "mcp-client")
		if err != nil {
			return nil, mcp.NewError("INTERNAL", "Failed to commit changes", err.Error())
		}

		return &CreateDirectoryResponse{
			Path:    req.Path,
			Created: created,
			Commit:  commitHash,
		}, nil
	}
}

// --- List Directory ---

type ListDirectoryRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}

type ListDirectoryResponse struct {
	Entries []string `json:"entries"`
}

func makeListDirectoryHandler(manager *workspace.Manager) HandlerFunc {
	return func(params []byte) (interface{}, *mcp.Error) {
		var req ListDirectoryRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, mcp.NewError("INVALID_INPUT", "Failed to parse parameters", err.Error())
		}

		absPath, err := manager.SafePath(req.WorkspaceID, req.Path)
		if err != nil {
			return nil, mcp.NewError("OUT_OF_BOUNDS", err.Error(), nil)
		}

		files, err := os.ReadDir(absPath)
		if err != nil {
			return nil, mcp.NewError("INTERNAL", "Failed to list directory", err.Error())
		}

		var entries []string
		for _, file := range files {
			prefix := "[FILE]"
			if file.IsDir() {
				prefix = "[DIR] "
			}
			entries = append(entries, prefix+" "+file.Name())
		}

		return &ListDirectoryResponse{Entries: entries}, nil
	}
}

// --- Get File Info ---

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

func makeGetFileInfoHandler(manager *workspace.Manager) HandlerFunc {
	return func(params []byte) (interface{}, *mcp.Error) {
		var req GetFileInfoRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, mcp.NewError("INVALID_INPUT", "Failed to parse parameters", err.Error())
		}

		absPath, err := manager.SafePath(req.WorkspaceID, req.Path)
		if err != nil {
			return nil, mcp.NewError("OUT_OF_BOUNDS", err.Error(), nil)
		}

		info, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, mcp.NewError("NOT_FOUND", "File or directory not found", req.Path)
			}
			return nil, mcp.NewError("INTERNAL", "Failed to get file info", err.Error())
		}

		fileType := "file"
		if info.IsDir() {
			fileType = "directory"
		}

		return &GetFileInfoResponse{
			Size:        info.Size(),
			Mtime:       info.ModTime().UTC().Format(time.RFC3339),
			Type:        fileType,
			Permissions: info.Mode().String(),
		}, nil
	}
}

// --- Read File ---

// ReadFileRequest defines the parameters for the fs/read_text_file tool.
type ReadFileRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
	Head        *int   `json:"head,omitempty"`
	Tail        *int   `json:"tail,omitempty"`
}

// ReadFileResponse defines the output for the fs/read_text_file tool.
type ReadFileResponse struct {
	Content    string `json:"content"`
	TotalLines int    `json:"totalLines,omitempty"`
	Head       *int   `json:"head,omitempty"`
	Tail       *int   `json:"tail,omitempty"`
}

func makeReadFileHandler(manager *workspace.Manager) HandlerFunc {
	return func(params []byte) (interface{}, *mcp.Error) {
		var req ReadFileRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, mcp.NewError("INVALID_INPUT", "Failed to parse parameters", err.Error())
		}

		if req.WorkspaceID == "" || req.Path == "" {
			return nil, mcp.NewError("INVALID_INPUT", "'workspaceId' and 'path' are required", nil)
		}

		if req.Head != nil && req.Tail != nil {
			return nil, mcp.NewError("INVALID_INPUT", "Cannot specify both 'head' and 'tail'", nil)
		}

		absPath, err := manager.SafePath(req.WorkspaceID, req.Path)
		if err != nil {
			return nil, mcp.NewError("OUT_OF_BOUNDS", err.Error(), nil)
		}

		contentBytes, err := os.ReadFile(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, mcp.NewError("NOT_FOUND", "File not found", req.Path)
			}
			return nil, mcp.NewError("INTERNAL", "Failed to read file", err.Error())
		}

		content := string(contentBytes)
		lines := strings.Split(content, "\n")
		totalLines := len(lines)

		resp := &ReadFileResponse{
			TotalLines: totalLines,
		}

		if req.Head != nil {
			h := *req.Head
			if h > totalLines {
				h = totalLines
			}
			resp.Content = strings.Join(lines[:h], "\n")
			resp.Head = &h
		} else if req.Tail != nil {
			t := *req.Tail
			if t > totalLines {
				t = totalLines
			}
			resp.Content = strings.Join(lines[totalLines-t:], "\n")
			resp.Tail = &t
		} else {
			resp.Content = content
		}

		return resp, nil
	}
}

// --- Write File ---

// WriteFileRequest defines the parameters for the fs/write_file tool.
type WriteFileRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
	Content     string `json:"content"`
}

// WriteFileResponse defines the output for the fs/write_file tool.
type WriteFileResponse struct {
	Path          string `json:"path"`
	BytesWritten  int    `json:"bytesWritten"`
	Overwritten   bool   `json:"overwritten"`
	Commit        string `json:"commit"`
}

func makeWriteFileHandler(manager *workspace.Manager) HandlerFunc {
	return func(params []byte) (interface{}, *mcp.Error) {
		var req WriteFileRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, mcp.NewError("INVALID_INPUT", "Failed to parse parameters", err.Error())
		}

		if req.WorkspaceID == "" || req.Path == "" {
			return nil, mcp.NewError("INVALID_INPUT", "'workspaceId' and 'path' are required", nil)
		}

		// Get the absolute path and ensure it's within the workspace.
		absPath, err := manager.SafePath(req.WorkspaceID, req.Path)
		if err != nil {
			return nil, mcp.NewError("OUT_OF_BOUNDS", err.Error(), nil)
		}

		// Check if the file already exists to set the 'overwritten' flag.
		_, err = os.Stat(absPath)
		overwritten := !os.IsNotExist(err)

		// Create parent directories if they don't exist.
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return nil, mcp.NewError("INTERNAL", "Failed to create parent directories", err.Error())
		}

		// Write the file.
		contentBytes := []byte(req.Content)
		if err := os.WriteFile(absPath, contentBytes, 0644); err != nil {
			return nil, mcp.NewError("INTERNAL", "Failed to write file", err.Error())
		}
		bytesWritten := len(contentBytes)

		// Commit the change.
		commitHash, err := manager.Commit(req.WorkspaceID, fmt.Sprintf("mcp/fs/write_file: Write %s", req.Path), "mcp-client")
		if err != nil {
			return nil, mcp.NewError("INTERNAL", "Failed to commit changes", err.Error())
		}

		return &WriteFileResponse{
			Path:         req.Path,
			BytesWritten: bytesWritten,
			Overwritten:  overwritten,
			Commit:       commitHash,
		}, nil
	}
}