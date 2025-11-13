const API_BASE_URL = "/api";

// In a real application, this would be stored securely (e.g., in localStorage or a cookie)
let authToken: string | null = null;

const api = {
  setAuthToken(token: string) {
    authToken = token;
  },

  async post(endpoint: string, data: any) {
    // if (!authToken) {
    //   throw new Error('Auth token not set');
    // }

    const response = await fetch(`${API_BASE_URL}/${endpoint}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify(data),
    });

    if (!response.ok) {
      throw new Error(`API error: ${response.status} ${response.statusText}`);
    }

    return response.json();
  },

  async createWorkspace(name: string): Promise<{ workspaceId: string; path: string }> {
    return this.post('tools/workspace_create', { name });
  },

  async listWorkspaces(): Promise<{ workspaces: { name: string; path: string }[] }> {
    return this.post('tools/workspace_list', {});
  },

  async writeFile(workspaceId: string, path: string, content: string): Promise<any> {
    return this.post('tools/fs_write_file', { workspaceId, path, content });
  },

  async readFile(workspaceId: string, path: string): Promise<{ content: string }> {
    return this.post('tools/fs_read_text_file', { workspaceId, path });
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
};

export default api;
