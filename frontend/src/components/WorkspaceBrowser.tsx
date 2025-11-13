import React, { useState } from 'react';
import FileEditor from './FileEditor';
import FileList from './FileList';
import WorkspaceList from './WorkspaceList';
import api from '../services/api';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Textarea } from './ui/textarea';
import { Card, CardContent, CardHeader, CardTitle } from './ui/card';
import { Label } from './ui/label';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "./ui/dialog";

interface WorkspaceBrowserProps {
  onLogout: () => void;
}

const WorkspaceBrowser: React.FC<WorkspaceBrowserProps> = ({ onLogout }) => {
  const [selectedWorkspace, setSelectedWorkspace] = useState<string>('');
  const [selectedFile, setSelectedFile] = useState<string>('');
  const [newWorkspaceName, setNewWorkspaceName] = useState<string>('');
  const [newFileName, setNewFileName] = useState<string>('');
  const [newFileContent, setNewFileContent] = useState<string>('');
  const [newDirName, setNewDirName] = useState<string>('');
  const [refetchWorkspaces, setRefetchWorkspaces] = useState<boolean>(false);
  const [refetchFiles, setRefetchFiles] = useState<boolean>(false);
  const [isFileOpen, setIsFileOpen] = useState(false);
  const [isDirOpen, setIsDirOpen] = useState(false);

  const handleSelectFile = (filePath: string) => {
    const isFile = filePath.startsWith('[FILE]');
    if (isFile) {
      setSelectedFile(filePath.substring(filePath.indexOf(' ') + 1));
    } else {
      setSelectedFile('');
    }
  };

  const handleCreateWorkspace = async () => {
    if (!newWorkspaceName) return;
    try {
      await api.createWorkspace(newWorkspaceName);
      setNewWorkspaceName('');
      setRefetchWorkspaces(!refetchWorkspaces);
    } catch (err) {
      console.error('Failed to create workspace', err);
    }
  };

  const handleCreateFile = async () => {
    if (!selectedWorkspace || !newFileName) return;
    try {
      await api.writeFile(selectedWorkspace, newFileName, newFileContent);
      setNewFileName('');
      setNewFileContent('');
      setRefetchFiles(!refetchFiles);
      setIsFileOpen(false);
    } catch (err) {
      console.error('Failed to create file', err);
    }
  };

  const handleCreateDirectory = async () => {
    if (!selectedWorkspace || !newDirName) return;
    try {
      await api.createDirectory(selectedWorkspace, newDirName);
      setNewDirName('');
      setRefetchFiles(!refetchFiles);
      setIsDirOpen(false);
    } catch (err) {
      console.error('Failed to create directory', err);
    }
  };

  return (
    <div className="flex h-screen bg-gray-50">
      {/* Left Sidebar */}
      <div className="w-1/4 flex flex-col bg-white border-r">
        <div className="p-4 border-b">
          <div className="flex justify-between items-center">
            <h1 className="text-xl font-semibold">Workspaces</h1>
            <Button variant="outline" size="sm" onClick={onLogout}>Logout</Button>
          </div>
        </div>
        <div className="p-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Create New Workspace</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex space-x-2">
                <Input
                  type="text"
                  placeholder="Workspace name"
                  value={newWorkspaceName}
                  onChange={(e) => setNewWorkspaceName(e.target.value)}
                />
                <Button onClick={handleCreateWorkspace}>Create</Button>
              </div>
            </CardContent>
          </Card>
        </div>
        <div className="flex-grow p-4 overflow-y-auto">
          <WorkspaceList
            onSelectWorkspace={setSelectedWorkspace}
            refetch={refetchWorkspaces}
            setRefetch={setRefetchWorkspaces}
            selectedWorkspace={selectedWorkspace}
          />
        </div>
      </div>

      {/* Main Content */}
      <div className="w-3/4 flex flex-col">
        {selectedWorkspace ? (
          <div className="flex flex-col h-full">
            <div className="p-4 border-b">
              <FileList
                workspaceId={selectedWorkspace}
                onSelectFile={handleSelectFile}
                refetch={refetchFiles}
                setRefetch={setRefetchFiles}
                openNewFile={() => setIsFileOpen(true)}
                openNewDir={() => setIsDirOpen(true)}
              />
            </div>
            <div className="flex-grow p-4 overflow-y-auto">
              {selectedFile && (
                <FileEditor workspaceId={selectedWorkspace} filePath={selectedFile} />
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
      <Dialog open={isFileOpen} onOpenChange={setIsFileOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create New File</DialogTitle>
          </DialogHeader>
          <div className="grid gap-4 py-4">
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="fileName" className="text-right">Name</Label>
              <Input
                id="fileName"
                value={newFileName}
                onChange={(e) => setNewFileName(e.target.value)}
                className="col-span-3"
              />
            </div>
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="fileContent" className="text-right">Content</Label>
              <Textarea
                id="fileContent"
                value={newFileContent}
                onChange={(e) => setNewFileContent(e.target.value)}
                className="col-span-3"
              />
            </div>
            <Button onClick={handleCreateFile}>Create File</Button>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={isDirOpen} onOpenChange={setIsDirOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create New Directory</DialogTitle>
          </DialogHeader>
          <div className="grid gap-4 py-4">
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="dirName" className="text-right">Name</Label>
              <Input
                id="dirName"
                value={newDirName}
                onChange={(e) => setNewDirName(e.target.value)}
                className="col-span-3"
              />
            </div>
            <Button onClick={handleCreateDirectory}>Create Directory</Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default WorkspaceBrowser;
