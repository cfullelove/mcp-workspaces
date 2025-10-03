package tool

import (
	"encoding/json"
	"mcp-workspace-manager/pkg/mcp"
	"mcp-workspace-manager/pkg/workspace"
)

// CreateWorkspaceRequest defines the expected parameters for the workspace/create tool.
type CreateWorkspaceRequest struct {
	Name string `json:"name"`
}

// CreateWorkspaceResponse defines the output for the workspace/create tool.
type CreateWorkspaceResponse struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}

// RegisterWorkspaceTools registers all workspace-related tools with the given registry.
func RegisterWorkspaceTools(registry *Registry, manager *workspace.Manager) {
	registry.Register("workspace/create", makeCreateWorkspaceHandler(manager))
}

// makeCreateWorkspaceHandler creates a handler for the workspace/create tool.
func makeCreateWorkspaceHandler(manager *workspace.Manager) HandlerFunc {
	return func(params []byte) (interface{}, *mcp.Error) {
		var req CreateWorkspaceRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, mcp.NewError("INVALID_INPUT", "Failed to parse parameters", err.Error())
		}

		if req.Name == "" {
			return nil, mcp.NewError("INVALID_INPUT", "Parameter 'name' cannot be empty", nil)
		}

		workspaceID, path, err := manager.Create(req.Name)
		if err != nil {
			return nil, mcp.NewError("INTERNAL", "Failed to create workspace", err.Error())
		}

		return &CreateWorkspaceResponse{
			WorkspaceID: workspaceID,
			Path:        path,
		}, nil
	}
}