import React, { useEffect, useState } from 'react';
import api from '../services/api';
import { Button } from './ui/button';
import { Textarea } from './ui/textarea';
import { Card, CardContent, CardHeader, CardTitle } from './ui/card';

interface FileEditorProps {
  workspaceId: string;
  filePath: string;
}

const FileEditor: React.FC<FileEditorProps> = ({ workspaceId, filePath }) => {
  const [content, setContent] = useState<string>('');
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!workspaceId || !filePath) return;

    const fetchContent = async () => {
      try {
        const response = await api.readFile(workspaceId, filePath);
        setContent(response.content);
      } catch (err) {
        setError('Failed to fetch file content');
      }
    };

    fetchContent();
  }, [workspaceId, filePath]);

  const handleSave = async () => {
    try {
      await api.writeFile(workspaceId, filePath, content);
    } catch (err) {
      console.error('Failed to save file', err);
    }
  };

  if (error) {
    return <p className="text-red-500">{error}</p>;
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>File Editor: {filePath}</CardTitle>
      </CardHeader>
      <CardContent>
        <Textarea
          value={content}
          onChange={(e) => setContent(e.target.value)}
          className="h-96"
        />
        <Button variant="outline" onClick={handleSave} className="mt-4">
          Save
        </Button>
      </CardContent>
    </Card>
  );
};

export default FileEditor;
