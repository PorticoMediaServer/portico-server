import {useEffect, useRef, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import type {TitleFile} from '@core/detail.ts';
import {findLibraryFile, separateLibraryFile} from '@core/title-identity.ts';
import {problem} from '../../admin/console';
import {serverChanged} from '../../app/server-changes';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Button, Dialog, Notice, Text} from '../../ui';

/**
 * Split a version off its title (owners): for a file that was grouped with the wrong title. It
 * becomes a title of its own, nothing is deleted, and the two can be merged again.
 */
export function SplitVersionDialog({libraryId, title, file, summary, onClose}: {libraryId: string; title: string; file: TitleFile; /** The version as the Files list names it ("4K · HEVC · MKV"). */ summary: string; onClose: () => void}) {
  const i18n = useI18n();
  const {t} = i18n;
  const {api} = useSession();
  const navigate = useNavigate();
  const [phase, setPhase] = useState<'ask' | 'working'>('ask');
  const [error, setError] = useState('');
  const stop = useRef<AbortController | null>(null);
  useEffect(() => () => stop.current?.abort(), []);
  const split = async () => {
    const controller = stop.current = new AbortController();
    setPhase('working'); setError('');
    try {
      const record = await findLibraryFile(api, libraryId, file.id, {signal: controller.signal});
      if (controller.signal.aborted) return;
      if (!record) { setError(t('title.split.notFound')); setPhase('ask'); return; }
      const itemId = await separateLibraryFile(api, libraryId, record);
      serverChanged();
      onClose();
      void navigate({to: '/media/$itemId', params: {itemId}, search: {library: libraryId}});
    } catch (e) {
      if (controller.signal.aborted) return;
      setError(problem(e, 'action') || t('title.split.failed'));
      setPhase('ask');
    }
  };
  const cancel = () => { stop.current?.abort(); onClose(); };
  return (
    <Dialog open onOpenChange={o => !o && cancel()} title={t('title.split.title')} description={`${title} · ${summary}`} width={480}
      actions={<><Button variant="ghost" label={t('action.cancel')} onClick={cancel} /><Button variant="primary" label={t('title.split.confirm')} loading={phase === 'working'} onClick={() => void split()} /></>}>
      {error ? <Notice tone="error" compact>{error}</Notice> : null}
      <Text as="p" variant="body" tone="secondary">{t('title.split.body')}</Text>
      <span role="status">{phase === 'working' ? <Text variant="caption" tone="tertiary">{t('title.split.working')}</Text> : null}</span>
    </Dialog>
  );
}
