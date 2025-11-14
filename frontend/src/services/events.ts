export type WorkspaceEvent = {
  id: number;
  ts: string;
  workspaceId: string;
  type:
    | "file.created"
    | "file.updated"
    | "file.deleted"
    | "file.moved"
    | "dir.created"
    | "dir.deleted"
    | "presence.join"
    | "presence.leave"
    | "lock.acquired"
    | "lock.released";
  path: string;
  prevPath?: string;
  isDir: boolean;
  size?: number;
  mtime?: string;
  actor?: { kind: "mcp" | "api" | "fswatch" | "user"; id?: string; display?: string };
  commit?: string;
  correlationId?: string;
};

export type WorkspaceEventListener = (evt: WorkspaceEvent) => void;

export function createWorkspaceEventSource(
  baseUrl: string,
  workspaceId: string,
  options?: {
    token?: string | null;
    since?: number | null;
    onEvent?: WorkspaceEventListener;
    onError?: (err: any) => void;
  }
): EventSource {
  const params = new URLSearchParams();
  params.set("workspaceId", workspaceId);
  if (options?.since != null) {
    params.set("since", String(options.since));
  }
  if (options?.token) {
    params.set("token", options.token);
  }
  // baseUrl should be the origin of the backend (e.g., window.location.origin)
  const url = `${baseUrl.replace(/\/$/, "")}/events?${params.toString()}`;
  const es = new EventSource(url);

  es.addEventListener("workspace.event", (e: MessageEvent) => {
    try {
      const data: WorkspaceEvent = JSON.parse(e.data);
      options?.onEvent && options.onEvent(data);
    } catch (err) {
      // no-op
    }
  });

  es.onerror = (err) => {
    options?.onError && options.onError(err);
  };

  return es;
}
