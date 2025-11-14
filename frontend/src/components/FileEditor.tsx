import React, { useEffect, useState, useRef } from 'react';
import api, { ApiError } from '../services/api';
import type { WorkspaceEvent } from '../services/events';
import { Button } from './ui/button';
import CodeMirror from '@uiw/react-codemirror';
import { javascript } from '@codemirror/lang-javascript';
import { markdown, markdownLanguage } from '@codemirror/lang-markdown';
import { languages } from '@codemirror/language-data';
import { vscodeLight } from '@uiw/codemirror-theme-vscode';
import { python } from '@codemirror/lang-python';
import { go as goLang } from '@codemirror/lang-go';
import { EditorView } from '@codemirror/view';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogClose,
} from "./ui/dialog";
import FileHistory from './FileHistory';

interface FileEditorProps {
  workspaceId: string;
  filePath: string;
  lastEvent?: WorkspaceEvent | null;
}

const FileEditor: React.FC<FileEditorProps> = ({ workspaceId, filePath, lastEvent }) => {
  const [content, setContent] = useState<string>('');
  const [error, setError] = useState<string | null>(null);

  // Concurrency metadata
  const [etag, setEtag] = useState<string | undefined>(undefined);
  const [workspaceHead, setWorkspaceHead] = useState<string | undefined>(undefined);

  // UI state
  const [dirty, setDirty] = useState<boolean>(false);
  const [saving, setSaving] = useState<boolean>(false);
  const [remoteChanged, setRemoteChanged] = useState<boolean>(false);
  const [conflict, setConflict] = useState<{ message?: string } | null>(null);
  const [historyOpen, setHistoryOpen] = useState<boolean>(false);

  const mounted = useRef<boolean>(false);
  const savingRef = useRef<boolean>(false);

  const fetchContent = async () => {
    if (!workspaceId || !filePath) return;
    try {
      const response = await api.readFile(workspaceId, filePath);
      setContent(response.content ?? '');
      setEtag(response.etag);
      setWorkspaceHead(response.workspaceHead);
      setDirty(false);
      setRemoteChanged(false);
      setConflict(null);
      setError(null);
    } catch (err) {
      setError('Failed to fetch file content');
    }
  };

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  // Load file on selection change
  useEffect(() => {
    if (!workspaceId || !filePath) return;
    fetchContent();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceId, filePath]);

  // React to server events for this file
  useEffect(() => {
    if (!lastEvent || !workspaceId || !filePath) return;
    if (lastEvent.workspaceId !== workspaceId) return;
    if (lastEvent.path !== filePath) return;

    // Suppress handling while an explicit save/overwrite is in-flight to avoid false "remote changes" flashes
    if (savingRef.current) return;

    if (lastEvent.type === 'file.updated') {
      if (dirty) {
        // Notify user that remote has changed while editing
        setRemoteChanged(true);
      } else {
        // Auto-refresh if not dirty
        fetchContent();
      }
    } else if (lastEvent.type === 'file.deleted') {
      setError('This file was deleted remotely.');
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lastEvent]);

  const handleSave = async () => {
    if (!workspaceId || !filePath) return;
    savingRef.current = true;
    setSaving(true);
    try {
      await api.writeFile(workspaceId, filePath, content, {
        ifMatchFileEtag: etag,
        ifMatchWorkspaceHead: workspaceHead,
      });
      // Refresh meta after save
      await fetchContent();
    } catch (err: any) {
      if (err instanceof ApiError && err.status === 409) {
        setConflict({ message: err.body || 'Save conflict detected on server.' });
      } else {
        console.error('Failed to save file', err);
      }
    } finally {
      if (mounted.current) setSaving(false);
      savingRef.current = false;
    }
  };

  const handleOverwriteRemote = async () => {
    if (!workspaceId || !filePath) return;
    savingRef.current = true;
    setSaving(true);
    try {
      // Overwrite by omitting preconditions
      await api.writeFile(workspaceId, filePath, content);
      await fetchContent();
    } catch (err) {
      console.error('Failed to overwrite remote', err);
    } finally {
      if (mounted.current) {
        setSaving(false);
        setConflict(null);
        setRemoteChanged(false);
      }
      savingRef.current = false;
    }
  };

  const handleReloadRemote = async () => {
    await fetchContent();
  };

  const leftAlign = EditorView.theme({
    '& .cm-content': { textAlign: 'left' }
  });

  // Ensure internal CodeMirror scroller handles overflow and editor fills its container
  const fullHeight = EditorView.theme({
    '&': { height: '100%' },
    '.cm-scroller': { overflow: 'auto' }
  });

  const cmExtensions = React.useMemo(() => {
    const ext = filePath.split('.').pop()?.toLowerCase();
    let languageExt: any[] = [];
    if (ext === 'md' || ext === 'markdown') {
      languageExt = [markdown({ base: markdownLanguage, codeLanguages: languages })];
    } else if (ext === 'js' || ext === 'jsx' || ext === 'ts' || ext === 'tsx') {
      languageExt = [javascript({ jsx: true, typescript: ext?.startsWith('ts') })];
    } else if (ext === 'py' || ext === 'python') {
      languageExt = [python()];
    } else if (ext === 'go') {
      languageExt = [goLang()];
    }
    return [fullHeight, leftAlign, ...languageExt];
  }, [filePath, leftAlign]);

  if (error) {
    return <p className="text-red-500">{error}</p>;
  }

  return (
    <div className="h-full flex flex-col">
      <div className="px-4 py-4 border-b">
        <div className="flex items-center justify-between">
          <div className="text-lg font-medium">
            Editing: {filePath}
          </div>
          <div className="flex items-center space-x-2">
            {dirty && <span className="text-xs text-orange-600">Unsaved changes</span>}
            <Button variant="outline" size="sm" onClick={() => setHistoryOpen(true)}>
              History
            </Button>
            <Button variant="outline" onClick={handleSave} disabled={saving || !dirty}>
              {saving ? 'Saving...' : 'Save'}
            </Button>
          </div>
        </div>
      </div>
      <div className="flex-1 min-h-0 flex flex-col px-4 py-2">
        {/* Remote change banner (when user has unsaved edits and a remote update arrives) */}
        {remoteChanged && !conflict && (
          <div className="mb-3 p-3 rounded border border-yellow-300 bg-yellow-50 text-yellow-900 text-sm flex items-center justify-between">
            <span>Remote changes detected for this file. Review before saving to avoid overwriting.</span>
            <div className="flex items-center space-x-2">
              <Button variant="outline" size="sm" onClick={handleReloadRemote}>
                Reload remote
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setRemoteChanged(false)}>
                Dismiss
              </Button>
            </div>
          </div>
        )}

        {/* Conflict banner after failed save with 409 */}
        {conflict && (
          <div className="mb-3 p-3 rounded border border-red-300 bg-red-50 text-red-900 text-sm">
            <div className="flex items-center justify-between">
              <span>{conflict.message || 'Save conflict detected. The file has changed on the server.'}</span>
            </div>
            <div className="mt-2 flex items-center space-x-2">
              <Button variant="outline" size="sm" onClick={handleReloadRemote}>
                Reload remote (discard my edits)
              </Button>
              <Button variant="destructive" size="sm" onClick={handleOverwriteRemote}>
                Overwrite remote with my version
              </Button>
            </div>
          </div>
        )}

        <div className="flex-1 min-h-0 w-full">
          <CodeMirror
            value={content}
            onChange={(val) => {
              setContent(val);
              setDirty(true);
            }}
            style={{ height: '100%', width: '100%' }}
            theme={vscodeLight}
            basicSetup={{
              lineNumbers: true,
              highlightActiveLine: true,
              bracketMatching: true,
              autocompletion: true,
            }}
            extensions={cmExtensions}
          />
        </div>
        <div className="mt-2 text-xs text-gray-500 flex items-center justify-between">
          <span>Etag: {etag || '—'}</span>
          <span>HEAD: {workspaceHead || '—'}</span>
        </div>
      </div>

      {/* History Dialog */}
      <Dialog open={historyOpen} onOpenChange={setHistoryOpen}>
        <DialogContent className="flex flex-col max-w-6xl w-[95vw] h-[85vh] p-2 sm:p-3">
          <DialogHeader>
            <DialogTitle>History: {filePath}</DialogTitle>
          </DialogHeader>
          <div className="flex-1 min-h-0">
            <FileHistory workspaceId={workspaceId} filePath={filePath} hideHeader />
          </div>
          <div className="mt-2 flex justify-end">
            <DialogClose asChild>
              <Button variant="outline" size="sm">Close</Button>
            </DialogClose>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default FileEditor;
