import React, {useEffect, useState} from 'react';
import {ScrollView, View} from 'react-native';
import {useAction, useClient, useQuery} from '../data/DataProvider';
import type {Entry} from '../data/types';
import {Sheet} from '../ui/Sheet';
import {Button, IconButton} from '../ui/Button';
import {Chip, ListRow, Loading, Notice, Tabs, TextField, Toggle} from '../ui/Controls';
import {Text} from '../ui/Text';
import {Artwork} from '../ui/Artwork';
import {Focusable} from '../ui/Focusable';
import {colors} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import {useToast} from './toast';
import {catalog, backdropFor, posterFor, stillFor} from '../data/fixtures/catalog';

type Tab = 'general' | 'artwork' | 'tags' | 'people' | 'match' | 'history';
const FIELDS: {key: string; label: string; multiline?: boolean}[] = [
  {key: 'title', label: 'Title'}, {key: 'sortTitle', label: 'Sort title'}, {key: 'originalTitle', label: 'Original title'}, {key: 'year', label: 'Year'},
  {key: 'releaseDate', label: 'Release date'}, {key: 'contentRating', label: 'Content rating'}, {key: 'studio', label: 'Studio'}, {key: 'tagline', label: 'Tagline'}, {key: 'summary', label: 'Summary', multiline: true},
];

/** Edit metadata… for one title, or several at once (Selection › Edit). */
export function MetadataEditor({entries, onClose}: {entries?: Entry[]; onClose: () => void}) {
  const client = useClient();
  const toast = useToast();
  const m = useMetrics();
  const visible = !!entries?.length;
  const one = entries?.length === 1 ? entries[0] : undefined;
  const [tab, setTab] = useState<Tab>('general');
  const [values, setValues] = useState<Record<string, string>>({});
  const [locked, setLocked] = useState<Set<string>>(new Set(['title']));
  const [lockEdited, setLockEdited] = useState(true);
  const [genres, setGenres] = useState<string[]>([]);
  const [genreDraft, setGenreDraft] = useState('');
  const [query, setQuery] = useState('');
  const [poster, setPoster] = useState(0);
  const {busy, run, error} = useAction();
  const meta = useQuery(c => (one ? c.catalog.metadata(one.id) : Promise.resolve({} as Record<string, string>)), [one?.id]);
  const candidates = useQuery(c => (one && tab === 'match' ? c.catalog.matchCandidates(one.id, query) : Promise.resolve([])), [one?.id, tab, query]);
  useEffect(() => {
    if (!visible) { setTab('general'); return; }
    setValues(meta.data ?? {});
    setGenres((meta.data?.genres ?? '').split(',').map(s => s.trim()).filter(Boolean));
  }, [visible, meta.data]);
  if (!entries) return null;
  const set = (k: string, v: string) => setValues(prev => ({...prev, [k]: v}));
  const save = () => void run(async () => {
    await client.catalog.saveMetadata(entries.map(e => e.id), {...values, genres: genres.join(', ')}, lockEdited);
    onClose();
    toast({message: one ? 'Metadata saved.' : `${entries.length} updated.`, icon: 'check'});
  });
  const posters = one ? [posterFor(one.id) ?? one.image, ...catalog.movies.slice(0, 5).map(x => posterFor(x.id))].filter(Boolean) : [];
  const backgrounds = one ? [backdropFor(one.id), stillFor(`${one.id}-a1`), stillFor(`${one.id}-a2`), stillFor(`${one.id}-a3`)] : [];
  const tabs: {value: Tab; label: string}[] = one ? [{value: 'general', label: 'General'}, {value: 'artwork', label: 'Artwork'}, {value: 'tags', label: 'Tags & genres'}, {value: 'people', label: 'Cast & crew'}, {value: 'match', label: 'Fix match'}, {value: 'history', label: 'History'}] : [{value: 'general', label: 'General'}, {value: 'tags', label: 'Tags & genres'}];
  return (
    <Sheet visible={visible} onClose={onClose} title={one ? `Edit ${one.title}` : `Edit ${entries.length} items`} subtitle={one ? 'Locked fields keep your value when metadata refreshes.' : 'Fill only the fields to change; blanks are left alone.'} size="lg"
      footer={<><Toggle label="Lock edited fields" value={lockEdited} onChange={setLockEdited} /><View style={{flex: 1}} /><Button label="Cancel" variant="ghost" onPress={onClose} /><Button label={one ? 'Save changes' : `Apply to ${entries.length} items`} variant="primary" loading={busy} onPress={save} /></>}>
      <Tabs tabs={tabs} value={tab} onChange={setTab} />
      {error ? <Notice tone="error" body={error} /> : null}
      {tab === 'general' ? (meta.loading && one ? <Loading label="Loading metadata" /> : (
        <View style={{gap: m.space.md}}>
          {one ? (
            <View style={{flexDirection: 'row', gap: m.space.sm}}>
              <Button size="sm" icon="lock" label="Lock all" variant="ghost" onPress={() => setLocked(new Set(FIELDS.map(f => f.key)))} />
              <Button size="sm" icon="unlock" label="Unlock all" variant="ghost" onPress={() => setLocked(new Set())} />
            </View>
          ) : null}
          {FIELDS.map(f => (
            <View key={f.key} style={{flexDirection: 'row', alignItems: 'flex-end', gap: m.space.sm}}>
              <TextField style={{flex: 1}} label={f.label} value={one ? values[f.key] ?? '' : values[f.key] ?? ''} placeholder={one ? undefined : 'Mixed · leave unchanged'} onChangeText={v => set(f.key, v)} multiline={f.multiline} />
              {one ? <IconButton icon={locked.has(f.key) ? 'lock' : 'unlock'} label={locked.has(f.key) ? 'Locked: refreshes won’t change this' : 'Unlocked: a refresh may change this'} selected={locked.has(f.key)} variant="secondary" onPress={() => setLocked(prev => { const n = new Set(prev); if (n.has(f.key)) n.delete(f.key); else n.add(f.key); return n; })} /> : null}
            </View>
          ))}
        </View>
      )) : null}
      {tab === 'artwork' && one ? (
        <View style={{gap: m.space.md}}>
          <Text variant="captionStrong" tone="secondary">Poster</Text>
          <ScrollView horizontal contentContainerStyle={{gap: m.space.sm}}>
            {posters.map((p, i) => (
              <Focusable key={i} onPress={() => setPoster(i)} focusScale={1.05} style={({focused}) => ({borderRadius: 10, borderWidth: 3, borderColor: focused ? colors.focus : poster === i ? colors.accent : 'transparent'})}>
                <Artwork source={p} title={one.title} width={m.isTV ? 140 : 96} height={m.isTV ? 210 : 144} radius={8} />
                {i === 0 ? <Text variant="micro" tone="accent" center>In use</Text> : null}
              </Focusable>
            ))}
          </ScrollView>
          <Text variant="captionStrong" tone="secondary">Background</Text>
          <ScrollView horizontal contentContainerStyle={{gap: m.space.sm}}>
            {backgrounds.map((p, i) => <Artwork key={i} source={p} title={one.title} width={m.isTV ? 320 : 200} height={m.isTV ? 180 : 112} radius={8} />)}
          </ScrollView>
          <View style={{flexDirection: 'row', gap: m.space.sm}}><Button label="Find more" icon="search" variant="secondary" /><Button label="Re-fetch chosen" icon="refresh" variant="ghost" /></View>
        </View>
      ) : null}
      {tab === 'tags' ? (
        <View style={{gap: m.space.md}}>
          <Text variant="captionStrong" tone="secondary">Genres</Text>
          <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: m.space.xs}}>
            {genres.length ? genres.map(g => <Chip key={g} label={g} selected onClear={() => setGenres(genres.filter(x => x !== g))} onPress={() => setGenres(genres.filter(x => x !== g))} />) : <Text variant="caption" tone="tertiary">No genres.</Text>}
          </View>
          <TextField label="Add a genre" help="Type a genre and press Enter" value={genreDraft} onChangeText={setGenreDraft} onSubmitEditing={() => { if (genreDraft.trim()) setGenres([...genres, genreDraft.trim()]); setGenreDraft(''); }} />
          <TextField label="Labels" help="Comma separated. Labels can hide titles from a profile." value={values.labels ?? ''} onChangeText={v => set('labels', v)} />
        </View>
      ) : null}
      {tab === 'people' && one ? <CastEditor id={one.id} /> : null}
      {tab === 'match' && one ? (
        <View style={{gap: m.space.md}}>
          <Notice tone="info" title="Current match" body={`${one.title} (${one.year ?? '—'}) · TMDB · Matched`} />
          <TextField label="Search the provider" help="Title, or a provider id" value={query} onChangeText={setQuery} placeholder={one.title} icon="search" />
          {candidates.loading ? <Loading label="Searching" /> : candidates.data?.map(c => (
            <ListRow key={c.id} left={<Artwork source={c.image} title={c.title} width={m.isTV ? 60 : 40} height={m.isTV ? 90 : 60} radius={4} />} title={`${c.title} (${c.year})`} subtitle={`${c.provider} · ${c.score}% match · ${c.id}`} onPress={() => void run(async () => { await client.catalog.fixMatch(one.id, c.id); onClose(); toast({message: 'Match updated. Details and artwork refresh in a moment.'}); })} chevron={false} />
          ))}
        </View>
      ) : null}
      {tab === 'history' ? (
        <View>
          {[['Title', 'Maya · 3 days ago'], ['Poster', 'Justin · 2 weeks ago'], ['Summary', 'Justin · 1 month ago']].map(([f, w]) => <ListRow key={f} icon="history" title={`${f} edited`} subtitle={w} trailing={<Button size="sm" label="Undo to before this" variant="ghost" />} />)}
        </View>
      ) : null}
    </Sheet>
  );
}

function CastEditor({id}: {id: string}) {
  const m = useMetrics();
  const q = useQuery(c => (c.catalog.entry(id)?.kind === 'movie' ? c.catalog.movie(id) : Promise.resolve(undefined)), [id]);
  if (q.loading) return <Loading />;
  const cast = q.data ? [...q.data.directors, ...q.data.cast] : [];
  return (
    <View style={{gap: m.space.xs}}>
      {cast.length ? cast.map(c => <ListRow key={c.personId + (c.character ?? c.job)} left={<Artwork source={c.image} title={c.name} width={m.isTV ? 52 : 36} height={m.isTV ? 52 : 36} round />} title={c.name} subtitle={c.character ? `Character: ${c.character}` : `Department: ${c.job}`} trailing={<IconButton icon="trash" label={`Remove ${c.name}`} size={m.controlSmall} />} />) : <Text variant="caption" tone="tertiary">Credits are kept on individual items. Open a movie or episode to edit them.</Text>}
      <Button label="Add person" icon="plus" variant="secondary" />
    </View>
  );
}
