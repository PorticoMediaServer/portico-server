// Controlled transport fixture: actual component and strict filesystem parser, no live file access.
import {StrictMode, useMemo, useState} from 'react';
import {createRoot} from 'react-dom/client';
import type {LibraryContentApi} from '@core/library-content.ts';
import {ServerFolderPicker} from '../../src/kit/settings/ServerFolderPicker';
import {BackupList} from '../../src/kit/settings/BackupList';
import {BackupConfirmationDialog} from '../../src/kit/settings/BackupConfirmationDialog';
import {defaultI18n} from '@i18n';
import {IconSprite} from '../../src/kit/Icon';
import '../../src/kit/tokens.css';
import '../../src/kit/fonts.css';
import '../../src/bootstrap.css';

const folder = '/mnt/media/Backups';
const proof = {requests: [] as {path: string;aborted: boolean}[], choices: [] as readonly string[][], closes: 0, released: 0, restoreOffers: [] as string[], deleteOffers: [] as string[], confirmations: [] as {id: string;aborted: boolean}[], failures: 0};
let release: (() => void) | undefined;
let complete: (() => void) | undefined;
let current = true;const listeners = new Set<() => void>();
const authority = {isCurrent: () => current, subscribeAuthority: (listener: () => void) => {listeners.add(listener);return () => {listeners.delete(listener);};}};
Object.assign(window, {backupPickerProof: proof, releaseBackupPage: () => {release?.();release = undefined;}, completeBackupConfirmation: () => {complete?.();complete = undefined;}, retireBackupAuthority: () => {current = false;for (const listener of listeners) listener();}});
function Fixture() {
  const [open, setOpen] = useState(true), [identity, setIdentity] = useState(0);
  const params = new URLSearchParams(location.search), purpose = params.get('purpose') === 'folder' ? 'folder' as const : 'backup' as const;
  const api = useMemo<LibraryContentApi>(() => ({async request<T>(path: string, _method?: string, _body?: unknown, signal?: AbortSignal): Promise<T> {
    const query = new URL(path, location.origin).searchParams, current = query.get('path') ?? '';
    const seen = {path, aborted: signal?.aborted ?? false};proof.requests.push(seen);
    signal?.addEventListener('abort', () => {seen.aborted = true;}, {once: true});
    if (query.has('cursor')) await new Promise<void>(resolve => {release = () => {proof.released++;resolve();};});
    const entry = (name: string, kind: 'directory' | 'file' | 'unknown', extra = {}) => ({name, path: `${current}/${name}`, kind, readable: true, symlink: false, bytes: kind === 'file' ? 18_000_000 : 0, hidden: false, ...extra});
    const frame = new URLSearchParams(location.search).has('frame');
    const entries = !current ? [] : frame ? [entry('old', 'directory'), entry('portico-2026-10-20-1812.db', 'file', {bytes: 17*1024*1024}), entry('portico-2026-10-21-0300.db', 'file', {bytes: 18*1024*1024})] : current.endsWith('/old') ? [entry('older.PBA', 'file')] : query.has('cursor') ? [entry('late.db', 'file')] : [entry('old', 'directory'), entry('plain.db', 'file'), entry('protected.pba', 'file'), entry('unrelated.txt', 'file'), entry('link.db', 'file', {symlink: true}), entry('unreadable.db', 'file', {readable: false}), entry('unknown.db', 'unknown')];
    return {result: {path: current, parent: current ? current.slice(0, current.lastIndexOf('/')) || '/' : '', separator: '/', platform: 'linux', writable: false, writableChecked: false, suggestions: [], truncated: false,
      roots: [{path: '/mnt/media/Backups', name: 'Backups', kind: 'directory', description: ''}], entries, nextCursor: !frame && current === folder && !query.has('cursor') ? 'actual-cursor' : ''}} as T;
  }}), [identity]);
  if (params.has('confirm')) return <><IconSprite /><BackupConfirmationDialog open={open} onClose={() => {setOpen(false);proof.closes++;}} mode={params.get('confirm') === 'restore' ? 'restore' : 'delete'} scopeKey="controlled-confirmation" {...authority} format={defaultI18n}
    backup={{id: 'actual-target', createdAt: '2026-10-14T09:47:00-03:00', kind: 'pre-restore', schemaVersion: 40, serverVersion: '1.4.0', bytes: 16*1024*1024, path: '/not-displayed/backup.db'}}
    submit={(backup, signal) => {const seen = {id: backup.id, aborted: signal.aborted};proof.confirmations.push(seen);signal.addEventListener('abort', () => {seen.aborted = true;}, {once: true});return new Promise<void>(resolve => {complete = resolve;});}} onFailure={() => {proof.failures++;}} /></>;
  if (params.has('list')) return <><IconSprite /><BackupList scopeKey="controlled-only" {...authority} format={defaultI18n} busy={params.has('busy')} requestRestore={backup => proof.restoreOffers.push(backup.id)} requestDelete={backup => proof.deleteOffers.push(backup.id)} newest={{id: 'newest', label: 'Newest'}}
    backups={[{id: 'newest', createdAt: '2026-10-22T03:00:00-03:00', kind: 'scheduled', schemaVersion: 41, serverVersion: '1.4.2', bytes: 18*1024*1024, path: '/private-not-drawn/portico.db'}, {id: 'manual', createdAt: '2026-10-20T18:12:00-03:00', kind: 'manual', schemaVersion: 41, serverVersion: '1.4.2', bytes: 17*1024*1024, path: '/private-not-drawn/manual.db'}]} /></>;
  return <><IconSprite /><button type="button" onClick={() => setOpen(true)}>Reopen fixture</button><button type="button" onClick={() => setIdentity(value => value + 1)}>Replace fixture scope</button>
    <ServerFolderPicker open={open} api={api} purpose={purpose} initialPath={folder} onClose={() => {proof.closes++;setOpen(false);}} onChoose={paths => {(proof.choices as string[][]).push([...paths]);}} />
  </>;
}
createRoot(document.getElementById('root')!).render(<StrictMode><Fixture /></StrictMode>);
