package mcpsdk

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"

	"mcp-workspace-manager/pkg/events"
	"mcp-workspace-manager/pkg/workspace"
)

// Shared tool implementations used by both MCP server tools and REST API.

func isProtectedName(name string) bool {
	return name == ".git" || name == ".gitkeep"
}

func isProtectedPath(rel string) bool {
	cleaned := filepath.Clean(rel)
	for _, seg := range strings.Split(cleaned, string(os.PathSeparator)) {
		if seg == "" {
			continue
		}
		if isProtectedName(seg) {
			return true
		}
	}
	return false
}

func WorkspaceCreate(ctx context.Context, wm *workspace.Manager, input CreateWorkspaceRequest) (CreateWorkspaceResponse, error) {
	if input.Name == "" {
		return CreateWorkspaceResponse{}, fmt.Errorf("invalid input: 'name' is required")
	}
	id, path, err := wm.Create(input.Name)
	if err != nil {
		return CreateWorkspaceResponse{}, err
	}
	return CreateWorkspaceResponse{WorkspaceID: id, Path: path}, nil
}

func WorkspaceList(ctx context.Context, wm *workspace.Manager, input ListWorkspacesRequest) (ListWorkspacesResponse, error) {
	workspaces, err := wm.List()
	if err != nil {
		return ListWorkspacesResponse{}, err
	}
	var out []WorkspaceInfo
	for _, w := range workspaces {
		out = append(out, WorkspaceInfo{
			Name: w.Name,
			Path: w.Path,
		})
	}
	return ListWorkspacesResponse{Workspaces: out}, nil
}

func FSWriteFile(ctx context.Context, wm *workspace.Manager, a WriteFileRequest) (WriteFileResponse, error) {
	if a.WorkspaceID == "" || a.Path == "" {
		return WriteFileResponse{}, fmt.Errorf("INVALID_INPUT: 'workspaceId' and 'path' are required")
	}
	if isProtectedPath(a.Path) {
		return WriteFileResponse{}, fmt.Errorf("NOT_FOUND: file not found")
	}
	absPath, err := wm.SafePath(a.WorkspaceID, a.Path)
	if err != nil {
		return WriteFileResponse{}, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
	}
	_, statErr := os.Stat(absPath)
	overwritten := !os.IsNotExist(statErr)

	// Preconditions
	var currEtag string
	if overwritten {
		if b, errR := os.ReadFile(absPath); errR == nil {
			sum := sha256.Sum256(b)
			currEtag = fmt.Sprintf("%x", sum[:])
		}
	}
	if a.IfMatchFileEtag != nil && *a.IfMatchFileEtag != "" && overwritten && currEtag != *a.IfMatchFileEtag {
		return WriteFileResponse{}, fmt.Errorf("CONFLICT: file etag mismatch")
	}
	if a.IfMatchWorkspaceHead != nil && *a.IfMatchWorkspaceHead != "" {
		if head, _ := wm.HeadCommit(a.WorkspaceID); head != *a.IfMatchWorkspaceHead {
			return WriteFileResponse{}, fmt.Errorf("CONFLICT: workspace head mismatch")
		}
	}

	// Prepare content and short-circuit if no-op (unchanged file)
	contentBytes := []byte(a.Content)
	if overwritten {
		sumNew := sha256.Sum256(contentBytes)
		newEtag := fmt.Sprintf("%x", sumNew[:])
		if currEtag != "" && newEtag == currEtag {
			// No changes; do not write, do not commit, do not emit events
			return WriteFileResponse{Path: a.Path, BytesWritten: 0, Overwritten: overwritten, Commit: ""}, nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return WriteFileResponse{}, fmt.Errorf("INTERNAL: failed to create parent directories: %v", err)
	}
	if err := os.WriteFile(absPath, contentBytes, 0644); err != nil {
		return WriteFileResponse{}, fmt.Errorf("INTERNAL: failed to write file: %v", err)
	}
	commit, err := wm.Commit(a.WorkspaceID, fmt.Sprintf("mcp/fs_write_file: Write %s", a.Path), "mcp-client")
	if err != nil {
		return WriteFileResponse{}, fmt.Errorf("INTERNAL: failed to commit changes: %v", err)
	}

	// Publish event
	evtType := "file.created"
	if overwritten {
		evtType = "file.updated"
	}
	commitCopy := commit
	publishWorkspaceEvent(a.WorkspaceID, events.WorkspaceEvent{
		Type:   evtType,
		Path:   a.Path,
		IsDir:  false,
		Commit: &commitCopy,
	})

	return WriteFileResponse{Path: a.Path, BytesWritten: len(contentBytes), Overwritten: overwritten, Commit: commit}, nil
}

func FSReadTextFile(ctx context.Context, wm *workspace.Manager, a ReadFileRequest) (ReadFileResponse, error) {
	if a.WorkspaceID == "" || a.Path == "" {
		return ReadFileResponse{}, fmt.Errorf("INVALID_INPUT: 'workspaceId' and 'path' are required")
	}
	if a.Head != nil && a.Tail != nil {
		return ReadFileResponse{}, fmt.Errorf("INVALID_INPUT: cannot specify both 'head' and 'tail'")
	}
	if isProtectedPath(a.Path) {
		return ReadFileResponse{}, fmt.Errorf("NOT_FOUND: file not found")
	}
	absPath, err := wm.SafePath(a.WorkspaceID, a.Path)
	if err != nil {
		return ReadFileResponse{}, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
	}
	contentBytes, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ReadFileResponse{}, fmt.Errorf("NOT_FOUND: file not found")
		}
		return ReadFileResponse{}, fmt.Errorf("INTERNAL: failed to read file: %v", err)
	}
	content := string(contentBytes)
	lines := strings.Split(content, "\n")
	total := len(lines)

	sum := sha256.Sum256(contentBytes)
	etag := fmt.Sprintf("%x", sum[:])

	var mtimeStr string
	if info, statErr := os.Stat(absPath); statErr == nil {
		mtimeStr = info.ModTime().UTC().Format(time.RFC3339)
	}

	head, _ := wm.HeadCommit(a.WorkspaceID)

	resp := ReadFileResponse{
		TotalLines:    total,
		Etag:          etag,
		Mtime:         mtimeStr,
		WorkspaceHead: head,
	}

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
	return resp, nil
}

func FSCreateDirectory(ctx context.Context, wm *workspace.Manager, a CreateDirectoryRequest) (CreateDirectoryResponse, error) {
	if isProtectedPath(a.Path) {
		return CreateDirectoryResponse{}, fmt.Errorf("NOT_FOUND: file not found")
	}
	absPath, err := wm.SafePath(a.WorkspaceID, a.Path)
	if err != nil {
		return CreateDirectoryResponse{}, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
	}
	_, statErr := os.Stat(absPath)
	created := os.IsNotExist(statErr)
	if err := os.MkdirAll(absPath, 0755); err != nil {
		return CreateDirectoryResponse{}, fmt.Errorf("INTERNAL: failed to create directory: %v", err)
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
		return CreateDirectoryResponse{}, fmt.Errorf("INTERNAL: failed to commit changes: %v", err)
	}

	// Publish event
	commitCopy := commit
	publishWorkspaceEvent(a.WorkspaceID, events.WorkspaceEvent{
		Type:   "dir.created",
		Path:   a.Path,
		IsDir:  true,
		Commit: &commitCopy,
	})

	return CreateDirectoryResponse{Path: a.Path, Created: created, Commit: commit}, nil
}

func FSListDirectory(ctx context.Context, wm *workspace.Manager, a ListDirectoryRequest) (ListDirectoryResponse, error) {
	absPath, err := wm.SafePath(a.WorkspaceID, a.Path)
	if err != nil {
		return ListDirectoryResponse{}, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
	}
	files, err := os.ReadDir(absPath)
	if err != nil {
		return ListDirectoryResponse{}, fmt.Errorf("INTERNAL: failed to list directory: %v", err)
	}
	var entries []string
	for _, f := range files {
		if isProtectedName(f.Name()) {
			continue
		}
		prefix := "[FILE]"
		if f.IsDir() {
			prefix = "[DIR]"
		}
		entries = append(entries, prefix+" "+f.Name())
	}
	return ListDirectoryResponse{Entries: entries}, nil
}

func FSGetFileInfo(ctx context.Context, wm *workspace.Manager, a GetFileInfoRequest) (GetFileInfoResponse, error) {
	if isProtectedPath(a.Path) {
		return GetFileInfoResponse{}, fmt.Errorf("NOT_FOUND: file or directory not found")
	}
	absPath, err := wm.SafePath(a.WorkspaceID, a.Path)
	if err != nil {
		return GetFileInfoResponse{}, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return GetFileInfoResponse{}, fmt.Errorf("NOT_FOUND: file or directory not found")
		}
		return GetFileInfoResponse{}, fmt.Errorf("INTERNAL: failed to get file info: %v", err)
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
	return out, nil
}

func FSGetCommitHistory(ctx context.Context, wm *workspace.Manager, a GetCommitHistoryRequest) (GetCommitHistoryResponse, error) {
	if a.WorkspaceID == "" {
		return GetCommitHistoryResponse{}, fmt.Errorf("INVALID_INPUT: 'workspaceId' is required")
	}
	limit := 20
	if a.Limit > 0 {
		limit = a.Limit
	}
	commits, err := wm.GetCommitHistory(a.WorkspaceID, limit)
	if err != nil {
		return GetCommitHistoryResponse{}, fmt.Errorf("INTERNAL: failed to get commit history: %v", err)
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
	return GetCommitHistoryResponse{Log: log}, nil
}

func FSMoveFile(ctx context.Context, wm *workspace.Manager, a MoveFileRequest) (MoveFileResponse, error) {
	if isProtectedPath(a.Source) || isProtectedPath(a.Destination) {
		return MoveFileResponse{}, fmt.Errorf("NOT_FOUND: file not found")
	}
	src, err := wm.SafePath(a.WorkspaceID, a.Source)
	if err != nil {
		return MoveFileResponse{}, fmt.Errorf("OUT_OF_BOUNDS: source path invalid: %v", err)
	}
	dst, err := wm.SafePath(a.WorkspaceID, a.Destination)
	if err != nil {
		return MoveFileResponse{}, fmt.Errorf("OUT_OF_BOUNDS: destination path invalid: %v", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		return MoveFileResponse{}, fmt.Errorf("ALREADY_EXISTS: destination exists")
	}
	if err := os.Rename(src, dst); err != nil {
		return MoveFileResponse{}, fmt.Errorf("INTERNAL: move failed: %v", err)
	}
	commit, err := wm.Commit(a.WorkspaceID, fmt.Sprintf("mcp/fs_move_file: Move %s to %s", a.Source, a.Destination), "mcp-client")
	if err != nil {
		return MoveFileResponse{}, fmt.Errorf("INTERNAL: commit failed: %v", err)
	}

	// Determine dir/file
	isDir := false
	if info, statErr := os.Stat(dst); statErr == nil {
		isDir = info.IsDir()
	}
	// Publish event
	commitCopy := commit
	prev := a.Source
	publishWorkspaceEvent(a.WorkspaceID, events.WorkspaceEvent{
		Type:     "file.moved",
		Path:     a.Destination,
		PrevPath: &prev,
		IsDir:    isDir,
		Commit:   &commitCopy,
	})

	return MoveFileResponse{Source: a.Source, Destination: a.Destination, Commit: commit}, nil
}

func FSEditFile(ctx context.Context, wm *workspace.Manager, a EditFileRequest) (any, error) {
	if a.WorkspaceID == "" || a.Path == "" || len(a.Edits) == 0 {
		return nil, fmt.Errorf("INVALID_INPUT: 'workspaceId', 'path', and 'edits' are required")
	}
	if isProtectedPath(a.Path) {
		return nil, fmt.Errorf("NOT_FOUND: file not found")
	}
	absPath, err := wm.SafePath(a.WorkspaceID, a.Path)
	if err != nil {
		return nil, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
	}
	orig, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("NOT_FOUND: file not found")
	}
	// Preconditions (current etag)
	sum := sha256.Sum256(orig)
	currEtag := fmt.Sprintf("%x", sum[:])
	if a.IfMatchFileEtag != nil && *a.IfMatchFileEtag != "" && currEtag != *a.IfMatchFileEtag {
		return nil, fmt.Errorf("CONFLICT: file etag mismatch")
	}
	if a.IfMatchWorkspaceHead != nil && *a.IfMatchWorkspaceHead != "" {
		if head, _ := wm.HeadCommit(a.WorkspaceID); head != *a.IfMatchWorkspaceHead {
			return nil, fmt.Errorf("CONFLICT: workspace head mismatch")
		}
	}

	newContent := string(orig)
	matches := 0
	for _, e := range a.Edits {
		matches += strings.Count(newContent, e.OldText)
		newContent = strings.ReplaceAll(newContent, e.OldText, e.NewText)
	}

	// If no effective change, short-circuit (no write, no commit, no event)
	if newContent == string(orig) {
		out := EditFileResponse{DryRun: false, Path: a.Path, Changes: 0, BytesWritten: 0, Commit: ""}
		return out, nil
	}

	if a.DryRun {
		dmp := diffmatchpatch.New()
		diffs := dmp.DiffMain(string(orig), newContent, true)
		out := EditFileDryRunResponse{DryRun: true, Diff: dmp.DiffPrettyText(diffs), Matches: matches}
		return out, nil
	}

	contentBytes := []byte(newContent)
	if err := os.WriteFile(absPath, contentBytes, 0644); err != nil {
		return nil, fmt.Errorf("INTERNAL: failed to write edited file: %v", err)
	}
	commit, err := wm.Commit(a.WorkspaceID, fmt.Sprintf("mcp/fs_edit_file: Edit %s", a.Path), "mcp-client")
	if err != nil {
		return nil, fmt.Errorf("INTERNAL: failed to commit changes: %v", err)
	}

	// Publish event
	commitCopy := commit
	publishWorkspaceEvent(a.WorkspaceID, events.WorkspaceEvent{
		Type:   "file.updated",
		Path:   a.Path,
		IsDir:  false,
		Commit: &commitCopy,
	})

	out := EditFileResponse{DryRun: false, Path: a.Path, Changes: len(a.Edits), BytesWritten: len(contentBytes), Commit: commit}
	return out, nil
}

func FSReadMultipleFiles(ctx context.Context, wm *workspace.Manager, a ReadMultipleFilesRequest) (ReadMultipleFilesResponse, error) {
	if a.WorkspaceID == "" || len(a.Paths) == 0 {
		return ReadMultipleFilesResponse{}, fmt.Errorf("INVALID_INPUT: 'workspaceId' and 'paths' are required")
	}
	out := make([]FileReadResult, 0, len(a.Paths))
	for _, p := range a.Paths {
		if isProtectedPath(p) {
			errStr := "NOT_FOUND: file not found"
			out = append(out, FileReadResult{Path: p, OK: false, Error: &errStr})
			continue
		}
		abs, err := wm.SafePath(a.WorkspaceID, p)
		if err != nil {
			errStr := err.Error()
			out = append(out, FileReadResult{Path: p, OK: false, Error: &errStr})
			continue
		}
		contentBytes, err := os.ReadFile(abs)
		if err != nil {
			errStr := err.Error()
			out = append(out, FileReadResult{Path: p, OK: false, Error: &errStr})
			continue
		}
		content := string(contentBytes)
		out = append(out, FileReadResult{Path: p, OK: true, Content: &content})
	}
	return ReadMultipleFilesResponse{Results: out}, nil
}

func FSListDirectoryWithSizes(ctx context.Context, wm *workspace.Manager, a ListDirectoryWithSizesRequest) (ListDirectoryWithSizesResponse, error) {
	abs, err := wm.SafePath(a.WorkspaceID, a.Path)
	if err != nil {
		return ListDirectoryWithSizesResponse{}, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
	}
	files, err := os.ReadDir(abs)
	if err != nil {
		return ListDirectoryWithSizesResponse{}, fmt.Errorf("INTERNAL: failed to list directory: %v", err)
	}
	var entries []EntryInfo
	var totals TotalsInfo
	for _, f := range files {
		if isProtectedName(f.Name()) {
			continue
		}
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
	return ListDirectoryWithSizesResponse{Entries: entries, Totals: totals}, nil
}

func FSSearchFiles(ctx context.Context, wm *workspace.Manager, a SearchFilesRequest) (SearchFilesResponse, error) {
	if a.WorkspaceID == "" || a.Pattern == "" {
		return SearchFilesResponse{}, fmt.Errorf("INVALID_INPUT: 'workspaceId' and 'pattern' are required")
	}
	start, err := wm.SafePath(a.WorkspaceID, a.Path)
	if err != nil {
		return SearchFilesResponse{}, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
	}
	var matches []string
	err = filepath.WalkDir(start, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if isProtectedName(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if isProtectedName(d.Name()) {
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
		return SearchFilesResponse{}, fmt.Errorf("INTERNAL: search failed: %v", err)
	}
	return SearchFilesResponse{Matches: matches}, nil
}

func FSDirectoryTree(ctx context.Context, wm *workspace.Manager, a DirectoryTreeRequest) (any, error) {
	start, err := wm.SafePath(a.WorkspaceID, a.Path)
	if err != nil {
		return nil, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
	}
	tree, err := buildTree(start, a.ExcludePatterns)
	if err != nil {
		return nil, fmt.Errorf("INTERNAL: failed to build directory tree: %v", err)
	}
	return DirectoryTreeResponse{Tree: tree}, nil
}

func FSReadMediaFile(ctx context.Context, wm *workspace.Manager, a ReadMediaFileRequest) (ReadMediaFileResponse, error) {
	if isProtectedPath(a.Path) {
		return ReadMediaFileResponse{}, fmt.Errorf("NOT_FOUND: file not found")
	}
	abs, err := wm.SafePath(a.WorkspaceID, a.Path)
	if err != nil {
		return ReadMediaFileResponse{}, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
	}
	content, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return ReadMediaFileResponse{}, fmt.Errorf("NOT_FOUND: file not found")
		}
		return ReadMediaFileResponse{}, fmt.Errorf("INTERNAL: failed to read media file: %v", err)
	}
	const maxMediaFileSize = 10 * 1024 * 1024
	if len(content) > maxMediaFileSize {
		return ReadMediaFileResponse{}, fmt.Errorf("UNSUPPORTED: media file too large (max 10MB)")
	}
	mimeType := http.DetectContentType(content)
	encoded := base64.StdEncoding.EncodeToString(content)
	return ReadMediaFileResponse{
		MimeType: mimeType,
		Base64:   encoded,
		Size:     int64(len(content)),
	}, nil
}

// Helper used by REST layer to detect EOF in some contexts.
var _ = io.EOF

func FSDeleteFile(ctx context.Context, wm *workspace.Manager, a DeleteFileRequest) (DeleteFileResponse, error) {
	if a.WorkspaceID == "" || a.Path == "" {
		return DeleteFileResponse{}, fmt.Errorf("INVALID_INPUT: 'workspaceId' and 'path' are required")
	}
	if isProtectedPath(a.Path) {
		return DeleteFileResponse{}, fmt.Errorf("NOT_FOUND: file not found")
	}
	absPath, err := wm.SafePath(a.WorkspaceID, a.Path)
	if err != nil {
		return DeleteFileResponse{}, fmt.Errorf("OUT_OF_BOUNDS: %v", err)
	}
	// Determine if directory before removal
	isDir := false
	if info, statErr := os.Stat(absPath); statErr == nil {
		isDir = info.IsDir()
	}
	if err := os.RemoveAll(absPath); err != nil {
		return DeleteFileResponse{}, fmt.Errorf("INTERNAL: failed to delete file: %v", err)
	}
	commit, err := wm.Commit(a.WorkspaceID, fmt.Sprintf("mcp/fs_delete_file: Delete %s", a.Path), "mcp-client")
	if err != nil {
		return DeleteFileResponse{}, fmt.Errorf("INTERNAL: failed to commit changes: %v", err)
	}

	// Publish event
	evtType := "file.deleted"
	if isDir {
		evtType = "dir.deleted"
	}
	commitCopy := commit
	publishWorkspaceEvent(a.WorkspaceID, events.WorkspaceEvent{
		Type:   evtType,
		Path:   a.Path,
		IsDir:  isDir,
		Commit: &commitCopy,
	})

	return DeleteFileResponse{Path: a.Path, Commit: commit}, nil
}
