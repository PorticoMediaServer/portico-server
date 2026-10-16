import {useState} from 'react';
import type {Library} from '@core/index.ts';
import type {PinnedResource} from '@core/library-pins.ts';
import {iconFor} from '@core/presentation/index.ts';
import {useI18n} from '../app/i18n';
import {errorText} from '../app/errors';
import {Dialog, IconButton, ListRow, Notice, Text} from '../ui';
import type {usePinActions} from '../app/pins';

export const pinnedResourceIcon = (kind: PinnedResource['kind']) => (kind === 'playlist' ? 'queue' : kind === 'view' ? 'filter' : 'collection');

/**
 * What is pinned, and in what order: every library with a pin to turn on or off (pinned ones lead
 * the rail, in the order set here), then the pinned collections, saved views and playlists, each
 * moved up or down or unpinned. Every change is saved as it is made.
 */
export function PinEditor({open, onOpenChange, libraries, pinned, actions}: {open: boolean; onOpenChange: (open: boolean) => void; libraries: readonly Library[]; pinned: readonly PinnedResource[]; actions: ReturnType<typeof usePinActions>}) {
  const {t} = useI18n();
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState('');
  const run = (work: () => Promise<void>) => {
    if (busy) return;
    setBusy(true); setProblem('');
    void work().catch(e => setProblem(errorText(e, 'library', 'save'))).finally(() => setBusy(false));
  };
  const pinnedLibraries = libraries.filter(l => l.pinned === true);
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title={t('pins.editTitle')} description={t('pins.librariesHint')}>
      {problem ? <Notice tone="error">{problem}</Notice> : null}
      <Text as="h3" variant="caption" tone="secondary">{t('pins.libraries')}</Text>
      <div role="list">
        {libraries.map(library => {
          const at = pinnedLibraries.findIndex(l => l.id === library.id);
          const isPinned = at >= 0;
          return (
            <ListRow key={library.id} as="div" icon={iconFor(library.kind)} title={library.name} actions={<>
              {isPinned ? <IconButton name="chevronUp" variant="ghost" size="sm" label={t('pins.moveUp', {name: library.name})} disabled={busy || at === 0} onClick={() => run(() => actions.moveLibrary(library.id, -1))} /> : null}
              {isPinned ? <IconButton name="chevronDown" variant="ghost" size="sm" label={t('pins.moveDown', {name: library.name})} disabled={busy || at === pinnedLibraries.length - 1} onClick={() => run(() => actions.moveLibrary(library.id, 1))} /> : null}
              <IconButton name={isPinned ? 'bookmarkFilled' : 'bookmark'} variant="ghost" size="sm" aria-pressed={isPinned} label={isPinned ? t('pins.unpinNamed', {name: library.name}) : t('pins.pinNamed', {name: library.name})} disabled={busy} onClick={() => run(() => actions.pinLibrary(library.id, !isPinned))} />
            </>} />
          );
        })}
      </div>
      <Text as="h3" variant="caption" tone="secondary">{t('pins.saved')}</Text>
      {pinned.length ? (
        <div role="list">
          {pinned.map((entry, index) => (
            <ListRow key={entry.kind + ':' + entry.id} as="div" icon={pinnedResourceIcon(entry.kind)} title={entry.name} actions={<>
              <IconButton name="chevronUp" variant="ghost" size="sm" label={t('pins.moveUp', {name: entry.name})} disabled={busy || index === 0} onClick={() => run(() => actions.moveResource(entry, -1))} />
              <IconButton name="chevronDown" variant="ghost" size="sm" label={t('pins.moveDown', {name: entry.name})} disabled={busy || index === pinned.length - 1} onClick={() => run(() => actions.moveResource(entry, 1))} />
              <IconButton name="bookmarkFilled" variant="ghost" size="sm" label={t('pins.unpinNamed', {name: entry.name})} disabled={busy} onClick={() => run(() => actions.unpinResource(entry))} />
            </>} />
          ))}
        </div>
      ) : <Text as="p" variant="body" tone="secondary">{t('pins.noneSaved')}</Text>}
    </Dialog>
  );
}
