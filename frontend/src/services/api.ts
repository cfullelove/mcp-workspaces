const API_BASE_URL = "/api";

// In a real application, this would be stored securely (e.g., in localStorage or a cookie)
let authToken: string | null = null;

export class ApiError extends Error {
  status: number;
  body: string;
  constructor(status: number, body: string) {
    super(`API error: ${status} ${body}`);
    this.status = status;
    this.body = body;
  }
}

const api = {
  setAuthToken(token: string) {
    authToken = token;
  },

  async post(endpoint: string, data: any) {
    // if (!authToken) {
    //   throw new Error('Auth token not set');
    // }

    const response = await fetch(`${API_BASE_URL}/${endpoint}`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify(data),
    });

    if (!response.ok) {
      const bodyText = await response.text().catch(() => "");
      throw new ApiError(response.status, bodyText || response.statusText);
    }

    return response.json();
  },

  async createWorkspace(name: string): Promise<{ workspaceId: string; path: string }> {
    return this.post('tools/workspace_create', { name });
  },

  async listWorkspaces(): Promise<{ workspaces: { name: string; path: string }[] }> {
    return this.post('tools/workspace_list', {});
  },

  async writeFile(
    workspaceId: string,
    path: string,
    content: string,
    opts?: { ifMatchFileEtag?: string; ifMatchWorkspaceHead?: string }
  ): Promise<any> {
    const payload: any = { workspaceId, path, content };
    if (opts?.ifMatchFileEtag) payload.ifMatchFileEtag = opts.ifMatchFileEtag;
    if (opts?.ifMatchWorkspaceHead) payload.ifMatchWorkspaceHead = opts.ifMatchWorkspaceHead;
    return this.post("tools/fs_write_file", payload);
  },

  async readFile(
    workspaceId: string,
    path: string
  ): Promise<{ content: string; etag?: string; mtime?: string; workspaceHead?: string; totalLines?: number }> {
    return this.post("tools/fs_read_text_file", { workspaceId, path });
  },

  async createDirectory(workspaceId: string, path: string): Promise<any> {
    return this.post('tools/fs_create_directory', { workspaceId, path });
  },

  async listDirectory(workspaceId: string, path: string): Promise<{ entries: string[] }> {
    return this.post('tools/fs_list_directory', { workspaceId, path });
  },

  async deleteFile(workspaceId: string, path: string): Promise<any> {
    return this.post('tools/fs_delete_file', { workspaceId, path });
  },

  async moveFile(workspaceId: string, source: string, destination: string): Promise<any> {
    return this.post('tools/fs_move_file', { workspaceId, source, destination });
  },

  async getFileHistory(
    workspaceId: string,
    path: string,
    limit?: number
  ): Promise<{ log: { commit: string; author: string; date: string; message: string; parent?: string }[] }> {
    const payload: any = { workspaceId, path };
    if (typeof limit === 'number') payload.limit = limit;
    return this.post('tools/fs_get_commit_history', payload);
  },

  async readFileAtCommit(
    workspaceId: string,
    path: string,
    commit: string
  ): Promise<{ content: string; commit: string }> {
    return this.post('tools/fs_read_file_at_commit', { workspaceId, path, commit });
  },
};

export default api;
