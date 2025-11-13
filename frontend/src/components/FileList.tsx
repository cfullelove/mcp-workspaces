import React, { useEffect, useState } from 'react';
import api from '../services/api';
import { Card, CardContent, CardHeader, CardTitle } from './ui/card';
import { Button } from './ui/button';

interface FileListProps {
  workspaceId: string;
  onSelectFile: (filePath: string) => void;
  refetch: boolean;
  setRefetch: (refetch: boolean) => void;
}

const FileList: React.FC<FileListProps> = ({ workspaceId, onSelectFile, refetch, setRefetch }) => {
  const [files, setFiles] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);

  const fetchFiles = async () => {
    try {
      const response = await api.listDirectory(workspaceId, '.');
      setFiles(response.entries);
    } catch (err) {
      setError('Failed to fetch files');
    }
  };

  useEffect(() => {
    if (!workspaceId) return;
    fetchFiles();
  }, [workspaceId, refetch]);

  const handleDelete = async (filePath: string) => {
    try {
      // a file path is prefixed with [FILE] or [DIR], so we need to remove that
      const pathToDelete = filePath.substring(filePath.indexOf(' ') + 1);
      await api.deleteFile(workspaceId, pathToDelete);
      setRefetch(!refetch);
    } catch (err) {
      console.error('Failed to delete file', err);
    }
  };

  const handleRename = async (filePath: string) => {
    const newName = prompt('Enter new name');
    if (!newName) return;
    try {
      const oldPath = filePath.substring(filePath.indexOf(' ') + 1);
      await api.moveFile(workspaceId, oldPath, newName);
      setRefetch(!refetch);
    } catch (err) {
      console.error('Failed to rename file', err);
    }
  };

  if (error) {
    return <p className="text-red-500">{error}</p>;
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Files</CardTitle>
      </CardHeader>
      <CardContent>
        <ul>
          {files.map((file) => (
            <li key={file} className="flex justify-between items-center mb-2">
              <span
                onClick={() => onSelectFile(file)}
                className="cursor-pointer hover:underline"
              >
                {file}
              </span>
              <div>
                <Button variant="outline" size="sm" onClick={() => handleRename(file)} className="mr-2">
                  Rename
                </Button>
                <Button variant="destructive" size="sm" onClick={() => handleDelete(file)}>
                  Delete
                </Button>
              </div>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
};

export default FileList;
