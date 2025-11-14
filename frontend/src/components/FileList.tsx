import React, { useEffect, useState } from 'react';
import api from '../services/api';
import { Card, CardContent, CardHeader, CardTitle } from './ui/card';
import FileTree from './FileTree';
import { Button } from './ui/button';
import { FilePlus, FolderPlus } from 'lucide-react';

interface FileListProps {
  workspaceId: string;
  onSelectFile: (filePath: string) => void;
  refetch: boolean;
  setRefetch: (refetch: boolean) => void;
  openNewFile: () => void;
  openNewDir: () => void;
}

const FileList: React.FC<FileListProps> = ({ workspaceId, onSelectFile, refetch, setRefetch, openNewFile, openNewDir }) => {
  const [files, setFiles] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);

  const fetchFiles = async () => {
    try {
      const response = await api.listDirectory(workspaceId, '.');
      setFiles(response.entries || []);
    } catch (err) {
      setError('Failed to fetch files');
    }
  };

  useEffect(() => {
    if (!workspaceId) return;
    fetchFiles();
  }, [workspaceId, refetch]);

  if (error) {
    return <p className="text-red-500">{error}</p>;
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle>Files</CardTitle>
        <div className="space-x-2">
          <Button variant="outline" size="sm" onClick={openNewFile}>
            <FilePlus size={16} className="mr-2" />
            New File
          </Button>
          <Button variant="outline" size="sm" onClick={openNewDir}>
            <FolderPlus size={16} className="mr-2" />
            New Directory
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {files.map((file) => (
          <FileTree
            key={file}
            workspaceId={workspaceId}
            entry={file}
            parentPath=""
            onSelectFile={onSelectFile}
            level={0}
            refetch={() => setRefetch(!refetch)}
          />
        ))}
      </CardContent>
    </Card>
  );
};

export default FileList;
