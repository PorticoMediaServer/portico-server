import React from 'react';
import {useNavigate, useSearch} from '@tanstack/react-router';
import {settingsRowId, type SettingsPage as StructurePage, type SettingsRow as StructureRow, type SettingsSection} from '@core/presentation/index.ts';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {PageRefresh, PageRefreshControl, SettingsGroup, StateView, Tabs} from '../../ui';
import {FormLoadNotice, SaveBar, ServerFormsProvider, ServerSettingRowView, useFormsAbsent} from './ServerForms';
import {ServerPanel, panelIsGroup} from './ServerPanels';
import s from './Settings.module.css';

/**
 * One page of the Server heading, drawn from the authored structure: status and lists are
 * panels, plain settings are rows bound to the page's forms, and the page has one Save.
 * Owners only: the structure lists no Server page for anyone else.
 */
export function ServerPage({page}: {page: StructurePage}) {
  const session = useSession();
  const {t} = useI18n();
  const navigate = useNavigate();
  const search = useSearch({from: '/app/settings/$section'});
  if (!session.owner) return <StateView icon="lock" title={t('web.console.ownerTitle')} body={t('web.console.ownerBody')} action={{label: t('route.goHome'), onClick: () => void navigate({to: '/'})}} />;
  const tab = page.tabs ? (page.tabs.find(x => x.id === search.tab) ?? page.tabs[0]!).id : undefined;
  const sections = page.sections.filter(section => !page.tabs || section.tab === tab);
  return (
    <ServerFormsProvider key={page.id}>
      <PageRefresh>
        <div className={s.serverPage}>
          <header className={s.serverHead}>
            <h1 className={s.title}>{t(page.title)}</h1>
            <PageRefreshControl />
          </header>
          {/* An item opened inside a page (a library, an account) takes the page; its panel draws the way back. */}
          {page.tabs && !search.id ? <Tabs label={t(page.title)} items={page.tabs.map(x => ({id: x.id, label: t(x.title)}))} value={tab!} onChange={id => void navigate({to: '.', search: {tab: id}})} /> : null}
          {sections.map(section => <ServerSection key={section.id} section={section} />)}
          <SaveBar />
        </div>
      </PageRefresh>
    </ServerFormsProvider>
  );
}

function ServerSection({section}: {section: SettingsSection}) {
  const {t} = useI18n();
  const groups = section.rows.filter(row => row.kind === 'custom' && panelIsGroup(row.id));
  const rows = section.rows.filter(row => !(row.kind === 'custom' && panelIsGroup(row.id)));
  const forms = [...new Set(rows.flatMap(row => (row.kind === 'server-setting' ? [row.form] : [])))];
  // A group of settings whose documents this server does not have is left out, heading and all.
  const absent = useFormsAbsent(forms) && rows.every(row => row.kind === 'server-setting');
  const draw = (row: StructureRow) => row.kind === 'custom' ? <ServerPanel key={row.id} id={row.id} /> : row.kind === 'server-setting' ? <ServerSettingRowView key={settingsRowId(row)} row={row} /> : null;
  const body = (
    <>
      {groups.map(draw)}
      {rows.length && !absent ? (
        <SettingsGroup title={!section.advanced && section.title ? t(section.title) : undefined} description={section.description ? t(section.description) : undefined}>
          {forms.map(id => <FormLoadNotice key={id} id={id} />)}
          {rows.map(draw)}
        </SettingsGroup>
      ) : null}
    </>
  );
  return section.advanced ? <details className={s.advanced}><summary>{t(section.title ?? 'settings.section.advanced')}</summary>{body}</details> : body;
}
