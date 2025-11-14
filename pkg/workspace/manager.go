package workspace

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Manager handles all operations related to workspaces.
type Manager struct {
	rootPath string
}

type Workspace struct {
	Name string
	Path string
}

// NewManager creates a new Workspace Manager.
// It ensures the root directory for workspaces exists.
func NewManager(rootPath string) (*Manager, error) {
	if rootPath == "" {
		return nil, fmt.Errorf("workspaces root path cannot be empty")
	}
	// Ensure the root directory exists
	if err := os.MkdirAll(rootPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspaces root directory: %w", err)
	}
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for workspaces root: %w", err)
	}
	return &Manager{rootPath: absRoot}, nil
}

// RootPath returns the absolute root path for all workspaces.
func (m *Manager) RootPath() string {
	return m.rootPath
}

// Create initializes a new workspace.
// It generates a slug, creates a directory, and initializes a git repository.
func (m *Manager) Create(name string) (string, string, error) {
	slug := GenerateSlug(name)
	workspacePath := filepath.Join(m.rootPath, slug)

	// Ensure uniqueness by appending a short hash if the directory already exists.
	// This is a simple approach; more robust strategies could be used in a real app.
	if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
		slog.Warn("Workspace with this slug already exists, generating a unique name", "slug", slug)
		// Simple disambiguation using a timestamp hash.
		hash := time.Now().Format("20060102150405")
		slug = fmt.Sprintf("%s-%s", slug, hash)
		workspacePath = filepath.Join(m.rootPath, slug)
	}

	// Create the workspace directory
	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create workspace directory: %w", err)
	}

	// Initialize a new git repository
	_, err := git.PlainInit(workspacePath, false)
	if err != nil {
		return "", "", fmt.Errorf("failed to initialize git repository: %w", err)
	}

	// Create a .gitkeep file to allow for an initial commit
	gitkeepPath := filepath.Join(workspacePath, ".gitkeep")
	if f, err := os.Create(gitkeepPath); err == nil {
		f.Close()
	}

	slog.Info("Successfully created and initialized workspace", "id", slug, "path", workspacePath)

	// Create an initial commit
	if _, err := m.Commit(slug, "Initial commit", "system"); err != nil {
		// This is not a fatal error for creation, but we should still log it.
		slog.Warn("Failed to create initial commit", "workspaceId", slug, "error", err)
	}

	return slug, workspacePath, nil
}

// SafePath resolves a relative path from within a workspace and ensures it does not escape the workspace root.
// It returns the absolute, cleaned path.
func (m *Manager) SafePath(workspaceID, relativePath string) (string, error) {
	workspaceRoot := filepath.Join(m.rootPath, workspaceID)
	if _, err := os.Stat(workspaceRoot); os.IsNotExist(err) {
		return "", fmt.Errorf("workspace '%s' not found", workspaceID)
	}

	// The path to be joined with the workspace root.
	// We clean it to prevent trivial directory traversal attacks.
	cleanedRelativePath := filepath.Clean(relativePath)

	// Prevent absolute paths in the relative path input
	if filepath.IsAbs(cleanedRelativePath) {
		return "", fmt.Errorf("path must be relative")
	}

	// Join the workspace root with the user-provided path.
	absPath := filepath.Join(workspaceRoot, cleanedRelativePath)

	// Final check: ensure the resulting absolute path is still within the workspace root.
	// This handles more complex traversals like `../..` that `filepath.Clean` might simplify
	// but not fully prevent from escaping in all contexts.
	if !strings.HasPrefix(absPath, workspaceRoot) {
		return "", fmt.Errorf("path escapes workspace boundaries")
	}

	return absPath, nil
}

// GetCommitHistory returns the commit log for a workspace.
func (m *Manager) GetCommitHistory(workspaceID string, limit int) ([]object.Commit, error) {
	workspacePath := filepath.Join(m.rootPath, workspaceID)
	repo, err := git.PlainOpen(workspacePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}

	cIter, err := repo.Log(&git.LogOptions{Order: git.LogOrderCommitterTime})
	if err != nil {
		return nil, fmt.Errorf("failed to get commit iterator: %w", err)
	}
	defer cIter.Close()

	var commits []object.Commit
	for {
		if len(commits) >= limit {
			break
		}
		commit, err := cIter.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		commits = append(commits, *commit)
	}
	return commits, nil
}

// GetFileCommitHistory returns commits that modified the specified file path within a workspace.
func (m *Manager) GetFileCommitHistory(workspaceID, relPath string, limit int) ([]object.Commit, error) {
	workspacePath := filepath.Join(m.rootPath, workspaceID)
	repo, err := git.PlainOpen(workspacePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}

	cIter, err := repo.Log(&git.LogOptions{Order: git.LogOrderCommitterTime})
	if err != nil {
		return nil, fmt.Errorf("failed to get commit iterator: %w", err)
	}
	defer cIter.Close()

	var commits []object.Commit
	for {
		if len(commits) >= limit {
			break
		}
		commit, err := cIter.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		// Compare with first parent if any; if none (root), include if file exists in this commit.
		if parent, err := commit.Parents().Next(); err == nil && parent != nil {
			pt, err := parent.Tree()
			if err != nil {
				continue
			}
			ct, err := commit.Tree()
			if err != nil {
				continue
			}
			patch, err := pt.Patch(ct)
			if err != nil {
				continue
			}
			fileChanged := false
			for _, fp := range patch.FilePatches() {
				from, to := fp.Files()
				if (from != nil && from.Path() == relPath) || (to != nil && to.Path() == relPath) {
					fileChanged = true
					break
				}
			}
			if fileChanged {
				commits = append(commits, *commit)
			}
		} else {
			// Root commit case: include if the file exists in this tree (treated as an addition)
			if t, err := commit.Tree(); err == nil {
				if _, ferr := t.File(relPath); ferr == nil {
					commits = append(commits, *commit)
				}
			}
		}
	}
	return commits, nil
}

// ReadFileAtCommit returns the file content at a given commit hash.
func (m *Manager) ReadFileAtCommit(workspaceID, relPath, commitHash string) (string, error) {
	workspacePath := filepath.Join(m.rootPath, workspaceID)
	repo, err := git.PlainOpen(workspacePath)
	if err != nil {
		return "", fmt.Errorf("failed to open git repository: %w", err)
	}
	h := plumbing.NewHash(commitHash)
	c, err := repo.CommitObject(h)
	if err != nil {
		return "", fmt.Errorf("failed to resolve commit: %w", err)
	}
	t, err := c.Tree()
	if err != nil {
		return "", fmt.Errorf("failed to get commit tree: %w", err)
	}
	f, err := t.File(relPath)
	if err != nil {
		return "", fmt.Errorf("file not found at commit")
	}
	r, err := f.Reader()
	if err != nil {
		return "", fmt.Errorf("failed to open file reader: %w", err)
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("failed to read file at commit: %w", err)
	}
	return string(b), nil
}

// Commit creates a new commit in the specified workspace's git repository.
// It stages all changes before committing and returns the commit hash.
func (m *Manager) Commit(workspaceID, message, authorName string) (string, error) {
	workspacePath := filepath.Join(m.rootPath, workspaceID)
	repo, err := git.PlainOpen(workspacePath)
	if err != nil {
		return "", fmt.Errorf("failed to open git repository: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}

	// Stage all changes. `git add -A`
	if err := worktree.AddGlob("."); err != nil {
		return "", fmt.Errorf("failed to stage changes: %w", err)
	}

	// Commit the changes
	commitHash, err := worktree.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  authorName,
			Email: "mcp-server@localhost", // Placeholder email
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to commit changes: %w", err)
	}

	slog.Debug("Successfully committed changes", "workspaceId", workspaceID, "commit", commitHash.String())
	return commitHash.String(), nil
}

// List returns a slice of all workspaces.
func (m *Manager) List() ([]Workspace, error) {
	entries, err := os.ReadDir(m.rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read workspaces root directory: %w", err)
	}

	var workspaces []Workspace
	for _, entry := range entries {
		if entry.IsDir() {
			// Basic check to see if it's a git repository
			_, err := git.PlainOpen(filepath.Join(m.rootPath, entry.Name()))
			if err == nil {
				workspaces = append(workspaces, Workspace{
					Name: entry.Name(),
					Path: filepath.Join(m.rootPath, entry.Name()),
				})
			}
		}
	}
	return workspaces, nil
}

// HeadCommit returns the current HEAD commit hash for the workspace repository.
// If the repository has no commits yet, it returns an empty string without error.
func (m *Manager) HeadCommit(workspaceID string) (string, error) {
	workspacePath := filepath.Join(m.rootPath, workspaceID)
	repo, err := git.PlainOpen(workspacePath)
	if err != nil {
		return "", err
	}
	ref, err := repo.Head()
	if err != nil {
		// No commits yet or HEAD not found
		return "", nil
	}
	return ref.Hash().String(), nil
}
