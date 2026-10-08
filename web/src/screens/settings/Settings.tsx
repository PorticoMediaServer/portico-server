import React, {Suspense, useEffect, useMemo, useState} from 'react';
import {Link, useLocation, useNavigate, useParams} from '@tanstack/react-router';
import {findSettingsPage, searchSettings, settingsPageId, settingsRowId, settingsStructure, type SettingsPage as StructurePage, type SettingsRow as StructureRow, type SettingsSection} from '@core/presentation/index.ts';
import type {PreferencePatch} from '@core/preferences.ts';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {useServerPreferences} from '../../app/server-preferences';
import {useDownloads} from '../../app/downloads';
import {errorText} from '../../app/errors';
import {Icon, Input, Loading, Notice, Page, SettingsGroup, useNarrow} from '../../ui';
import {RowView} from './SettingsRows';
import {AccountPanel, isGroupPanel} from './AccountPanels';
import s from './Settings.module.css';

// The Server heading's pages carry the console's forms and charts: loaded when an owner opens one.
const ServerPage = React.lazy(() => import('./ServerPages').then(m => ({default: m.ServerPage})));

/** Addresses from before the one Settings screen (Justin, 2 Oct 2026), so old links still land. */
const FORMER: Readonly<Record<string, string>> = {account: 'profile', preferences: 'playback', servers: 'device', notifications: 'profile'};

export function SettingsScreen() {
  return <SettingsFrame />;
}
export function SettingsSectionScreen() {
  const {section} = useParams({from: '/app/settings/$section'});
  return <SettingsFrame address={FORMER[section] ?? section} />;
}

/**
 * The one Settings screen: pages listed under Account and Server on the left, the page on the
 * right, drawn from `settingsStructure` (client-core). The rail is untouched. On a narrow screen
 * the page list is the first screen and each page pushes.
 */
function SettingsFrame({address}: {address?: string}) {
  const session = useSession();
  const {t} = useI18n();
  const narrow = useNarrow();
  const server = useServerPreferences();
  const downloads = useDownloads();
  const local = session.session?.viewer.authority === 'local';
  const registry = server.snapshot?.registry;
  const structure = useMemo(() => settingsStructure({
    owner: session.owner,
    platform: 'web',
    capabilities: {
      ...(registry ? {preferenceKeys: new Set(registry.fields.map(f => f.key))} : {}),
      localAccount: local,
      porticoMember: local && !!session.local?.hostedAccountId,
      hostedAccount: !!session.hosted,
      downloads: !downloads.state.unavailable,
      // The devices list (and renaming this browser in it) is the server account's.
      deviceName: local,
    },
  }), [session.owner, registry, local, session.local?.hostedAccountId, session.hosted, downloads.state.unavailable]);
  const [query, setQuery] = useState('');
  const matches = useMemo(() => searchSettings(structure, query, t), [structure, query, t]);
  const current = address || !narrow ? findSettingsPage(structure, address) : undefined;
  const list = (
    <nav className={s.nav} aria-label={t('settings.pages')}>
      <Input hideLabel type="search" label={t('settings.search')} placeholder={t('settings.search')} value={query} onChange={e => setQuery(e.target.value)} />
      {query.trim().length >= 2 ? (
        <div className={s.matches}>
          {matches.map(m => (
            <Link key={m.address + m.rowId} to="/settings/$section" params={{section: m.address}} search={{}} hash={m.rowId ? rowAnchor(m.rowId) : undefined} className={s.match} onClick={() => setQuery('')}>
              <span className={s.matchLabel}>{m.label}</span>
              {m.label !== m.pageTitle ? <span className={s.matchPage}>{m.pageTitle}</span> : null}
            </Link>
          ))}
          {!matches.length ? <p className={s.noMatch}>{t('settings.search.none', {query: query.trim()})}</p> : null}
        </div>
      ) : structure.headings.map(heading => (
        <React.Fragment key={heading.id}>
          <div className={s.navHeading}>{t(heading.title)}</div>
          {heading.pages.map(page => {
            const to = settingsPageId(heading.id, page.id);
            return (
              <Link key={to} to="/settings/$section" params={{section: to}} search={{}} className={s.navItem} aria-current={current && current.page === page ? 'page' : undefined}>
                <Icon name={page.icon} size={17} /><span className={s.navLabel}>{t(page.title)}</span>{narrow ? <Icon name="forward" size={15} /> : null}
              </Link>
            );
          })}
        </React.Fragment>
      ))}
    </nav>
  );
  if (!current) return <Page><div className={s.layout}><h1 className={s.listTitle}>{t('settings.title')}</h1>{list}</div></Page>;
  return (
    <Page>
      <div className={s.layout}>
        {narrow ? null : list}
        <div className={s.pane}>
          {narrow ? <Link to="/settings" className={s.back}><Icon name="back" size={14} />{t('settings.title')}</Link> : null}
          {current.heading.id === 'server'
            ? <Suspense fallback={<Loading label={t(current.page.title)} />}><ServerPage page={current.page} /></Suspense>
            : <AccountPage key={current.page.id} page={current.page} />}
        </div>
      </div>
    </Page>
  );
}

const rowAnchor = (rowId: string) => 'setting-' + rowId.replace(/[^A-Za-z0-9_-]/g, '-');

/** A personal page: rows save on change and say so briefly (handoff, pass 5). */
function AccountPage({page}: {page: StructurePage}) {
  const {t} = useI18n();
  const server = useServerPreferences();
  const navigate = useNavigate();
  const hash = useLocation({select: l => l.hash});
  const [saved, setSaved] = useState(0);
  const [failure, setFailure] = useState<string>();
  const save = (patch: PreferencePatch) => {
    setFailure(undefined);
    void server.set(patch).then(() => setSaved(Date.now()), (e: unknown) => setFailure(errorText(e, 'preferences', 'save')));
  };
  useEffect(() => {
    if (!saved) return;
    const timer = setTimeout(() => setSaved(0), 2000);
    return () => clearTimeout(timer);
  }, [saved]);
  // A search match opens its page and brings the row into view, marked for a moment.
  useEffect(() => {
    if (!hash) return;
    const row = document.getElementById(hash);
    if (!row) return;
    row.closest('details')?.setAttribute('open', '');
    row.scrollIntoView({block: 'center'});
    row.dataset.found = 'true';
    const timer = setTimeout(() => { delete row.dataset.found; void navigate({to: '.', hash: '', replace: true}); }, 1800);
    return () => clearTimeout(timer);
  }, [hash, navigate, server.snapshot]);
  const needsServer = page.sections.some(section => section.rows.some(row => row.kind === 'preference' || (row.kind === 'composite' && row.storage === 'registry')));
  return (
    <div className={s.page}>
      <header className={s.head}>
        <h1 className={s.title}>{t(page.title)}</h1>
        <span className={s.saved} role="status">{saved && !failure ? <><Icon name="check" size={14} />{t('settings.saved')}</> : null}</span>
      </header>
      {failure ? <Notice tone="error" compact title={t('settings.notSaved')}>{failure}</Notice> : null}
      {needsServer && server.error && !server.snapshot ? <Notice tone="warning" title={t('web.prefs.unavailableTitle')} action={{label: t('action.tryAgain'), onClick: server.reload}}>{t('web.prefs.unavailableBody')}</Notice> : null}
      {needsServer && server.loading && !server.snapshot ? <Loading label={t('web.prefs.loading')} /> : null}
      {!needsServer || server.snapshot ? page.sections.map(section => <SectionView key={section.id} section={section} save={save} />) : null}
    </div>
  );
}

function SectionView({section, save}: {section: SettingsSection; save: (patch: PreferencePatch) => void}) {
  const {t} = useI18n();
  const server = useServerPreferences();
  // A row that depends on another setting is absent while that setting is off (the countdown, without auto-play).
  const shown = section.rows.filter((row): row is Exclude<StructureRow, {kind: 'server-setting'}> => row.kind !== 'server-setting').filter(row => row.kind === 'custom' || !row.when || server.value(row.when.key, row.when.equals) === row.when.equals);
  const groups = shown.filter(row => row.kind === 'custom' && isGroupPanel(row.id));
  const rows = shown.filter(row => !(row.kind === 'custom' && isGroupPanel(row.id)));
  const draw = (row: Exclude<StructureRow, {kind: 'server-setting'}>) => row.kind === 'custom' ? <AccountPanel key={row.id} id={row.id} /> : <RowView key={settingsRowId(row)} row={row} anchor={rowAnchor(settingsRowId(row))} save={save} />;
  const body = (
    <>
      {groups.map(draw)}
      {rows.length ? <SettingsGroup title={!section.advanced && section.title ? t(section.title) : undefined}>{rows.map(draw)}</SettingsGroup> : null}
    </>
  );
  if (!shown.length) return null;
  return section.advanced ? <details className={s.advanced}><summary>{t(section.title ?? 'settings.section.advanced')}</summary>{body}</details> : body;
}
