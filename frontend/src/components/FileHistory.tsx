import React, { useEffect, useMemo, useState } from 'react';
import api from '../services/api';
import { Card, CardContent, CardHeader, CardTitle } from './ui/card';
import { diffLines } from 'diff';
import type { Change } from 'diff';

type CommitItem = {
  commit: string;
  author: string;
  date: string;
  message: string;
  parent?: string;
};

interface FileHistoryProps {
  workspaceId: string;
  filePath: string;
  hideHeader?: boolean;
}

const FileHistory: React.FC<FileHistoryProps> = ({ workspaceId, filePath, hideHeader = false }) => {
  const [commits, setCommits] = useState<CommitItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [selectedCommit, setSelectedCommit] = useState<CommitItem | null>(null);
  const [oldContent, setOldContent] = useState<string>('');
  const [newContent, setNewContent] = useState<string>('');
  const [diffLoading, setDiffLoading] = useState(false);
  const baseName = useMemo(() => (filePath.split('/').pop() || filePath), [filePath]);

  useEffect(() => {
    const run = async () => {
      if (!workspaceId || !filePath) return;
      setLoading(true);
      setError(null);
      try {
        const res = await api.getFileHistory(workspaceId, filePath, 50);
        const log = (res?.log || []) as CommitItem[];
        setCommits(log);
        setSelectedCommit(log[0] || null);
      } catch (e) {
        setError('Failed to load history');
      } finally {
        setLoading(false);
      }
    };
    run();
  }, [workspaceId, filePath]);

  useEffect(() => {
    const loadDiff = async () => {
      if (!selectedCommit) return;
      setDiffLoading(true);
      try {
        const newRes = await api.readFileAtCommit(workspaceId, filePath, selectedCommit.commit);
        let oldText = '';
        if (selectedCommit.parent && selectedCommit.parent.trim() !== '') {
          try {
            const oldRes = await api.readFileAtCommit(workspaceId, filePath, selectedCommit.parent);
            oldText = oldRes.content ?? '';
          } catch {
            // parent might not contain the file (deleted/created); treat as empty
            oldText = '';
          }
        }
        setNewContent(newRes.content ?? '');
        setOldContent(oldText);
      } catch (e) {
        setOldContent('');
        setNewContent('');
      } finally {
        setDiffLoading(false);
      }
    };
    loadDiff();
  }, [selectedCommit, workspaceId, filePath]);

  const parts: Change[] = useMemo(() => {
    try {
      return diffLines(oldContent, newContent);
    } catch {
      return [{ value: 'Unable to compute diff' } as Change];
    }
  }, [oldContent, newContent]);

  return (
    <Card className="h-full flex flex-col">
      {!hideHeader && (
        <CardHeader className="py-3">
          <div className="flex items-center justify-between">
            <CardTitle className="text-base">History: {baseName}</CardTitle>
          </div>
        </CardHeader>
      )}
      <CardContent className="flex-1 min-h-0 flex gap-4 p-4">
        <div className="w-72 shrink-0 pr-4 overflow-y-auto space-y-2">
          {loading ? (
            <p className="text-sm text-gray-500">Loading…</p>
          ) : error ? (
            <p className="text-sm text-red-600">{error}</p>
          ) : commits.length === 0 ? (
            <p className="text-sm text-gray-500">No history.</p>
          ) : (
            <ul className="space-y-2">
              {commits.map((c) => {
                const isSelected = selectedCommit?.commit === c.commit;
                const title = (c.message || '').split('\n')[0] || '(no message)';
                return (
                  <li key={c.commit}>
                    <button
                      type="button"
                      onClick={() => setSelectedCommit(c)}
                      className={[
                        'w-full text-left p-2 rounded-md border transition-colors',
                        isSelected ? 'bg-blue-50 border-blue-300' : 'border-gray-200 hover:bg-gray-50'
                      ].join(' ')}
                    >
                      <div className="text-sm font-medium truncate">{title}</div>
                      <div className="text-[11px] text-gray-600 font-mono truncate">
                        {c.commit.slice(0, 10)} · {new Date(c.date).toLocaleString()}
                      </div>
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
        </div>

        <div className="flex-1 min-w-0 overflow-auto pl-4 border-l">
          {diffLoading ? (
            <p className="text-sm text-gray-500">Loading diff…</p>
          ) : selectedCommit ? (
            <>
              <div className="mb-2 text-xs text-gray-600 font-mono flex items-center gap-2">
                <span>Comparing</span>
                <span className="px-1.5 py-0.5 rounded bg-gray-100">{selectedCommit.parent ? selectedCommit.parent.slice(0, 10) : '∅'}</span>
                <span>→</span>
                <span className="px-1.5 py-0.5 rounded bg-gray-100">{selectedCommit.commit.slice(0, 10)}</span>
              </div>
              <pre className="text-xs whitespace-pre-wrap leading-5 border rounded-md p-3 bg-white">
              {parts.map((p, idx) => {
                const bg = p.added ? 'bg-green-100' : p.removed ? 'bg-red-100' : '';
                const prefix = p.added ? '+ ' : p.removed ? '- ' : '  ';
                return (
                  <span key={idx} className={bg}>
                    {p.value
                      .split('\n')
                      .map((line: string, i: number, arr: string[]) =>
                        i === arr.length - 1 && line === '' ? '' : `${prefix}${line}\n`
                      )
                      .join('')}
                  </span>
                );
              })}
            </pre>
            </>
          ) : (
            <p className="text-sm text-gray-500">Select a commit to view its diff.</p>
          )}
        </div>
      </CardContent>
    </Card>
  );
};

export default FileHistory;
