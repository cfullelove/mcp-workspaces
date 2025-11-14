import React, { useEffect, useState } from 'react';
import api from '../services/api';
import { Card, CardContent, CardHeader, CardTitle } from './ui/card';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogClose } from './ui/dialog';
import { Plus } from 'lucide-react';

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

  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [newWorkspaceName, setNewWorkspaceName] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

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

  const handleCreateWorkspace = async (e?: React.FormEvent) => {
    e?.preventDefault();
    if (!newWorkspaceName.trim()) return;
    try {
      setIsSubmitting(true);
      await api.createWorkspace(newWorkspaceName.trim());
      setNewWorkspaceName('');
      setIsCreateOpen(false);
      setFormError(null);
      setRefetch(!refetch);
    } catch (err) {
      setFormError('Failed to create workspace');
    } finally {
      setIsSubmitting(false);
    }
  };

  if (error) {
    return <p className="text-red-500">{error}</p>;
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle>Workspaces</CardTitle>
        <Button
          variant="ghost"
          size="icon"
          onClick={() => setIsCreateOpen(true)}
          aria-label="Create workspace"
        >
          <Plus className="h-4 w-4" />
        </Button>
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
      <Dialog open={isCreateOpen} onOpenChange={setIsCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create New Workspace</DialogTitle>
          </DialogHeader>
          <form onSubmit={handleCreateWorkspace} className="grid gap-4 py-4">
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="workspaceName" className="text-right">Name</Label>
              <Input
                id="workspaceName"
                value={newWorkspaceName}
                onChange={(e) => setNewWorkspaceName(e.target.value)}
                className="col-span-3"
                autoFocus
              />
            </div>
            {formError && <p className="text-red-500 text-sm">{formError}</p>}
            <div className="flex justify-end gap-2">
              <DialogClose asChild>
                <Button type="button" variant="outline" disabled={isSubmitting}>Cancel</Button>
              </DialogClose>
              <Button type="submit" disabled={!newWorkspaceName.trim() || isSubmitting}>Create</Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
    </Card>
  );
};

export default WorkspaceList;
