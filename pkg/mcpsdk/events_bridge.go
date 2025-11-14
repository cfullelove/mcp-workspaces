package mcpsdk

import "mcp-workspace-manager/pkg/events"

// eventHub is initialized by RunHTTP (and can be reused by other transports if needed).
var eventHub *events.Hub

// publishWorkspaceEvent safely publishes an event if the hub is initialized.
func publishWorkspaceEvent(workspaceID string, evt events.WorkspaceEvent) {
	if eventHub == nil {
		return
	}
	eventHub.Publish(workspaceID, evt)
}
