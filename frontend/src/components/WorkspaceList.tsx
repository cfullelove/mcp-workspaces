import React, { useEffect, useState } from 'react';
import api from '../services/api';
import { Card, CardContent, CardHeader, CardTitle } from './ui/card';

interface Workspace {
  name: string;
  path: string;
}

interface WorkspaceListProps {
  onSelectWorkspace: (workspaceId: string) => void;
  refetch: boolean;
  setRefetch: (refetch: boolean) => void;
  selectedWorkspace: string;
}

const WorkspaceList: React.FC<WorkspaceListProps> = ({ onSelectWorkspace, refetch, setRefetch, selectedWorkspace }) => {
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [error, setError] = useState<string | null>(null);

  const fetchWorkspaces = async () => {
    try {
      const response = await api.listWorkspaces();
      setWorkspaces(response.workspaces || []);
    } catch (err) {
      setError('Failed to fetch workspaces');
    }
  };

  useEffect(() => {
    fetchWorkspaces();
  }, [refetch]);

  if (error) {
    return <p className="text-red-500">{error}</p>;
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Workspaces</CardTitle>
      </CardHeader>
      <CardContent>
        <ul className="space-y-2">
          {workspaces.map((workspace) => (
            <li
              key={workspace.name}
              onClick={() => onSelectWorkspace(workspace.name)}
              className={`cursor-pointer p-2 rounded ${
                selectedWorkspace === workspace.name ? 'bg-blue-100' : 'hover:bg-gray-100'
              }`}
            >
              {workspace.name}
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
};

export default WorkspaceList;
