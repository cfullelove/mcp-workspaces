import React, { useState, useEffect } from 'react';
import api from '../services/api';
import { Folder, File, ChevronRight, ChevronDown, Edit, Trash, Check, X } from 'lucide-react';
import { Button } from './ui/button';
import { Input } from './ui/input';

interface FileTreeProps {
  workspaceId: string;
  entry: string;
  parentPath: string;
  onSelectFile: (filePath: string) => void;
  onSelectDirectory?: (dirPath: string) => void;
  selectedPath?: string;
  selectedType?: 'file' | 'dir' | '';
  createTargetPath?: string;
  createType?: 'file' | 'dir' | '';
  onCreateInline?: (name: string, dirPath: string, type: 'file' | 'dir') => void;
  onCancelCreateInline?: () => void;
  level: number;
  refetch: () => void;
}

const FileTree: React.FC<FileTreeProps> = ({ workspaceId, entry, parentPath, onSelectFile, onSelectDirectory, selectedPath, selectedType, createTargetPath, createType, onCreateInline, onCancelCreateInline, level, refetch }) => {
  const [entries, setEntries] = useState<string[]>([]);
  const [isOpen, setIsOpen] = useState(false);
  const [isRenaming, setIsRenaming] = useState(false);
  const [newName, setNewName] = useState('');
  const [createName, setCreateName] = useState('');

  const isDirectory = (entryStr: string) => entryStr.startsWith('[DIR]');
  const getEntryName = (entryStr: string) => entryStr.substring(entryStr.indexOf(' ') + 1);
  const entryName = getEntryName(entry);
  const fullPath = parentPath ? `${parentPath}/${entryName}` : entryName;

  const fetchEntries = async () => {
    if (!isDirectory(entry)) return;
    try {
      const response = await api.listDirectory(workspaceId, fullPath);
      setEntries(response.entries || []);
    } catch (error) {
      console.error(`Failed to fetch entries for ${fullPath}`, error);
    }
  };

  useEffect(() => {
    if (isOpen) {
      fetchEntries();
    }
  }, [isOpen, refetch]);

  useEffect(() => {
    if (createTargetPath === fullPath && createType && !isOpen) {
      setIsOpen(true);
    }
  }, [createTargetPath, createType]);

  const handleToggle = () => {
    setIsOpen(!isOpen);
    if (onSelectDirectory) {
      onSelectDirectory(fullPath);
    }
  };

  const handleSelect = () => {
    onSelectFile(`[FILE] ${fullPath}`);
  };

  const handleDelete = async () => {
    if (confirm(`Are you sure you want to delete ${entryName}?`)) {
      try {
        await api.deleteFile(workspaceId, fullPath);
        refetch();
      } catch (err) {
        console.error('Failed to delete file', err);
      }
    }
  };

  const handleRename = async () => {
    if (!newName || newName === entryName) {
      setIsRenaming(false);
      return;
    }
    try {
      const newFullPath = parentPath ? `${parentPath}/${newName}` : newName;
      await api.moveFile(workspaceId, fullPath, newFullPath);
      refetch();
      setIsRenaming(false);
    } catch (err) {
      console.error('Failed to rename file', err);
    }
  };

  const startRenaming = (e: React.MouseEvent) => {
    e.stopPropagation();
    setNewName(entryName);
    setIsRenaming(true);
  };

  const itemContent = (
    <div className="flex items-center justify-between w-full group" onClick={isDirectory(entry) ? handleToggle : handleSelect}>
      <div className="flex items-center">
        {isDirectory(entry) && (
          <>
            {isOpen ? <ChevronDown size={16} className="mr-2" /> : <ChevronRight size={16} className="mr-2" />}
            <Folder size={16} className="mr-2 text-blue-500" />
          </>
        )}
        {!isDirectory(entry) && <File size={16} className="mr-2 ml-6 text-gray-500" />}
        <span>{entryName}</span>
      </div>
      <div className="hidden group-hover:flex items-center space-x-2">
        <Button variant="ghost" size="icon" onClick={startRenaming}>
          <Edit size={16} />
        </Button>
        <Button variant="ghost" size="icon" onClick={(e) => { e.stopPropagation(); handleDelete(); }}>
          <Trash size={16} />
        </Button>
      </div>
    </div>
  );

  const renamingContent = (
    <div className="flex items-center w-full">
      <Input
        type="text"
        value={newName}
        onChange={(e) => setNewName(e.target.value)}
        onBlur={handleRename}
        onKeyDown={(e) => e.key === 'Enter' && handleRename()}
        autoFocus
        className="h-8"
        onClick={(e) => e.stopPropagation()}
      />
    </div>
  );

  if (isDirectory(entry)) {
    return (
      <div>
        <div
          className={`flex items-center cursor-pointer p-1 h-10 rounded hover:bg-gray-100 ${selectedType === 'dir' && selectedPath === fullPath ? 'bg-blue-100' : ''}`}
          style={{ paddingLeft: `${level * 1.5}rem` }}
        >
          {isRenaming ? renamingContent : itemContent}
        </div>
        {isOpen && (
          <div>
            {createTargetPath === fullPath && createType && (
              <div
                className="flex items-center p-1"
                style={{ paddingLeft: `${(level + 1) * 1.5}rem` }}
                onClick={(e) => e.stopPropagation()}
              >
                <Input
                  type="text"
                  value={createName}
                  onChange={(e) => setCreateName(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && onCreateInline && createName.trim()) {
                      onCreateInline(createName.trim(), fullPath, createType);
                      setCreateName('');
                    }
                    if (e.key === 'Escape') {
                      onCancelCreateInline && onCancelCreateInline();
                      setCreateName('');
                    }
                  }}
                  autoFocus
                  className="h-8"
                />
                <div className="ml-2 flex items-center space-x-1">
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => {
                      if (onCreateInline && createName.trim()) {
                        onCreateInline(createName.trim(), fullPath, createType);
                        setCreateName('');
                      }
                    }}
                    disabled={!createName.trim()}
                    aria-label="Create"
                  >
                    <Check className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => {
                      onCancelCreateInline && onCancelCreateInline();
                      setCreateName('');
                    }}
                    aria-label="Cancel"
                  >
                    <X className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            )}
            {entries.map((childEntry) => (
              <FileTree
                key={childEntry}
                workspaceId={workspaceId}
                entry={childEntry}
                parentPath={fullPath}
                onSelectFile={onSelectFile}
                onSelectDirectory={onSelectDirectory}
                selectedPath={selectedPath}
                selectedType={selectedType}
                createTargetPath={createTargetPath}
                createType={createType}
                onCreateInline={onCreateInline}
                onCancelCreateInline={onCancelCreateInline}
                level={level + 1}
                refetch={refetch}
              />
            ))}
          </div>
        )}
      </div>
    );
  }

  return (
    <div
      className={`flex items-center cursor-pointer p-1 h-10 rounded hover:bg-gray-100 ${selectedType === 'file' && selectedPath === fullPath ? 'bg-blue-100' : ''}`}
      style={{ paddingLeft: `${level * 1.5}rem` }}
    >
      {isRenaming ? renamingContent : itemContent}
    </div>
  );
};

export default FileTree;
