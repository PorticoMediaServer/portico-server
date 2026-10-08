import React, {useEffect, useState} from 'react';
import {ScrollView, StyleSheet, View} from 'react-native';
import {useRoute, type RouteProp} from '@react-navigation/native';
import {useQuery, useServices} from '../../data/hooks';
import type {Credit, Item, Movie} from '../../data/types';
import {Artwork} from '../../design/Artwork';
import {Button, IconButton} from '../../design/Button';
import {Badge, Chip, ListRow, Panel, Segmented, TextField, ToggleRow} from '../../design/Controls';
import {Focusable} from '../../design/Focusable';
import {Icon} from '../../design/Icon';
import {ui} from '../../design/overlays';
import {EmptyState} from '../../design/States';
import {Text} from '../../design/Text';
import {colors, isTV, radius, u} from '../../design/tokens';
import {useLayout} from '../../design/useLayout';
import {Screen} from '../../nav/Screen';
import {back, go, type Routes} from '../../nav/routes';
import {useSession} from '../../state/session';
import {plural} from '../../design/format';

type Tab = 'general' | 'artwork' | 'tags' | 'people' | 'match' | 'history';

/** Owner metadata editor (web: shared/MetadataEditor): fields with locks, artwork, genres, credits, fix match and history. */
export function MetadataEditorScreen() {
  const route = useRoute<RouteProp<Routes, 'MetadataEditor'>>();
  const s = useServices();
  const session = useSession();
  const bulk = route.params.ids && route.params.ids.length > 1 ? route.params.ids : null;
  const q = useQuery('item:' + route.params.id, () => s.catalog.item(route.params.id));
  const it = q.data as Movie | undefined;
  const [tab, setTab] = useState<Tab>('general');
  if (session.role === 'member') return <Screen><EmptyState icon="lock" title="Owner access is no longer available." body="Only the server owner and administrators can edit metadata." /></Screen>;
  if (!it) return <Screen heading="Edit metadata" />;
  if (bulk) return <BulkEditor ids={bulk} />;
  return (
    <Screen title="Edit metadata">
      <View style={{flexDirection: 'row', alignItems: 'center', gap: u(14), marginBottom: u(14)}}>
        <Artwork art={it.art.poster ?? it.art.square} tint={it.tint} style={{width: u(60), height: u(90), borderRadius: 8}} />
        <View style={{flex: 1}}>
          <Text variant="eyebrow" tone="accent">Edit metadata</Text>
          <Text variant="display" numberOfLines={1}>{it.title}</Text>
          <Text variant="caption" tone="tertiary">{it.kind} · {it.year} · Matched · TMDB 58112</Text>
        </View>
      </View>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{gap: u(8), marginBottom: u(14)}}>
        {([['general', 'General'], ['artwork', 'Artwork'], ['tags', 'Tags & genres'], ['people', 'Cast & crew'], ['match', 'Fix match'], ['history', 'History']] as const).map(([id, l], i) => <Chip key={id} label={l} active={tab === id} onPress={() => setTab(id)} hasTVPreferredFocus={i === 0} />)}
      </ScrollView>
      {tab === 'general' ? <General it={it} /> : null}
      {tab === 'artwork' ? <ArtworkTab it={it} /> : null}
      {tab === 'tags' ? <Tags it={it} /> : null}
      {tab === 'people' ? <People credits={'credits' in it ? it.credits : []} /> : null}
      {tab === 'match' ? <Match it={it} /> : null}
      {tab === 'history' ? <History /> : null}
    </Screen>
  );
}

function Field({label, value, onChange, locked, onLock, multiline, help}: {label: string; value: string; onChange: (v: string) => void; locked: boolean; onLock: () => void; multiline?: boolean; help?: string}) {
  return (
    <View style={{flexDirection: 'row', alignItems: 'flex-start', gap: 8}}>
      <TextField label={label} value={value} onChangeText={onChange} multiline={multiline} help={help ?? (locked ? 'Locked: refreshes won’t change this' : 'Unlocked: a refresh may change this')} style={{flex: 1}} autoCapitalize="sentences" />
      <View style={{marginTop: u(24)}}><IconButton icon={locked ? 'lock' : 'unlock'} label={locked ? `Unlock ${label}` : `Lock ${label}`} active={locked} onPress={onLock} size={38} /></View>
    </View>
  );
}

function General({it}: {it: Movie}) {
  const [f, setF] = useState({title: it.title, sortTitle: it.sortTitle, year: String(it.year ?? ''), tagline: it.tagline ?? '', summary: it.synopsis ?? '', rating: it.contentRating, studio: it.studio ?? '', released: `${it.year}-05-17`});
  const [locks, setLocks] = useState<Record<string, boolean>>({title: true});
  const lock = (k: string) => setLocks(l => ({...l, [k]: !l[k]}));
  const field = (k: keyof typeof f, label: string, multiline?: boolean) => <Field label={label} value={f[k]} onChange={v => { setF({...f, [k]: v}); setLocks(l => ({...l, [k]: true})); }} locked={!!locks[k]} onLock={() => lock(k)} multiline={multiline} />;
  return (
    <Panel>
      {field('title', 'Title')}
      {field('sortTitle', 'Sort title')}
      {field('year', 'Year')}
      {field('released', 'Release date')}
      {field('rating', 'Content rating')}
      {field('studio', 'Studio')}
      {field('tagline', 'Tagline')}
      {field('summary', 'Summary', true)}
      <View style={{flexDirection: 'row', gap: 10, marginTop: 6, flexWrap: 'wrap'}}>
        <Button label="Lock everything" icon="lock" size="sm" onPress={() => setLocks(Object.fromEntries(Object.keys(f).map(k => [k, true])))} />
        <Button label="Unlock everything" icon="unlock" size="sm" onPress={() => setLocks({})} />
        <Button label="Save changes" variant="primary" onPress={() => { if (!f.title.trim()) return ui.toast('A title is required.', {tone: 'error'}); if (!/^\d{4}$/.test(f.year)) return ui.toast('Year must be between 1870 and 2100.', {tone: 'error'}); ui.toast('Metadata saved. Edited fields are locked.'); back(); }} />
      </View>
    </Panel>
  );
}

function ArtworkTab({it}: {it: Movie}) {
  const layout = useLayout();
  const [role, setRole] = useState<'poster' | 'backdrop' | 'logo'>('poster');
  const s = useServices();
  const others = useQuery('art-candidates:' + it.id, async () => (await s.catalog.browse({libraryId: it.libraryId, filters: {quick: [], fields: {genre: it.genres.slice(0, 1)}}, sort: {field: 'random', direction: 'asc'}, pageSize: 6})).items);
  const w = isTV ? 180 : layout.compact ? 96 : 130;
  const cands = [it, ...(others.data ?? []).filter(x => x.id !== it.id)].slice(0, 6);
  const [inUse, setInUse] = useState(it.id);
  return (
    <Panel>
      <Segmented value={role} onChange={setRole} options={[{value: 'poster', label: 'Poster'}, {value: 'backdrop', label: 'Background'}, {value: 'logo', label: 'Logo'}]} />
      {role === 'logo' ? <Text variant="body" tone="tertiary" style={{marginTop: 12}}>Nothing to choose from for this role yet. Find more asks the providers again.</Text> : (
        <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 12, marginTop: 14}}>
          {cands.map((c: Item) => (
            <Focusable key={c.id} onPress={() => setInUse(c.id)} scale={1.06} accessibilityLabel={`Use artwork from ${c.title}`}>
              {({focused}) => (
                <View>
                  <Artwork art={role === 'poster' ? c.art.poster : c.art.backdrop} tint={c.tint} style={[{width: role === 'poster' ? w : w * 1.6, height: role === 'poster' ? w * 1.5 : w * 0.9, borderRadius: 8}, (inUse === c.id || (focused && isTV)) && {borderWidth: 3, borderColor: inUse === c.id ? colors.accentStrong : colors.white}]} />
                  {inUse === c.id ? <Badge label="In use" /> : <Text variant="micro" tone="tertiary">{c.id === it.id ? 'Local file' : 'TMDB'} · 2000×3000</Text>}
                </View>
              )}
            </Focusable>
          ))}
        </View>
      )}
      <View style={{flexDirection: 'row', gap: 10, marginTop: 14}}>
        <Button label="Find more" icon="search" size="sm" onPress={() => ui.toast('Asking the providers again…')} />
        <Button label="Show all pictures" size="sm" variant="ghost" onPress={() => ui.toast('Showing 14 more candidates.')} />
        <Button label="Save" variant="primary" size="sm" onPress={() => ui.toast('Artwork saved.')} />
      </View>
    </Panel>
  );
}

function Tags({it}: {it: Movie}) {
  const [genres, setGenres] = useState(it.genres);
  const [locked, setLocked] = useState(false);
  const [add, setAdd] = useState('');
  return (
    <Panel title="Genres" right={<Badge label={locked ? 'Locked' : 'Unlocked'} tone={locked ? 'amber' : 'neutral'} />}>
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 8, marginBottom: 12}}>
        {genres.map(g => <Chip key={g} label={g} icon="x" onPress={() => setGenres(genres.filter(x => x !== g))} />)}
        {!genres.length ? <Text variant="caption" tone="tertiary">No genres.</Text> : null}
      </View>
      <TextField label="Add a genre" value={add} onChangeText={setAdd} help="Type a genre and press Enter" onSubmit={() => { if (add.trim()) setGenres([...genres, add.trim()]); setAdd(''); }} autoCapitalize="words" />
      <ToggleRow label="Lock genres" help="Refreshes won’t change them" value={locked} onChange={setLocked} />
      <Text variant="captionStrong" tone="secondary" style={{marginTop: 10}}>Labels</Text>
      <View style={{flexDirection: 'row', gap: 8, marginVertical: 8}}><Chip label="Family favorite" /><Chip label="4K remaster" /></View>
      <Button label="Save genres" variant="primary" size="sm" onPress={() => ui.toast('Genres saved.')} style={{alignSelf: 'flex-start'}} />
    </Panel>
  );
}

function People({credits}: {credits: Credit[]}) {
  const [list, setList] = useState(credits);
  return (
    <Panel title="Cast & crew">
      {!list.length ? <Text variant="caption" tone="tertiary">No credits.</Text> : null}
      {list.map((c, i) => (
        <ListRow key={c.personId + i} label={c.name} help={c.character ? `Character · ${c.character}` : `${c.department} · ${c.job ?? ''}`} compact chevron={false}
          right={<View style={{flexDirection: 'row'}}><IconButton icon="arrowUp" label="Move up" size={30} variant="ghost" onPress={() => i > 0 && setList(l => { const n = [...l]; [n[i - 1], n[i]] = [n[i], n[i - 1]]; return n; })} /><IconButton icon="x" label={`Remove ${c.name}`} size={30} variant="ghost" onPress={() => setList(list.filter((_, k) => k !== i))} /></View>} />
      ))}
      <View style={{flexDirection: 'row', gap: 10, marginTop: 10}}>
        <Button label="Add person" icon="userPlus" size="sm" onPress={() => ui.prompt({title: 'Add person', fields: [{key: 'n', label: 'Name'}, {key: 'd', label: 'Department', placeholder: 'Acting, Directing…'}, {key: 'c', label: 'Character or role'}], submitLabel: 'Add', onSubmit: v => { if (!v.n.trim()) return 'Enter a name.'; setList([...list, {personId: 'new-' + v.n, name: v.n, department: (v.d as Credit['department']) || 'Acting', character: v.c}]); return null; }})} />
        <Button label="Save credits" variant="primary" size="sm" onPress={() => ui.toast('Credits saved.')} />
      </View>
    </Panel>
  );
}

function Match({it}: {it: Movie}) {
  const [query, setQuery] = useState(it.title);
  const [results, setResults] = useState<{id: string; title: string; year: number; votes: number}[]>([]);
  const [locked, setLocked] = useState(true);
  useEffect(() => {
    setResults([{id: 'tmdb-58112', title: it.title, year: it.year ?? 2000, votes: 1840}, {id: 'tmdb-91203', title: it.title, year: (it.year ?? 2000) - 23, votes: 212}, {id: 'tmdb-77410', title: `${it.title}: The Director’s Cut`, year: it.year ?? 2000, votes: 64}]);
  }, [it]);
  return (
    <View>
      <Panel title="Current match" right={<Badge label="Matched" tone="healthy" />}>
        <ListRow label={`${it.title} (${it.year})`} help="TMDB 58112 · Identity locked" right={<IconButton icon={locked ? 'lock' : 'unlock'} label={locked ? 'Unlock identity' : 'Lock identity'} active={locked} onPress={() => setLocked(!locked)} size={34} />} />
      </Panel>
      <Panel title="Search the provider" lede="Title, or a provider id">
        <TextField value={query} onChangeText={setQuery} icon="search" onSubmit={() => ui.toast('Searching TMDB…')} />
        {results.map((r, i) => (
          <ListRow key={r.id} label={`${r.title} (${r.year})`} help={`${r.id.replace('-', ' ').toUpperCase()} · ${r.votes} votes`} hasTVPreferredFocus={i === 0}
            onPress={() => ui.confirm({title: 'Use this match?', body: `Details and artwork are replaced from ${r.title} (${r.year}). Fields you locked are kept.`, confirmLabel: 'Use this match', onConfirm: () => { ui.toast('Match applied. Getting details…'); back(); }})} />
        ))}
      </Panel>
    </View>
  );
}

function History() {
  const rows = [
    {what: 'Summary edited', who: 'Justin', when: 'Today, 9:14 AM'},
    {what: 'Poster changed', who: 'Justin', when: 'Sep 12'},
    {what: 'Matched to TMDB 58112', who: 'Maya', when: 'Aug 30'},
  ];
  return (
    <Panel title="Recent changes">
      {rows.map(r => <ListRow key={r.what} icon="history" label={r.what} help={`${r.who} · ${r.when}`} value="Undo to before this" onPress={() => ui.confirm({title: 'Undo to before this change?', body: 'Later edits to these fields are rolled back too.', confirmLabel: 'Undo', onConfirm: () => ui.toast('Changes undone.')})} />)}
    </Panel>
  );
}

function BulkEditor({ids}: {ids: string[]}) {
  const [genreAdd, setGenreAdd] = useState('');
  const [genreRemove, setGenreRemove] = useState('');
  const [rating, setRating] = useState('');
  const [lockEdited, setLockEdited] = useState(true);
  return (
    <Screen heading={`Edit ${plural(ids.length, 'item', 'items')}`} eyebrow="Edit metadata">
      <Text variant="body" tone="secondary" style={{marginBottom: 12}}>Fill only the fields to change; blanks are left alone.</Text>
      <Panel>
        <TextField label="Content rating" value={rating} onChangeText={setRating} placeholder="Mixed · leave unchanged" />
        <TextField label="Add genres" value={genreAdd} onChangeText={setGenreAdd} help="Comma separated" />
        <TextField label="Remove genres" value={genreRemove} onChangeText={setGenreRemove} help="Comma separated" />
        <ToggleRow label="Lock edited fields so refreshes keep them" value={lockEdited} onChange={setLockEdited} />
        <Button label={`Apply to ${plural(ids.length, 'item', 'items')}`} variant="primary" onPress={() => { ui.toast(`${ids.length} updated. Every selected item was updated.`); go('Home'); }} style={{marginTop: 10, alignSelf: 'flex-start'}} />
      </Panel>
    </Screen>
  );
}

// ---------------------------------------------------------------- Feedback

export function FeedbackScreen() {
  const route = useRoute<RouteProp<Routes, 'Feedback'>>();
  const s = useServices();
  const session = useSession();
  const item = useQuery(route.params?.itemId ? 'item:' + route.params.itemId : null, () => s.catalog.item(route.params!.itemId!));
  const [kind, setKind] = useState(route.params?.itemId ? 'title' : 'app');
  const [category, setCategory] = useState('');
  const [message, setMessage] = useState('');
  const [diag, setDiag] = useState(true);
  const [sent, setSent] = useState<null | 'sent' | 'duplicate'>(null);
  const [error, setError] = useState<string | null>(null);
  const cats = kind === 'title' ? ['Wrong match', 'Subtitles', 'Audio', 'Picture quality', 'Playback', 'Missing episodes', 'Other'] : ['Playback', 'Something looks wrong', 'Idea', 'Other'];
  if (sent) return <Screen><EmptyState icon="checkCircle" title={sent === 'sent' ? 'Thanks, that’s been sent' : 'Already reported'} body={sent === 'sent' ? 'The server’s owner can see it now. Any reply will arrive in your notifications.' : 'You reported this recently, so it has been added to that report rather than sent again.'} action={{label: 'Done', onPress: back}} /></Screen>;
  return (
    <Screen heading="Report a problem" eyebrow={item.data ? `About “${item.data.title}”` : 'Feedback'}>
      <Panel>
        <Text variant="captionStrong" tone="secondary" style={{marginBottom: 6}}>What is it about?</Text>
        <Segmented value={kind} onChange={setKind} options={[...(route.params?.itemId ? [{value: 'title', label: 'This title'}] : []), {value: 'app', label: 'The app'}, {value: 'server', label: 'The server'}]} />
        <Text variant="captionStrong" tone="secondary" style={{marginTop: 14, marginBottom: 6}}>Category</Text>
        <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 8}}>{cats.map((c, i) => <Chip key={c} label={c} active={category === c} onPress={() => setCategory(c)} hasTVPreferredFocus={i === 0} />)}</View>
        <TextField label="What happened?" value={message} onChangeText={setMessage} multiline placeholder="What you were doing, what you expected, and what happened instead." help={`${message.length} / 2000`} maxLength={2000} style={{marginTop: 14}} autoCapitalize="sentences" />
        <ToggleRow label={route.params?.playback ? 'Include technical details about this playback' : 'Include technical details'} help="How the stream was delivered and what the player reported. No viewing history beyond this title." value={diag} onChange={setDiag} />
        {error ? <Text variant="caption" tone="danger">{error}</Text> : null}
        <Text variant="caption" tone="tertiary" style={{marginVertical: 8}}>Sent as {session.profile?.name} to this server’s owner.</Text>
        <Button label="Send" icon="message" variant="primary" style={{alignSelf: 'flex-start'}} onPress={() => { if (!category) return setError('Choose a category.'); if (message.trim().length < 10) return setError('Describe it in at least 10 characters.'); s.feedback.send({kind, category, message, itemId: route.params?.itemId, diagnostics: diag}).then(setSent); }} />
      </Panel>
    </Screen>
  );
}

const styles = StyleSheet.create({card: {borderRadius: radius.md}});
export {styles, Icon};
