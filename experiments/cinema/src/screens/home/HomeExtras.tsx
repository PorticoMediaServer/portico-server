import React, {useEffect, useState} from 'react';
import {View} from 'react-native';
import {useRoute, type RouteProp} from '@react-navigation/native';
import {useQuery, useServices} from '../../data/hooks';
import type {HomeLayoutEntry, NotificationItem} from '../../data/types';
import {Button, IconButton} from '../../design/Button';
import {Card} from '../../design/Card';
import {Badge, ListRow, Panel, Segmented, ToggleRow} from '../../design/Controls';
import {ui} from '../../design/overlays';
import {Banner, EmptyState, ErrorState, GridSkeleton, RowsSkeleton, Skeleton} from '../../design/States';
import {Text} from '../../design/Text';
import {colors, isTV, u} from '../../design/tokens';
import {useLayout} from '../../design/useLayout';
import {Screen} from '../../nav/Screen';
import {screenIndex} from '../../nav/screenIndex';
import {back, type Routes} from '../../nav/routes';
import {isManager, useSession} from '../../state/session';
import {openItemMenu} from '../shared/actions';
import {openItem, shapeFor, toCard} from '../shared/cards';
import {useItems} from '../shared/useItems';
import {plural} from '../../design/format';

/** A row's See all: the row as a paged grid. */
export function HomeRowScreen() {
  const route = useRoute<RouteProp<Routes, 'HomeRow'>>();
  const s = useServices();
  const session = useSession();
  const layout = useLayout();
  const rowId = route.params.rowId;
  const home = useQuery('row:' + rowId, async () => (await s.catalog.row(rowId)) ?? (await s.catalog.discover(rowId.split('-').slice(0, 2).join('-'))).find(r => r.id === rowId));
  const row = home.data;
  const items = useItems(row?.itemIds);
  const first = row?.itemIds.map(id => items.lookup(id)).find(Boolean);
  const shape = first ? shapeFor(first) : 'poster';
  const cols = isTV ? (shape === 'poster' ? 7 : 4) : layout.compact ? (shape === 'poster' ? 3 : 2) : 5;
  const gap = isTV ? 36 : 12;
  const w = Math.floor((layout.width - layout.railWidth - layout.gutter * 2 - (cols - 1) * gap) / cols);
  return (
    <Screen title={row?.title}>
      <Text variant="eyebrow" tone="accent">{row ? `${plural(row.itemIds.length, 'title', 'titles')}` : ' '}</Text>
      <Text variant="hero" style={{marginBottom: u(16)}}>{row ? `${row.title}${row.subject ? ' ' + row.subject : ''}` : ' '}</Text>
      {!row ? <GridSkeleton columns={cols} width={w} /> : null}
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap}}>
        {(row?.itemIds ?? []).map((id, i) => {
          const it = items.lookup(id);
          return it ? <Card key={id} item={toCard(it, s.user.state(id), {lookup: items.lookup})} width={w} hasTVPreferredFocus={i === 0} showTitle="always" onPress={() => openItem(it)} onLongPress={() => openItemMenu(s, it, {origin: row?.recommendation ? 'recommendation' : row?.kind === 'continue' ? 'continue' : 'default', manager: isManager(session.role)})} /> : null;
        })}
      </View>
    </Screen>
  );
}

/** Customize Home: show, hide and reorder rows; add a saved view row; reset. */
export function HomeCustomiseScreen() {
  const s = useServices();
  const q = useQuery('home-layout', () => s.catalog.homeLayout());
  const rows = useQuery('home-rows-all', async () => {
    const layout = await s.catalog.homeLayout();
    return Promise.all(layout.map(l => s.catalog.row(l.rowId)));
  });
  const views = useQuery('views', () => s.lists.savedViews());
  const [layout, setLayout] = useState<HomeLayoutEntry[] | null>(null);
  useEffect(() => {
    if (q.data && !layout) setLayout(q.data);
  }, [q.data, layout]);
  const title = (id: string) => {
    const r = rows.data?.find(x => x?.id === id);
    return r ? `${r.title}${r.subject ? ' ' + r.subject : ''}` : id;
  };
  const move = (i: number, d: number) => setLayout(l => {
    if (!l) return l;
    const n = [...l];
    const j = i + d;
    if (j < 0 || j >= n.length) return l;
    [n[i], n[j]] = [n[j], n[i]];
    return n;
  });
  return (
    <Screen heading="Customize Home" eyebrow="Home">
      <Text variant="body" tone="secondary" style={{marginBottom: u(14)}}>Choose which rows appear on Home and in what order.</Text>
      <Panel>
        {(layout ?? []).map((l, i) => (
          <View key={l.rowId} style={{flexDirection: 'row', alignItems: 'center'}}>
            <View style={{flex: 1}}>
              {l.alwaysOn ? <ListRow label={title(l.rowId)} help="Always on Home" compact /> : <ToggleRow label={title(l.rowId)} help={l.visible ? 'On Home' : 'Hidden'} value={l.visible} onChange={v => setLayout(ls => ls!.map(x => (x.rowId === l.rowId ? {...x, visible: v} : x)))} />}
            </View>
            <IconButton icon="arrowUp" label={`Move ${title(l.rowId)} up`} size={32} variant="ghost" onPress={() => move(i, -1)} />
            <IconButton icon="arrowDown" label={`Move ${title(l.rowId)} down`} size={32} variant="ghost" onPress={() => move(i, 1)} />
          </View>
        ))}
        {!layout ? <RowsSkeleton rows={1} /> : null}
      </Panel>
      <View style={{flexDirection: 'row', gap: 10, flexWrap: 'wrap'}}>
        <Button label="Add a saved view" icon="plus" onPress={() => ui.menu({title: 'Add a saved view', subtitle: 'Choose a saved view to show on Home.', actions: (views.data ?? []).map(v => ({label: v.title, checked: v.onHome, onPress: () => { v.onHome = true; ui.toast(`${v.title} added to Home.`); }}))})} />
        <Button label="Reset to default" icon="refresh" onPress={() => q.data && setLayout(q.data.map(x => ({...x, visible: true})))} />
        <Button label="Save" variant="primary" hasTVPreferredFocus onPress={() => layout && s.catalog.setHomeLayout(layout).then(() => { ui.toast('Home updated.'); back(); })} />
      </View>
    </Screen>
  );
}

/** Notifications inbox: all, unread, archived; mark read/unread, archive, mark all read. */
export function NotificationsScreen() {
  const s = useServices();
  const [view, setView] = useState<'all' | 'unread' | 'archived'>('all');
  const q = useQuery('notifications', () => s.account.notifications());
  const list = (q.data ?? []).filter(n => (view === 'archived' ? n.archived : !n.archived && (view === 'all' || !n.read)));
  const menu = (n: NotificationItem) => ui.menu({title: n.title, actions: [
    {label: n.read ? 'Mark as unread' : 'Mark as read', icon: 'check', onPress: () => s.account.updateNotification(n.id, {read: !n.read})},
    {label: n.archived ? 'Move to inbox' : 'Archive', icon: 'archive', onPress: () => s.account.updateNotification(n.id, {archived: !n.archived})},
  ]});
  const icon = (n: NotificationItem) => (n.kind === 'security' ? 'shield' : n.kind === 'urgent' ? 'alert' : n.kind === 'server' ? 'server' : n.kind === 'notice' ? 'message' : 'bell');
  return (
    <Screen heading="Notifications" eyebrow="Inbox">
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 10, alignItems: 'center', marginBottom: u(14)}}>
        <Segmented value={view} onChange={setView} options={[{value: 'all', label: 'All'}, {value: 'unread', label: 'Unread'}, {value: 'archived', label: 'Archived'}]} />
        <Button label="Mark all as read" size="sm" icon="check" onPress={() => s.account.markAllRead()} />
      </View>
      {q.data && !list.length ? <EmptyState icon="bell" title={view === 'archived' ? 'Nothing archived' : 'You’re all caught up'} body="Messages from your server and account appear here." /> : null}
      {list.map((n, i) => (
        <ListRow key={n.id} icon={icon(n)} label={n.title} help={`${n.body} · ${new Date(n.at).toLocaleDateString('en-US', {month: 'short', day: 'numeric'})}${n.audience === 'admin' ? ' · Administrators' : ''}`} onPress={() => menu(n)} hasTVPreferredFocus={i === 0} right={!n.read ? <View style={{width: 10, height: 10, borderRadius: 5, backgroundColor: colors.accentStrong}} /> : undefined} />
      ))}
    </Screen>
  );
}

/** Every loading, empty and error treatment in one place for review. */
export function StatesScreen() {
  const layout = useLayout();
  return (
    <Screen heading="States" eyebrow="Design review">
      <Text variant="title">Loading</Text>
      <RowsSkeleton rows={1} />
      <Skeleton width="60%" height={u(20)} style={{marginVertical: u(8)}} />
      <Text variant="title" style={{marginTop: u(20)}}>Banners</Text>
      <Banner icon="refresh" tone="warn" title="Reconnecting to Ehler Home Theater…" body="Portico keeps trying and brings this page back as soon as it reconnects." />
      <Banner icon="refresh" tone="info" title="Scanning your files…" body="Titles appear here as Portico finds them." />
 <Banner icon="shieldCheck" tone="info" title="Accept ownership of this server’s Portico registration" body="You’re the owner of Ehler Home Theater now. Accept so your Portico Account holds its registration." action={{label: 'Accept', onPress: () => ui.toast('You now hold Ehler Home Theater’s Portico registration.')}} />
      <Banner icon="alert" tone="danger" title="This guide is out of date" body="Refresh to get the latest program information." action={{label: 'Refresh', onPress: () => ui.toast('Guide refresh started.')}} />
      <Text variant="title" style={{marginTop: u(20)}}>Empty</Text>
      <View style={{flexDirection: layout.compact ? 'column' : 'row'}}>
        <View style={{flex: 1}}><EmptyState compact icon="bookmark" title="Your Watchlist is empty" body="Add a title to your Watchlist from its page and it waits for you here." /></View>
        <View style={{flex: 1}}><EmptyState compact icon="filter" title="No titles match" body="Loosen a filter or clear them all." action={{label: 'Clear filters', onPress: () => {}}} /></View>
      </View>
      <Text variant="title" style={{marginTop: u(20)}}>Errors</Text>
      <ErrorState onRetry={() => ui.toast('Trying again…')} />
      <View style={{flexDirection: 'row', gap: 8}}>
        <Badge label="Recording" tone="record" /><Badge label="New" tone="amber" /><Badge label="4K" tone="neutral" /><Badge label="Healthy" tone="healthy" />
      </View>
    </Screen>
  );
}

/** Settings › About › Screen index: every screen and state, one tap away. */
export function ScreenIndexScreen() {
  const s = useServices();
  const groups = [...new Set(screenIndex.map(e => e.group))];
  return (
    <Screen heading="Screen index" eyebrow="About · Review">
      <Text variant="body" tone="secondary" style={{marginBottom: u(12)}}>Every screen is also reachable with portico-cinema://screen/&lt;id&gt;.</Text>
      {groups.map(g => (
        <Panel key={g} title={g}>
          {screenIndex.filter(e => e.group === g).map((e, i) => <ListRow key={e.id} label={e.label} value={e.id} compact onPress={() => void e.open(s)} hasTVPreferredFocus={g === groups[0] && i === 0} />)}
        </Panel>
      ))}
    </Screen>
  );
}

export {isTV};
