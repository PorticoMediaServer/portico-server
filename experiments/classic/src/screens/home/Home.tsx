import React, {useState} from 'react';
import {View} from 'react-native';
import {useClient, useQuery} from '../../data/DataProvider';
import type {HomeRow} from '../../data/types';
import {Screen} from '../../ui/Page';
import {Shelf, ShelfSkeleton} from '../../ui/Shelf';
import {Button} from '../../ui/Button';
import {EmptyState, Skeleton} from '../../ui/Controls';
import {useMetrics} from '../../theme/layout';
import {useCardHandlers} from '../../app/useCards';
import {useActions} from '../../app/actions';
import {useSession} from '../../app/session';
import {useToast} from '../../app/toast';
import {push, root} from '../../nav/nav';
import {Hero} from './Hero';
import {CustomizeHome} from './CustomizeHome';
import {ErrorView} from '../../ui/Page';

/**
 * Home: the featured hero, then every row the server composed for this
 * profile, in the profile's order (Continue Watching, Up Next, recently added
 * per library, Picks for you, Because you watched…).
 */
export function HomeScreen() {
  const m = useMetrics();
  const client = useClient();
  const actions = useActions();
  const toast = useToast();
  const {profile, isAdmin} = useSession();
  const doc = useQuery(c => c.home.document(), [profile?.id], ['home', 'personal', 'saved']);
  const cards = useCardHandlers();
  const continueCards = useCardHandlers('continue');
  const [customize, setCustomize] = useState(false);
  const seeAll = (row: HomeRow) => {
    if (row.id.startsWith('view:')) return push('Resource', {kind: 'view', id: row.id.slice(5)});
    if (row.kind === 'live') return root('Live', {view: 'guide'});
    if (row.kind === 'watchlist') return root('Saved', {tab: 'watchlist'});
    if (row.kind === 'favorites') return root('Saved', {tab: 'favorites'});
    return push('HomeRow', {rowId: row.id, title: row.title});
  };
  const d = doc.data;
  return (
    <Screen bleedTop refreshing={doc.loading} onRefresh={doc.refresh} contentStyle={{gap: m.rowGap}}>
      {d?.featured.length ? (
        <Hero items={d.featured} eyebrow="Featured"
          onPlay={e => void actions.play(e)} onDetails={e => actions.open(e)}
          watchlisted={e => client.personal.get(e.id).watchlisted}
          onWatchlist={async e => { const on = !client.personal.get(e.id).watchlisted; await client.personal.setWatchlist([e.id], on); toast({message: on ? 'Added to Watchlist.' : 'Removed from Watchlist.', icon: 'saved'}); doc.refresh(); }} />
      ) : !d ? <HeroSkeleton /> : <View style={{height: m.isTV ? 60 : 110}} />}
      {doc.error && !d ? <ErrorView message={doc.error} onRetry={doc.refresh} /> : null}
      {!d && !doc.error ? <><ShelfSkeleton count={7} /><ShelfSkeleton /><ShelfSkeleton /></> : null}
      {d?.rows.map((row, i) => {
        const h = row.kind === 'continue' ? continueCards : cards;
        return (
          <Shelf key={row.id} title={row.title} subtitle={row.reason} entries={row.entries} shape={row.shape} total={row.total}
            onPress={h.onPress} onLongPress={h.onLongPress} onFocus={h.onFocus}
            onSeeAll={row.total > 6 || row.kind === 'recent' || row.kind === 'picks' ? () => seeAll(row) : undefined}
            first={!d.featured.length && i === 0} />
        );
      })}
      {d && !d.rows.length && !d.featured.length ? (
        <EmptyState icon="home" title="Nothing here yet" body={isAdmin ? 'Titles appear here once your libraries have been scanned.' : 'Titles appear here once the owner adds media to the libraries you can see.'} action={isAdmin ? {label: 'Add library', icon: 'plus', onPress: () => root('Admin', {section: 'libraries'})} : undefined} />
      ) : null}
      {d && (d.rows.length || d.featured.length) ? (
        <View style={{alignItems: 'center', paddingHorizontal: m.gutter, marginTop: -m.rowGap / 3}}>
          <Button label="Customize Home" icon="sliders" variant="secondary" size="sm" onPress={() => setCustomize(true)} />
        </View>
      ) : null}
      <CustomizeHome visible={customize} onClose={() => setCustomize(false)} />
    </Screen>
  );
}

function HeroSkeleton() {
  const m = useMetrics();
  return (
    <View style={{height: m.hero, justifyContent: 'flex-end', padding: m.gutter, gap: 12}}>
      <Skeleton width={m.isTV ? 520 : 240} height={m.type.display.size} />
      <Skeleton width={m.isTV ? 380 : 180} height={m.type.caption.size} />
      <Skeleton width={m.isTV ? 700 : 300} height={m.type.body.size * 2} />
    </View>
  );
}
