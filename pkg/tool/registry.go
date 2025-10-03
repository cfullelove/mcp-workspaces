package tool

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"mcp-workspace-manager/pkg/mcp"
)

// HandlerFunc defines the signature for a tool handler function.
// It takes the MCP request parameters and returns a result or an error.
type HandlerFunc func(params []byte) (interface{}, *mcp.Error)

// Registry holds a map of tool names to their handler functions.
type Registry struct {
	handlers map[string]HandlerFunc
}

// NewRegistry creates a new tool registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]HandlerFunc),
	}
}

// Register adds a new tool handler to the registry.
func (r *Registry) Register(toolName string, handler HandlerFunc) {
	if _, exists := r.handlers[toolName]; exists {
		slog.Warn("Overwriting an existing tool handler", "tool", toolName)
	}
	r.handlers[toolName] = handler
	slog.Debug("Registered tool handler", "tool", toolName)
}

// Dispatch finds the appropriate handler for a request and executes it.
func (r *Registry) Dispatch(req *mcp.Request) *mcp.Response {
	handler, found := r.handlers[req.Tool]
	if !found {
		slog.Warn("No handler found for tool", "tool", req.Tool)
		return &mcp.Response{
			ID:    req.ID,
			Error: mcp.NewError("NOT_FOUND", fmt.Sprintf("Tool '%s' not found", req.Tool), nil),
		}
	}

	// Execute the handler
	result, mcpErr := handler(req.Params)
	if mcpErr != nil {
		return &mcp.Response{
			ID:    req.ID,
			Error: mcpErr,
		}
	}

	// Marshal the result into raw JSON for the response
	resultBytes, err := json.Marshal(result)
	if err != nil {
		slog.Error("Failed to marshal tool result", "error", err, "tool", req.Tool)
		return &mcp.Response{
			ID:    req.ID,
			Error: mcp.NewError("INTERNAL", "Failed to serialize tool result", nil),
		}
	}

	return &mcp.Response{
		ID:     req.ID,
		Result: resultBytes,
	}
}