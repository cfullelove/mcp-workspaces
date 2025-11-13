import React, { useState, useEffect } from 'react';
import FileEditor from './FileEditor';
import FileTree from './FileTree';
import api from '../services/api';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogClose,
} from "./ui/dialog";
import { Plus, FilePlus, FolderPlus, Check, X } from 'lucide-react';

interface WorkspaceBrowserProps {
  onLogout: () => void;
}

const WorkspaceBrowser: React.FC<WorkspaceBrowserProps> = ({ onLogout }) => {
  const [selectedWorkspace, setSelectedWorkspace] = useState<string>('');
  const [selectedFile, setSelectedFile] = useState<string>('');
  const [refetchWorkspaces, setRefetchWorkspaces] = useState<boolean>(false);
  const [refetchFiles, setRefetchFiles] = useState<boolean>(false);
  const [selectedDirectory, setSelectedDirectory] = useState<string>('');
  const [selectedPath, setSelectedPath] = useState<string>('');
  const [selectedType, setSelectedType] = useState<'file' | 'dir' | ''>('');
  // Inline create state
  const [pendingCreateType, setPendingCreateType] = useState<'' | 'file' | 'dir'>('');
  const [pendingCreateDir, setPendingCreateDir] = useState<string>('');
  const [rootCreateName, setRootCreateName] = useState<string>('');

  // Workspaces dropdown + create dialog
  const [workspaces, setWorkspaces] = useState<{ name: string; path: string }[]>([]);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [newWorkspaceName, setNewWorkspaceName] = useState<string>('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  // Root entries for FileTree
  const [rootEntries, setRootEntries] = useState<string[]>([]);

  const handleSelectFile = (filePath: string) => {
    const isFile = filePath.startsWith('[FILE]');
    if (isFile) {
      const rel = filePath.substring(filePath.indexOf(' ') + 1);
      setSelectedFile(rel);
      setSelectedType('file');
      setSelectedPath(rel);
      const lastSlash = rel.lastIndexOf('/');
      const parent = lastSlash > -1 ? rel.substring(0, lastSlash) : '';
      setSelectedDirectory(parent);
    } else {
      setSelectedFile('');
    }
  };

  // Fetch list of workspaces for dropdown
  useEffect(() => {
    const fetchWorkspaces = async () => {
      try {
        const response = await api.listWorkspaces();
        setWorkspaces(response.workspaces || []);
      } catch (err) {
        console.error('Failed to fetch workspaces', err);
      }
    };
    fetchWorkspaces();
  }, [refetchWorkspaces]);

  // Fetch root entries for FileTree when workspace changes or refetchFiles flips
  useEffect(() => {
    const fetchRootEntries = async () => {
      if (!selectedWorkspace) {
        setRootEntries([]);
        return;
      }
      try {
        const response = await api.listDirectory(selectedWorkspace, '.');
        setRootEntries(response.entries || []);
      } catch (err) {
        console.error('Failed to fetch root entries', err);
      }
    };
    fetchRootEntries();
  }, [selectedWorkspace, refetchFiles]);

  const handleCreateWorkspace = async (e?: React.FormEvent) => {
    e?.preventDefault();
    if (!newWorkspaceName.trim()) return;
    try {
      setIsSubmitting(true);
      await api.createWorkspace(newWorkspaceName.trim());
      setNewWorkspaceName('');
      setIsCreateOpen(false);
      setRefetchWorkspaces(!refetchWorkspaces);
    } catch (err) {
      console.error('Failed to create workspace', err);
    } finally {
      setIsSubmitting(false);
    }
  };




  return (
    <div className="flex h-screen bg-gray-50">
      {/* Left Sidebar */}
      <div className="w-1/4 flex flex-col bg-white border-r">
        <div className="p-4 border-b">
          <div className="flex items-center gap-2">
            <select
              className="border rounded px-2 py-1 text-sm flex-1"
              value={selectedWorkspace}
              onChange={(e) => {
                setSelectedWorkspace(e.target.value);
                setSelectedFile('');
                setSelectedDirectory('');
                setSelectedType('');
                setSelectedPath('');
              }}
            >
              <option value="" disabled>Select workspace</option>
              {workspaces.map((w) => (
                <option key={w.name} value={w.name}>
                  {w.name}
                </option>
              ))}
            </select>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => setIsCreateOpen(true)}
              aria-label="Create workspace"
            >
              <Plus className="h-4 w-4" />
            </Button>
            <Button variant="outline" size="sm" onClick={onLogout}>Logout</Button>
          </div>
        </div>
        <div className="flex-grow p-4 overflow-y-auto">
          {selectedWorkspace ? (
            <>
              <div className="flex items-center justify-between mb-2">
                <div className="space-x-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      const base = selectedDirectory || (selectedFile ? selectedFile.substring(0, selectedFile.lastIndexOf('/')) : '');
                      setPendingCreateType('file');
                      setPendingCreateDir(base || '');
                      if (!base) {
                        setRootCreateName('');
                      }
                    }}
                  >
                    <FilePlus className="mr-2 h-4 w-4" />
                    New File
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      const base = selectedDirectory || (selectedFile ? selectedFile.substring(0, selectedFile.lastIndexOf('/')) : '');
                      setPendingCreateType('dir');
                      setPendingCreateDir(base || '');
                      if (!base) {
                        setRootCreateName('');
                      }
                    }}
                  >
                    <FolderPlus className="mr-2 h-4 w-4" />
                    New Directory
                  </Button>
                </div>
              </div>
              <div>
                {pendingCreateType && pendingCreateDir === '' && (
                  <div
                    className="flex items-center p-1"
                    onClick={(e) => e.stopPropagation()}
                  >
                    <Input
                      type="text"
                      value={rootCreateName}
                      onChange={(e) => setRootCreateName(e.target.value)}
                      onKeyDown={async (e) => {
                        if (e.key === 'Enter' && selectedWorkspace && rootCreateName.trim()) {
                          try {
                            if (pendingCreateType === 'file') {
                              await api.writeFile(selectedWorkspace, rootCreateName.trim(), '');
                            } else if (pendingCreateType === 'dir') {
                              await api.createDirectory(selectedWorkspace, rootCreateName.trim());
                            }
                            setRootCreateName('');
                            setPendingCreateType('');
                            setPendingCreateDir('');
                            setRefetchFiles(!refetchFiles);
                          } catch (err) {
                            console.error('Failed to create at root', err);
                          }
                        }
                        if (e.key === 'Escape') {
                          setPendingCreateType('');
                          setPendingCreateDir('');
                          setRootCreateName('');
                        }
                      }}
                      autoFocus
                      className="h-8"
                    />
                    <div className="ml-2 flex items-center space-x-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={async () => {
                          if (!selectedWorkspace || !rootCreateName.trim()) return;
                          try {
                            if (pendingCreateType === 'file') {
                              await api.writeFile(selectedWorkspace, rootCreateName.trim(), '');
                            } else if (pendingCreateType === 'dir') {
                              await api.createDirectory(selectedWorkspace, rootCreateName.trim());
                            }
                            setRootCreateName('');
                            setPendingCreateType('');
                            setPendingCreateDir('');
                            setRefetchFiles(!refetchFiles);
                          } catch (err) {
                            console.error('Failed to create at root', err);
                          }
                        }}
                        disabled={!rootCreateName.trim()}
                        aria-label="Create"
                      >
                        <Check className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => {
                          setPendingCreateType('');
                          setPendingCreateDir('');
                          setRootCreateName('');
                        }}
                        aria-label="Cancel"
                      >
                        <X className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                )}
                {rootEntries.map((entry) => (
                  <FileTree
                    key={entry}
                    workspaceId={selectedWorkspace}
                    entry={entry}
                    parentPath=""
                    onSelectFile={handleSelectFile}
                    onSelectDirectory={(dir) => { setSelectedDirectory(dir); setSelectedType('dir'); setSelectedPath(dir); }}
                    selectedPath={selectedPath}
                    selectedType={selectedType}
                    createTargetPath={pendingCreateDir}
                    createType={pendingCreateType}
                    onCreateInline={async (name, dirPath, type) => {
                      if (!selectedWorkspace) return;
                      try {
                        if (type === 'file') {
                          await api.writeFile(selectedWorkspace, `${dirPath}/${name}`, '');
                        } else if (type === 'dir') {
                          await api.createDirectory(selectedWorkspace, `${dirPath}/${name}`);
                        }
                        setPendingCreateType('');
                        setPendingCreateDir('');
                        setRefetchFiles(!refetchFiles);
                      } catch (err) {
                        console.error('Failed to create item', err);
                      }
                    }}
                    onCancelCreateInline={() => {
                      setPendingCreateType('');
                      setPendingCreateDir('');
                    }}
                    level={0}
                    refetch={() => setRefetchFiles(!refetchFiles)}
                  />
                ))}
              </div>
            </>
          ) : (
            <p className="text-gray-500">Select a workspace</p>
          )}
        </div>
      </div>

      {/* Main Content */}
      <div className="w-3/4 flex flex-col">
        {selectedWorkspace ? (
          <div className="flex flex-col h-full">
            <div className="flex-grow p-4 overflow-y-auto">
              {selectedFile ? (
                <FileEditor workspaceId={selectedWorkspace} filePath={selectedFile} />
              ) : (
                <p className="text-gray-500">Select a file from the tree</p>
              )}
            </div>
          </div>
        ) : (
          <div className="flex items-center justify-center h-full">
            <p className="text-gray-500">Select a workspace to get started</p>
          </div>
        )}
      </div>

      {/* Dialogs */}
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
            <div className="flex justify-end gap-2">
              <DialogClose asChild>
                <Button type="button" variant="outline" disabled={isSubmitting}>Cancel</Button>
              </DialogClose>
              <Button type="submit" disabled={!newWorkspaceName.trim() || isSubmitting}>Create</Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>


    </div>
  );
};

export default WorkspaceBrowser;
