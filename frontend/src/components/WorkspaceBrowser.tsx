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

const WorkspaceBrowser: React.FC = () => {
  const [selectedWorkspace, setSelectedWorkspace] = useState<string>('');
  const [selectedFile, setSelectedFile] = useState<string>('');
  const [newWorkspaceName, setNewWorkspaceName] = useState<string>('');
  const [newFileName, setNewFileName] = useState<string>('');
  const [newFileContent, setNewFileContent] = useState<string>('');
  const [newDirName, setNewDirName] = useState<string>('');
  const [refetchWorkspaces, setRefetchWorkspaces] = useState<boolean>(false);
  const [refetchFiles, setRefetchFiles] = useState<boolean>(false);

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
    } catch (err) {
      console.error('Failed to create directory', err);
    }
  };

  return (
    <div className="container mx-auto p-4">
      <h1 className="text-3xl font-bold mb-4">Workspace Browser</h1>
      <Card className="mb-4">
        <CardHeader>
          <CardTitle>Create New Workspace</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex space-x-2">
            <Input
              type="text"
              placeholder="Workspace name"
              value={newWorkspaceName}
              onChange={(e) => setNewWorkspaceName(e.target.value)}
            />
            <Button onClick={handleCreateWorkspace}>Create Workspace</Button>
          </div>
        </CardContent>
      </Card>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div>
          <WorkspaceList
            onSelectWorkspace={setSelectedWorkspace}
            refetch={refetchWorkspaces}
            setRefetch={setRefetchWorkspaces}
          />
          {selectedWorkspace && (
            <div className="mt-4">
              <FileList
                workspaceId={selectedWorkspace}
                onSelectFile={handleSelectFile}
                refetch={refetchFiles}
                setRefetch={setRefetchFiles}
              />
            </div>
          )}
        </div>
        <div>
          {selectedWorkspace && (
            <div className="space-y-4">
              <Card>
                <CardHeader>
                  <CardTitle>Create New File</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="space-y-2">
                    <Label htmlFor="fileName">File Name</Label>
                    <Input
                      id="fileName"
                      type="text"
                      placeholder="File name"
                      value={newFileName}
                      onChange={(e) => setNewFileName(e.target.value)}
                    />
                    <Label htmlFor="fileContent">File Content</Label>
                    <Textarea
                      id="fileContent"
                      placeholder="File content"
                      value={newFileContent}
                      onChange={(e) => setNewFileContent(e.target.value)}
                    />
                    <Button onClick={handleCreateFile}>Create File</Button>
                  </div>
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardTitle>Create New Directory</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="flex space-x-2">
                    <Input
                      type="text"
                      placeholder="Directory name"
                      value={newDirName}
                      onChange={(e) => setNewDirName(e.target.value)}
                    />
                    <Button onClick={handleCreateDirectory}>Create Directory</Button>
                  </div>
                </CardContent>
              </Card>
            </div>
          )}
        </div>
      </div>
      {selectedWorkspace && selectedFile && (
        <div className="mt-4">
          <FileEditor workspaceId={selectedWorkspace} filePath={selectedFile} />
        </div>
      )}
    </div>
  );
};

export default WorkspaceBrowser;
