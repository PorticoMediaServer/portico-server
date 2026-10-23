import {useEffect, useRef, useState} from 'react';
import {defaultI18n} from '@i18n';
import {SettingsPeopleClient, type SettingsPeoplePage, type SettingsPeopleQuery, type SettingsPersonRow, type SettingsBulkAccess, type SettingsBulkAction} from '@core/settings-people.ts';
import {settingsRowIndex, settingsSpec, type SettingsRow, type SettingsValue} from '@core/presentation/settings-spec.ts';
import {managesTier, type AccessMember, type AccessTier, type MemberLimits} from '@core/server-administration.ts';
import {SettingsGroup, SettingsHours, SettingsSelectionList, SettingsStringList, SettingsPanel, SettingsRows, SettingsTable, type SettingPresentation, type SettingsTableHandle, type TableQuery} from '../../kit/settings';
import {Button} from '../../kit/controls';
import {Dialog} from '../../kit/overlays/Dialog';
import {SettingsRow as Row} from '../../kit/shell/SettingsSurfaces';
import {StateView} from '../../kit/states';

const t = defaultI18n.t, section = settingsSpec.find(section => section.scope === 'server' && section.id === 'people')!;
const table = section.groups![0].rows[0].control, personSpec = section.panels!.person;
const label = (id: string) => t(settingsRowIndex.get(id)!.row.label);
const initialQuery: TableQuery = {search: '', filters: {role: 'all', status: 'all'}, sort: {by: 'active', direction: 'desc'}};
function queryOf(query: TableQuery): SettingsPeopleQuery {
  return {search: query.search, role: query.filters.role as SettingsPeopleQuery['role'], status: query.filters.status as SettingsPeopleQuery['status'], sort: query.sort.by as SettingsPeopleQuery['sort'], direction: query.sort.direction};
}
export type PeopleBindings = Readonly<{client: SettingsPeopleClient; serverName: string; libraries: readonly {id: string; name: string}[]; actor: {accountId: string; role: AccessTier}; invite: (opener: HTMLElement) => void; reportError: (error: unknown) => void}>;

/** The table query, detail revision and bulk transaction stay bound to one real session. */
export function PeopleScreen({bindings}: {bindings: PeopleBindings}) {
  const {client} = bindings;
  const [query, setQuery] = useState<TableQuery>(initialQuery), [page, setPage] = useState<SettingsPeoplePage>(), [error, setError] = useState<unknown>();
  const [loading, setLoading] = useState(false), [selected, setSelected] = useState<ReadonlySet<string>>(new Set()), [refresh, setRefresh] = useState(0);
  const [person, setPerson] = useState<AccessMember>(), [personId, setPersonId] = useState<string>(), [tab, setTab] = useState(personSpec.tabs![0].id);
  const [advanced, setAdvanced] = useState(false), [saving, setSaving] = useState(false), [saved, setSaved] = useState(false);
  const [chooseLibraries, setChooseLibraries] = useState(false);
  const [chooseHours, setChooseHours] = useState(false);
  const [command, setCommand] = useState<{action: SettingsBulkAction; rows: readonly SettingsPersonRow[]} | undefined>(), [bulk, setBulk] = useState<SettingsBulkAccess>();
  const tableRef = useRef<SettingsTableHandle>(null), opener = useRef<HTMLElement | null>(null), generation = useRef(0), mounted = useRef(true);
  const confirmOpener = useRef<HTMLElement | null>(null);
  useEffect(() => () => {mounted.current = false; ++generation.current;}, []);
  useEffect(() => {
    const current = ++generation.current, controller = new AbortController();
    setPage(undefined); setLoading(true); setError(undefined); setSelected(new Set());
    const timer = setTimeout(() => {void client.read(queryOf(query), '', controller.signal).then(value => {if (current === generation.current) setPage(value);}).catch(error => {if (!controller.signal.aborted && current === generation.current) setError(error);}).finally(() => {if (current === generation.current) setLoading(false);});}, query.search ? 200 : 0);
    return () => {clearTimeout(timer); controller.abort();};
  }, [client, query, refresh]);
  useEffect(() => {
    if (!personId) {setPerson(undefined); return;}
    const controller = new AbortController(); setPerson(undefined); setError(undefined); setChooseLibraries(false); setChooseHours(false);
    void client.person(personId, controller.signal).then(value => {if (!controller.signal.aborted) setPerson(value);}).catch(error => {if (!controller.signal.aborted) setError(error);});
    return () => controller.abort();
  }, [client, personId]);
  const more = () => {
    if (loading || !page?.nextCursor) return;
    const current = generation.current, before = page; setLoading(true);
    void client.read(queryOf(query), page.nextCursor).then(next => {if (current === generation.current) {
      const ids = new Set(before.items.map(row => row.accountId));
      setPage({...next, items: [...before.items, ...next.items.filter(row => !ids.has(row.accountId))]});
    }}).catch(error => {if (current === generation.current) setError(error);}).finally(() => {if (current === generation.current) setLoading(false);});
  };
  const run = async (action: () => Promise<void>) => {
    if (saving) return; setSaving(true); setSaved(false); setError(undefined);
    try {await action(); if (mounted.current) {setSaved(true); setRefresh(value => value + 1);}}
    catch (error) {if (mounted.current) {setError(error); bindings.reportError(error);}}
    finally {if (mounted.current) setSaving(false);}
  };
  const canEdit = !!person && person.accountId !== bindings.actor.accountId && managesTier(bindings.actor.role, person.role);
  const values: Readonly<Record<string, SettingsValue | undefined>> = person ? {
    'people.username': person.username, 'people.notes': person.notes ?? '', 'people.role': person.role === 'admin' ? 'administrator' : person.role,
    'people.libraries': person.allowedLibraries.length || chooseLibraries ? 'some' : 'all', 'people.maxStreams': person.limits.maxStreams === 0 ? 'unlimited' : String(person.limits.maxStreams),
    'people.awayAccess': person.limits.allowRemote ? 'allowed' : 'home', 'people.conversions': person.limits.conversion === 'audio' ? 'sound' : person.limits.conversion,
    'people.hours': person.limits.schedule.windows.length || chooseHours ? 'set' : 'any', 'people.hoursTimezone': person.limits.schedule.timezone,
    'people.maxRating': person.limits.maxContentRating || 'none', 'people.allowUnrated': person.limits.allowUnrated, 'people.accessTokens': person.limits.accessTokens,
  } : {};
  const state = (row: SettingsRow): SettingPresentation => ({present: !!person && (Object.hasOwn(values, row.id) || ['people.turnOff', 'people.delete', 'people.profiles', 'people.hoursList', 'people.hiddenLabels'].includes(row.id) || row.id === 'people.libraryList' && (chooseLibraries || !!person.allowedLibraries.length)),
    value: values[row.id], displayValue: typeof values[row.id] === 'string' ? values[row.id] as string : undefined, disabled: saving || !canEdit});
  const change = (row: SettingsRow, value: SettingsValue) => {
    if (!person || !canEdit) return;
    if (row.id === 'people.libraries' && value === 'some') {setChooseLibraries(true); return;}
    if (row.id === 'people.hours' && value === 'set') {setChooseHours(true); return;}
    const before = person;
    void run(async () => {
      if (row.id === 'people.notes') {const next = await client.notes(before, String(value)); setPerson(current => current?.accountId === before.accountId ? next : current); return;}
      if (row.id === 'people.role') {const next = await client.role(before, value === 'administrator' ? 'admin' : value as AccessTier); setPerson(current => current?.accountId === before.accountId ? next : current); return;}
      if (row.id === 'people.libraries' && value === 'all') {await libraryAccess([]); setChooseLibraries(false); return;}
      const patch: {-readonly [K in keyof MemberLimits]?: MemberLimits[K]} = {};
      if (row.id === 'people.maxStreams') patch.maxStreams = value === 'unlimited' ? 0 : value === 'default' ? 0 : Number(value);
      else if (row.id === 'people.awayAccess') patch.allowRemote = value === 'allowed';
      else if (row.id === 'people.conversions') patch.conversion = value === 'sound' ? 'audio' : value as MemberLimits['conversion'];
      else if (row.id === 'people.maxRating') patch.maxContentRating = value === 'none' ? '' : String(value);
      else if (row.id === 'people.allowUnrated') patch.allowUnrated = value === true;
      else if (row.id === 'people.accessTokens') patch.accessTokens = value === true;
      else if (row.id === 'people.hoursTimezone') patch.schedule = {...before.limits.schedule, timezone: String(value)};
      else if (row.id === 'people.hours' && value === 'any') patch.schedule = {...before.limits.schedule, windows: []};
      else return;
      const saved = await client.limits(before, {...before.limits, ...patch});
      setPerson(current => current?.accountId === before.accountId ? {...before, limits: saved.limits, limitsRevision: saved.revision} : current);
    });
  };
  const libraryAccess = async (allowedLibraries: readonly string[]) => {
    if (!person || !canEdit) return;
    const row = page?.items.find(row => row.accountId === person.accountId); if (!row) return;
    await client.bulk([{...row, revision: person.revision, limitsRevision: person.limitsRevision}], 'access', {allowedLibraries: [...allowedLibraries]});
    const next = await client.person(person.accountId); setPerson(current => current?.accountId === next.accountId ? next : current);
  };
  const hours = (windows: MemberLimits['schedule']['windows']) => {
    if (!person || !canEdit) return;
    const before = person;
    void run(async () => {const saved = await client.limits(before, {...before.limits, schedule: {...before.limits.schedule, windows}}); setPerson(current => current?.accountId === before.accountId ? {...before, limits: saved.limits, limitsRevision: saved.revision} : current);});
  };
  const ask = (action: SettingsBulkAction, ids: readonly string[], element?: HTMLElement) => {
    const rows = (page?.items ?? []).filter(row => ids.includes(row.accountId)).map(row => person?.accountId === row.accountId ? {...row, revision: person.revision, limitsRevision: person.limitsRevision} : row);
    if (!rows.length || rows.some(row => row.accountId === bindings.actor.accountId || !managesTier(bindings.actor.role, row.role))) return;
    confirmOpener.current = element ?? (document.activeElement instanceof HTMLElement ? document.activeElement : null); setCommand({action, rows}); if (action === 'access') setBulk({});
  };
  const personRow = page?.items.find(row => row.accountId === personId);
  const filtered = !!query.search.trim() || Object.values(query.filters).some(value => value !== 'all');
  const special = (row: SettingsRow) => {
    if (!person) return null;
    if (row.id === 'people.libraryList') return <SettingsSelectionList label={label(row.id)} minimum={1} items={bindings.libraries.map(library => ({id: library.id, label: library.name}))} selected={person.allowedLibraries} disabled={!canEdit || saving} onChange={ids => {void run(() => libraryAccess(ids));}} />;
    if (row.id === 'people.hoursList') return <SettingsHours windows={person.limits.schedule.windows} disabled={!canEdit || saving} onChange={hours} />;
    if (row.id === 'people.hiddenLabels') return <SettingsStringList label={label(row.id)} values={person.limits.tagPolicy.deniedLabels} addLabel={t(row.control.addLabel!)} disabled={!canEdit || saving} onChange={deniedLabels => {
      const before = person;
      void run(async () => {const saved = await client.limits(before, {...before.limits, tagPolicy: {deniedLabels}}); setPerson(current => current?.accountId === before.accountId ? {...before, limits: saved.limits, limitsRevision: saved.revision} : current);});
    }} />;
    if (row.id === 'people.profiles') return <>{person.profileNames?.map((name, index) => <Row key={index} label={name}><span /></Row>)}</>;
    if (row.control.type === 'readonly') return <span>{values[row.id] as string}</span>;
    if (row.control.type === 'action') return <Button size="compact" disabled={!canEdit || saving || !personRow} onClick={() => {if (personRow) ask(row.id === 'people.delete' ? 'remove' : person.disabled ? 'enable' : 'disable', [personId!]);}}>{row.id === 'people.turnOff' && person.disabled ? t('next.settings.turnOn') : t(row.control.label!)}</Button>;
    return null;
  };
  return <>
    <SettingsGroup title={t(section.groups![0].title!)} plain actions={<Button size="compact" variant="primary" icon="i-plus" iconSize={20} onClick={event => bindings.invite(event.currentTarget)}>{t(table.addLabel!)}</Button>}>
    {error && !page && !personId ? <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')} primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => setRefresh(value => value + 1)}} /> : <SettingsTable ref={tableRef} height={table.height}
      label={t(section.title)} rows={(page?.items ?? []).map(row => ({id: row.accountId, label: row.username, selectable: row.accountId !== bindings.actor.accountId && managesTier(bindings.actor.role, row.role), cells: {
        person: {title: row.username, meta: row.email, avatar: Array.from(row.username)[0]},
        role: {title: t(row.role === 'owner' ? 'next.shell.owner' : row.role === 'admin' ? 'next.shell.administrator' : 'next.settings.member'), tone: row.role === 'owner' ? 'ok' : row.role === 'admin' ? 'accent' : 'quiet'},
        access: {title: row.libraryNames.length ? row.libraryNames.join(', ') : t('next.settings.allLibraries'), meta: row.maxStreams ? t('next.settings.streams', {count: row.maxStreams}) : t('next.settings.unlimitedStreams')},
        profiles: row.profileNames.join(', '), active: {title: row.liveState === 'watching' ? t('next.settings.watchingNow') : row.liveState === 'listening' ? t('next.settings.listeningNow') : row.status !== 'active' ? t(`next.settings.personStatus.${row.status}` as 'next.settings.personStatus.off') : row.lastActiveAt ? new Intl.DateTimeFormat(undefined, {dateStyle: 'medium'}).format(new Date(row.lastActiveAt)) : t('next.settings.never')},
      }}))} columns={table.columns!.map(column => ({id: column.id!, label: t(column.label), kind: column.kind as 'main', sortable: column.sort, width: column.width}))}
      query={query} onQueryChange={setQuery} filters={table.filters!.map(filter => ({id: filter.id, label: t(filter.label), options: filter.options.map(option => ({value: option.value, label: t(option.label)}))}))}
      searchLabel={t(table.search!)} countLabel={t(filtered ? 'next.settings.peopleCount' : 'next.settings.peopleTotal', {matched: page?.matchedCount ?? 0, total: page?.totalCount ?? 0})} emptyLabel={t(table.empty!)} loading={loading}
      matchedCount={page?.matchedCount ?? 0} hasMore={!!page?.nextCursor} onLoadMore={more} selected={selected} onSelectionChange={setSelected}
      rowAction={{label: t(table.rowAction!.label), onAction: (row, element) => {opener.current = element; setPersonId(row.id); setTab(personSpec.tabs![0].id);}}}
      bulkActions={[
        {id: 'access', label: t(table.select!.bulk[0].label), onAction: (ids, element) => ask('access', ids, element)},
        {id: 'disable', label: t(table.select!.bulk[1].label), onAction: (ids, element) => ask('disable', ids, element)},
        {id: 'remove', label: t(table.select!.bulk[2].label), destructive: true, onAction: (ids, element) => ask('remove', ids, element)},
      ]} />}
    </SettingsGroup>
    {personId && <SettingsPanel open onClose={() => {setPersonId(undefined); setPerson(undefined);}} title={person?.username ?? personRow?.username ?? t(section.title)} audience={t(bindings.actor.role === 'owner' ? 'next.shell.serverVisibility.ownerShared' : 'next.shell.serverVisibility.administrator')}
      returnFocusRef={opener} selectedTab={tab} onTabChange={setTab} saveState={saving ? 'saving' : saved ? 'saved' : undefined}
      error={error ? t('next.states.pageFailedBody') : undefined} tabs={personSpec.tabs!.map(tab => ({id: tab.id, title: t(tab.title), content: !person ? <p role="status">{t('next.settings.loading')}</p> : <SettingsRows holder={tab} state={state} change={change} reset={() => {}} advancedOpen={advanced} onAdvancedChange={setAdvanced}
        renderSpecial={special} />
      }))} />}
    {command && <Dialog open title={command.action === 'access' ? t(table.select!.bulk[0].label) : t('next.settings.confirmPeople', {count: command.rows.length})} onOpenChange={open => {if (!open && !saving) {setCommand(undefined); setBulk(undefined);}}}
      returnFocusRef={confirmOpener} footer={<><Button disabled={saving} onClick={() => {setCommand(undefined); setBulk(undefined);}}>{t('next.settings.cancel')}</Button><Button variant="primary" loading={saving} disabled={command.action === 'access' && !Object.keys(bulk ?? {}).length} onClick={() => {void run(async () => {await client.bulk(command.rows, command.action, command.action === 'access' ? bulk : undefined); setCommand(undefined); setBulk(undefined); setSelected(new Set()); if (command.action === 'remove' && command.rows.some(row => row.accountId === personId)) {setPersonId(undefined); setPerson(undefined);} else if (personId) {const next = await client.person(personId); setPerson(current => current?.accountId === next.accountId ? next : current);}});}}>{t('next.settings.apply')}</Button></>}>
      {command.action === 'access' ? <SettingsRows holder={section.panels!.bulkAccess} advancedOpen={false} onAdvancedChange={() => {}} reset={() => {}} state={row => ({present: ['bulk.maxStreams', 'bulk.awayAccess', 'bulk.maxRating'].includes(row.id), value: row.id === 'bulk.maxStreams' ? bulk?.maxStreams === undefined ? 'keep' : bulk.maxStreams === 0 ? 'unlimited' : String(bulk.maxStreams) : row.id === 'bulk.awayAccess' ? bulk?.allowRemote === undefined ? 'keep' : bulk.allowRemote ? 'allowed' : 'home' : bulk?.maxContentRating === undefined ? 'keep' : bulk.maxContentRating || 'none'})}
        change={(row, value) => setBulk(before => {const next = {...before}; if (row.id === 'bulk.maxStreams') {if (value === 'keep') delete next.maxStreams; else next.maxStreams = value === 'unlimited' || value === 'default' ? 0 : Number(value);} else if (row.id === 'bulk.awayAccess') {if (value === 'keep') delete next.allowRemote; else next.allowRemote = value === 'allowed';} else {if (value === 'keep') delete next.maxContentRating; else next.maxContentRating = value === 'none' ? '' : String(value);} return next;})} renderSpecial={() => null} /> : <p>{t(command.action === 'remove' ? table.select!.bulk[2].confirm! : table.select!.bulk[1].confirm!)}</p>}
    </Dialog>}
  </>;
}
