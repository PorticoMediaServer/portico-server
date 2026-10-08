import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {Link} from '@tanstack/react-router';
import {PLAY_HISTORY_COLUMNS, PLAY_HISTORY_PERIODS, fetchPlayHistory, playHistoryPeriodLabel, playHistoryPlatform, playHistoryPlayed, playHistoryTitle, playHistoryType, playHistoryUser, type PlayHistoryEntry, type PlayHistoryFilters, type PlayHistoryPeriod} from '@core/play-history.ts';
import {useConsole, useRead} from '../../admin/console';
import {useI18n} from '../../app/i18n';
import {useLibrariesContext} from '../../app/libraries';
import {useSession} from '../../app/session';
import {present} from '../../app/errors';
import {Button, Notice, Select, Surface, Table, Text} from '../../ui';
import s from './PlayHistory.module.css';

/**
 * Settings › Server › Play history: every play on the server, newest first, filtered by person,
 * library and time. The columns, filters and wording are client-core's (`play-history.ts`).
 * Pages are appended as they are asked for, so the page reads what is on screen however long the
 * history is.
 */
export function PlayHistoryPanel() {
  const i18n = useI18n();
  const {t} = i18n;
  const {api} = useSession();
  const client = useConsole();
  const libraries = useLibrariesContext();
  const accounts = useRead(() => client.accountOptions(), [client]);
  const [filters, setFilters] = useState<PlayHistoryFilters>({period: 'all'});
  const [state, setState] = useState<{entries: readonly PlayHistoryEntry[]; total?: number; next: string; loading: boolean; error?: string}>({entries: [], next: '', loading: true});
  const run = useRef(0);
  const load = useCallback((cursor: string) => {
    const n = ++run.current;
    setState(prev => ({...(cursor ? prev : {entries: [], next: ''}), loading: true}));
    fetchPlayHistory(api, filters, cursor).then(
      page => { if (n === run.current) setState(prev => ({entries: cursor ? [...prev.entries, ...page.entries] : page.entries, total: page.total ?? prev.total, next: page.nextCursor, loading: false})); },
      error => { if (n === run.current) setState(prev => ({...prev, loading: false, error: present(error, 'server-console').body})); },
    );
  }, [api, filters]);
  useEffect(() => load(''), [load]);
  const libraryName = useMemo(() => new Map(libraries.items.map(l => [l.id, l.name])), [libraries.items]);
  const filtered = !!filters.accountId || !!filters.libraryId || filters.period !== 'all';
  const columns = PLAY_HISTORY_COLUMNS.map(c => ({label: t(c.label)}));
  return (
    <Surface padless>
      <div className={s.filters}>
        <Select hideLabel label={t('server.plays.filterLibrary')} value={filters.libraryId ?? ''} onChange={e => setFilters(f => ({...f, libraryId: e.target.value || undefined}))}
          options={[{value: '', label: t('server.plays.allLibraries')}, ...libraries.items.map(l => ({value: l.id, label: l.name}))]} />
        <Select hideLabel label={t('server.plays.filterUser')} value={filters.accountId ?? ''} onChange={e => setFilters(f => ({...f, accountId: e.target.value || undefined}))}
          options={[{value: '', label: t('server.plays.allUsers')}, ...(accounts.data ?? []).map(a => ({value: a.id, label: a.label}))]} />
        <Select hideLabel label={t('server.plays.filterPeriod')} value={filters.period} onChange={e => setFilters(f => ({...f, period: e.target.value as PlayHistoryPeriod}))}
          options={PLAY_HISTORY_PERIODS.map(p => ({value: p, label: t(playHistoryPeriodLabel(p))}))} />
        {state.total !== undefined ? <Text variant="caption" tone="secondary" className={s.total}>{t('server.plays.total', {count: state.total})}</Text> : null}
        <Button size="sm" variant="ghost" icon="refresh" label={t('action.refresh')} loading={state.loading && !state.entries.length} onClick={() => load('')} />
      </div>
      {state.error ? <div className={s.note}><Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: () => load(state.entries.length ? state.next : '')}}>{state.error}</Notice></div> : null}
      {state.entries.length ? (
        <Table columns={columns} className={s.table}>
          {state.entries.map(entry => (
            <tr key={entry.id}>
              <td>{playHistoryUser(entry)}</td>
              <td>{playHistoryType(entry, i18n)}</td>
              <td className={s.title}>
                {entry.itemId ? <Link to="/media/$itemId" params={{itemId: entry.itemId}} search={{library: entry.libraryId}}>{playHistoryTitle(entry, i18n)}</Link> : playHistoryTitle(entry, i18n)}
                {filters.libraryId ? null : <span className={s.library}>{libraryName.get(entry.libraryId) ?? t('server.plays.removedLibrary')}</span>}
              </td>
              <td>{entry.device ?? ''}</td>
              <td>{playHistoryPlatform(entry, i18n)}</td>
              <td className={s.played}><Played entry={entry} /></td>
            </tr>
          ))}
        </Table>
      ) : !state.loading && !state.error ? (
        <div className={s.note}><Text variant="caption" tone="tertiary">{t(filtered ? 'server.plays.emptyFiltered' : 'server.plays.empty')}</Text></div>
      ) : null}
      {state.next ? <div className={s.more}><Button size="sm" variant="outline" label={t('server.plays.more')} loading={state.loading} onClick={() => load(state.next)} /></div> : null}
    </Surface>
  );
}

function Played({entry}: {entry: PlayHistoryEntry}) {
  const played = playHistoryPlayed(entry, useI18n());
  return <>{played.date}<span className={s.library}>{[played.time, played.note].filter(Boolean).join(' · ')}</span></>;
}
