import type { FilesystemBrowseResponse } from '@porticomediaserver/client-core';
import {
  ArrowUp,
  ChevronRight,
  CircleAlert,
  Folder,
  FolderLock,
  FolderPlus,
  HardDrive,
  LoaderCircle,
  RefreshCw,
  ServerOff,
  X,
} from '#portico-icons';
import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent, type KeyboardEvent as ReactKeyboardEvent } from 'react';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { ModalOverlay } from '../../components/overlay/OverlayPortal';
import {
  filesystemBreadcrumbs,
  filesystemPathLabel,
  isAbsoluteFilesystemPath,
  joinFilesystemPath,
  sameFilesystemPath,
  validateNewFolderName,
} from './filesystemPath';
import { classifyFilesystemFailure, type FilesystemFailure, type FilesystemPickerSource } from './filesystemSource';
import './filesystem-picker.css';

export interface FilesystemPickerDialogProps {
  source: FilesystemPickerSource;
  initialPath?: string;
  title?: string;
  description?: string;
  confirmLabel?: string;
  canCreateDirectory?: boolean;
  onCancel: () => void;
  onSelect: (absolutePath: string) => void;
}

function isAbort(reason: unknown) {
  return reason instanceof DOMException && reason.name === 'AbortError';
}

function validateBrowseResponse(response: FilesystemBrowseResponse) {
  if (response.path && !isAbsoluteFilesystemPath(response.path)) throw new Error('The server returned a folder path that is not absolute.');
  return response;
}

function FailureIcon({ kind }: { kind: FilesystemFailure['kind'] }) {
  if (kind === 'permission') return <FolderLock />;
  if (kind === 'offline') return <ServerOff />;
  return <CircleAlert />;
}

function failureTitle(kind: FilesystemFailure['kind']) {
  if (kind === 'permission') return 'Folder access is unavailable';
  if (kind === 'offline') return 'Server connection lost';
  return 'Folder could not be opened';
}

export function FilesystemPickerDialog({
  source,
  initialPath = '',
  title = 'Choose server folder',
  description = 'Select a folder on the Portico server host.',
  confirmLabel = 'Select folder',
  canCreateDirectory = false,
  onCancel,
  onSelect,
}: FilesystemPickerDialogProps) {
  const [browse, setBrowse] = useState<FilesystemBrowseResponse>();
  const [manualPath, setManualPath] = useState(initialPath.trim());
  const [manualError, setManualError] = useState('');
  const [failure, setFailure] = useState<FilesystemFailure>();
  const [lastRequestedPath, setLastRequestedPath] = useState(initialPath.trim());
  const [loading, setLoading] = useState(true);
  const [newFolderOpen, setNewFolderOpen] = useState(false);
  const [newFolderName, setNewFolderName] = useState('');
  const [createError, setCreateError] = useState('');
  const [creating, setCreating] = useState(false);
  const requestSequence = useRef(0);
  const browseController = useRef<AbortController | undefined>(undefined);
  const createController = useRef<AbortController | undefined>(undefined);
  const folderListRef = useRef<HTMLDivElement>(null);

  const loadPath = useCallback(async (requested?: string, validateManual = false) => {
    const path = requested?.trim() || '';
    if (path && !isAbsoluteFilesystemPath(path)) {
      if (validateManual) setManualError('Enter an absolute server path, such as /srv/media or D:\\Media.');
      setLoading(false);
      return false;
    }
    browseController.current?.abort();
    const controller = new AbortController();
    browseController.current = controller;
    const sequence = ++requestSequence.current;
    setLastRequestedPath(path);
    setLoading(true);
    setFailure(undefined);
    setManualError('');
    setCreateError('');
    try {
      const response = validateBrowseResponse(await source.browse(path || undefined, controller.signal));
      if (sequence !== requestSequence.current || controller.signal.aborted) return false;
      setBrowse(response);
      if (response.path) setManualPath(response.path);
      setNewFolderOpen(false);
      setNewFolderName('');
      return true;
    } catch (reason) {
      if (sequence !== requestSequence.current || controller.signal.aborted || isAbort(reason)) return false;
      setFailure(classifyFilesystemFailure(reason));
      return false;
    } finally {
      if (sequence === requestSequence.current && !controller.signal.aborted) setLoading(false);
    }
  }, [source]);

  useEffect(() => {
    setManualPath(initialPath.trim());
    void loadPath(initialPath.trim() || undefined, Boolean(initialPath.trim()));
    return () => {
      browseController.current?.abort();
      createController.current?.abort();
    };
  }, [initialPath, loadPath]);

  const currentPath = browse?.path?.trim() || '';
  const selectedPath = isAbsoluteFilesystemPath(currentPath) ? currentPath : '';
  const directories = useMemo(() => (browse?.entries ?? [])
    .filter((entry) => entry.kind === 'directory')
    .sort((left, right) => left.name.localeCompare(right.name)), [browse?.entries]);
  const breadcrumbs = useMemo(() => filesystemBreadcrumbs(currentPath), [currentPath]);

  const submitManualPath = (event: FormEvent) => {
    event.preventDefault();
    void loadPath(manualPath, true);
  };

  const createFolder = async (event: FormEvent) => {
    event.preventDefault();
    const name = newFolderName.trim();
    const validationError = validateNewFolderName(name, currentPath);
    if (validationError) {
      setCreateError(validationError);
      return;
    }
    if (!isAbsoluteFilesystemPath(currentPath)) {
      setCreateError('Open a parent folder before creating a new folder.');
      return;
    }
    createController.current?.abort();
    const controller = new AbortController();
    createController.current = controller;
    setCreating(true);
    setCreateError('');
    try {
      const response = validateBrowseResponse(await source.createDirectory(joinFilesystemPath(currentPath, name), controller.signal));
      if (controller.signal.aborted) return;
      setBrowse(response);
      setManualPath(response.path);
      setLastRequestedPath(response.path);
      setFailure(undefined);
      setNewFolderOpen(false);
      setNewFolderName('');
    } catch (reason) {
      if (!controller.signal.aborted && !isAbort(reason)) setCreateError(classifyFilesystemFailure(reason).message);
    } finally {
      if (!controller.signal.aborted) setCreating(false);
    }
  };

  const onFolderListKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'ArrowLeft' && browse?.parent && isAbsoluteFilesystemPath(browse.parent)) {
      event.preventDefault();
      void loadPath(browse.parent);
      return;
    }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    const buttons = Array.from(folderListRef.current?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)') ?? []);
    if (!buttons.length) return;
    event.preventDefault();
    const current = buttons.indexOf(document.activeElement as HTMLButtonElement);
    if (event.key === 'Home') buttons[0]?.focus();
    else if (event.key === 'End') buttons.at(-1)?.focus();
    else {
      const offset = event.key === 'ArrowDown' ? 1 : -1;
      const next = current < 0 ? (offset > 0 ? 0 : buttons.length - 1) : (current + offset + buttons.length) % buttons.length;
      buttons[next]?.focus();
    }
  };

  const roots = browse?.roots ?? [];
  const initialFailure = !browse ? failure : undefined;
  const noFilesystem = !loading && !failure && !currentPath && roots.length === 0;

  return <ModalOverlay labelledBy="filesystem-picker-title" className="filesystem-picker-dialog" onDismiss={onCancel}>
    <header className="filesystem-picker-header">
      <div><h1 id="filesystem-picker-title">{title}</h1><p>{description}</p></div>
      <IconButton label="Close folder picker" onClick={onCancel}><X /></IconButton>
    </header>

    <form className="filesystem-path-form" onSubmit={submitManualPath}>
      <label htmlFor="filesystem-manual-path">Server path</label>
      <div><input id="filesystem-manual-path" value={manualPath} onChange={(event) => { setManualPath(event.target.value); setManualError(''); }} placeholder="/srv/media" spellCheck={false} autoComplete="off" /><SecondaryButton disabled={loading || !manualPath.trim()} onClick={() => void loadPath(manualPath, true)}>Open path</SecondaryButton></div>
      {manualError && <p className="filesystem-inline-error" role="alert">{manualError}</p>}
    </form>

    <div className="filesystem-picker-body">
      <aside className="filesystem-roots" aria-label="Server filesystem roots">
        <header><HardDrive /><span>Server mounts</span></header>
        {roots.length > 0
          ? <div>{roots.map((root) => <button key={root.path} type="button" className={sameFilesystemPath(currentPath, root.path) ? 'selected' : ''} aria-current={sameFilesystemPath(currentPath, root.path) ? 'location' : undefined} disabled={loading} onClick={() => void loadPath(root.path)}><span>{root.name}</span><small>{root.path}</small></button>)}</div>
          : <p>{loading ? 'Reading mounts…' : 'No roots reported'}</p>}
      </aside>

      <main className="filesystem-directory">
        <div className="filesystem-location-bar">
          <nav aria-label="Folder breadcrumbs">
            {breadcrumbs.map((crumb, index) => <span key={`${crumb.path}:${index}`}>{index > 0 && <ChevronRight />}<button type="button" disabled={loading || sameFilesystemPath(currentPath, crumb.path)} aria-current={sameFilesystemPath(currentPath, crumb.path) ? 'location' : undefined} onClick={() => void loadPath(crumb.path)}>{crumb.label}</button></span>)}
          </nav>
          <div>
            <IconButton label="Open parent folder" disabled={loading || !browse?.parent} onClick={() => browse?.parent && void loadPath(browse.parent)}><ArrowUp /></IconButton>
            <IconButton label="Refresh current folder" disabled={loading || !currentPath} onClick={() => void loadPath(currentPath)}><RefreshCw className={loading ? 'filesystem-spin' : ''} /></IconButton>
            {canCreateDirectory && <SecondaryButton disabled={loading || creating || !selectedPath} selected={newFolderOpen} onClick={() => { setNewFolderOpen((open) => !open); setCreateError(''); }}><FolderPlus /> New folder</SecondaryButton>}
          </div>
        </div>

        <div className="filesystem-current-path" aria-live="polite"><Folder /><span><strong>{currentPath ? filesystemPathLabel(currentPath) : 'No folder open'}</strong><span>{currentPath || 'The server did not return a current path.'}</span></span>{currentPath && <small>{directories.length} folder{directories.length === 1 ? '' : 's'}</small>}</div>

        {newFolderOpen && canCreateDirectory && <form className="filesystem-new-folder" onSubmit={createFolder}>
          <label htmlFor="filesystem-new-folder-name">New folder inside {filesystemPathLabel(currentPath)}</label>
          <div><input id="filesystem-new-folder-name" value={newFolderName} onChange={(event) => { setNewFolderName(event.target.value); setCreateError(''); }} disabled={creating} placeholder="Folder name" autoFocus /><SecondaryButton disabled={creating} onClick={() => { setNewFolderOpen(false); setNewFolderName(''); setCreateError(''); }}>Cancel</SecondaryButton><PrimaryButton type="submit" disabled={creating || !newFolderName.trim()}>{creating ? <LoaderCircle className="filesystem-spin" /> : <FolderPlus />} {creating ? 'Creating' : 'Create'}</PrimaryButton></div>
          {createError && <p className="filesystem-inline-error" role="alert">{createError}</p>}
        </form>}

        {failure && browse && <div className={`filesystem-notice ${failure.kind}`} role="alert"><FailureIcon kind={failure.kind} /><span><strong>{failureTitle(failure.kind)}</strong><span>{failure.message}</span></span><SecondaryButton onClick={() => void loadPath(lastRequestedPath || undefined)}><RefreshCw /> Try again</SecondaryButton></div>}

        <div ref={folderListRef} className="filesystem-folder-list" role="list" aria-label={currentPath ? `Folders in ${currentPath}` : 'Server folders'} aria-busy={loading} onKeyDown={onFolderListKeyDown}>
          {loading && !browse && <div className="filesystem-state" aria-live="polite"><LoaderCircle className="filesystem-spin" /><strong>Loading server folders</strong><p>Reading paths from the Portico server host.</p></div>}
          {initialFailure && <div className={`filesystem-state ${initialFailure.kind}`} role="alert"><FailureIcon kind={initialFailure.kind} /><strong>{failureTitle(initialFailure.kind)}</strong><p>{initialFailure.message}</p><SecondaryButton onClick={() => void loadPath(lastRequestedPath || undefined)}><RefreshCw /> Try again</SecondaryButton></div>}
          {noFilesystem && <div className="filesystem-state"><HardDrive /><strong>No server roots available</strong><p>Enter an absolute path above, or check that the server can read its host filesystem.</p></div>}
          {!loading && browse && currentPath && directories.length === 0 && <div className="filesystem-state empty"><Folder /><strong>No folders here</strong><p>You can select this folder, open its parent, or create a child folder if your account allows it.</p></div>}
          {browse && directories.map((entry) => <div className="filesystem-folder-row" role="listitem" key={entry.path}><button type="button" disabled={loading || !entry.readable} aria-label={entry.readable ? `Open folder ${entry.name}` : `Folder ${entry.name} is not readable`} onClick={() => entry.readable && void loadPath(entry.path)}><span className="filesystem-folder-icon">{entry.readable ? <Folder /> : <FolderLock />}</span><span><strong>{entry.name}</strong><span>{entry.path}</span></span><small>{entry.readable ? 'Open' : 'Not readable'}</small><ChevronRight /></button></div>)}
          {loading && browse && <div className="filesystem-loading-overlay" aria-live="polite"><LoaderCircle className="filesystem-spin" /> Opening folder</div>}
        </div>
      </main>
    </div>

    <footer className="filesystem-picker-footer">
      <div><span>Selected server folder</span><strong>{selectedPath || 'No validated folder selected'}</strong></div>
      <SecondaryButton onClick={onCancel}>Cancel</SecondaryButton>
      <PrimaryButton disabled={!selectedPath || loading || creating} onClick={() => selectedPath && onSelect(selectedPath)}>{confirmLabel}</PrimaryButton>
    </footer>
  </ModalOverlay>;
}
