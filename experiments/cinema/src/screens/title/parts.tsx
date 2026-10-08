import React, {useState} from 'react';
import {StyleSheet, View} from 'react-native';
import type {Services} from '../../data/services';
import type {Credit, Item, MediaVersion, Movie, Episode} from '../../data/types';
import {useQuery, useServices} from '../../data/hooks';
import {Button, IconButton} from '../../design/Button';
import {Badge, ListRow} from '../../design/Controls';
import {Focusable} from '../../design/Focusable';
import {Icon, type IconName} from '../../design/Icon';
import {ui} from '../../design/overlays';
import {Row} from '../../design/Row';
import {Text} from '../../design/Text';
import {colors, isTV, radius, u} from '../../design/tokens';
import {useLayout} from '../../design/useLayout';
import {push} from '../../nav/routes';
import {isManager, useSession} from '../../state/session';
import {openItemMenu, openRateSheet, openDownloadSheet, useItemState, type Origin} from '../shared/actions';
import {fmtRemaining, openItem, personCard, playItem} from '../shared/cards';
import {useItems} from '../shared/useItems';

/** Per-title playback choices made on the detail page (version, audio, subtitles). */
const choices = new Map<string, {version?: string; audio?: string; subtitle?: string}>();
export const playbackChoice = (id: string) => choices.get(id) ?? {};

/**
 * The action bar. Play/Resume leads; list toggles are round buttons with labels on touch; on TV
 * every control is one focus stop in a single row, with the focused one lit white.
 */
export function ActionBar({item, origin, extra, hidePlay}: {item: Item; origin?: Origin; extra?: React.ReactNode; hidePlay?: boolean}) {
  const s = useServices();
  const session = useSession();
  const st = useItemState(item.id);
  const layout = useLayout();
  const resumable = st.progress > 0 && !st.watched;
  const toggles: {icon: IconName; label: string; on: boolean; press: () => void}[] = [
    {icon: st.watchlist ? 'bookmarkCheck' : 'plus', label: 'Watchlist', on: st.watchlist, press: () => s.user.set([item.id], {watchlist: !st.watchlist})},
    {icon: 'heart', label: 'Favorite', on: st.favorite, press: () => s.user.set([item.id], {favorite: !st.favorite})},
    {icon: 'checkCircle', label: st.watched ? 'Watched' : 'Mark watched', on: st.watched, press: () => s.user.set([item.id], {watched: !st.watched})},
    {icon: 'star', label: st.rating ? `${st.rating} ★` : 'Rate', on: st.rating > 0, press: () => openRateSheet(s, item)},
    ...(!isTV ? [{icon: 'download' as IconName, label: 'Download', on: false, press: () => openDownloadSheet(s, item)}] : []),
  ];
  const more = () => openItemMenu(s, item, {origin, manager: isManager(session.role)});
  return (
    <View style={{marginTop: u(16)}}>
      <View style={styles.primary}>
        {hidePlay ? null : <Button
          label={resumable ? `Resume · ${fmtRemaining((1 - st.progress) * s.catalog.durationOf(item.id))}` : 'Play'}
          icon="play"
          variant="primary"
          size="lg"
          hasTVPreferredFocus
          onPress={() => playItem(item, {from: resumable ? st.position : 0})}
        />}
        {resumable && !hidePlay ? <Button label="Play from beginning" icon="skipBack" size="lg" onPress={() => playItem(item, {from: 0})} /> : null}
        {extra}
        {isTV ? toggles.map(t => <IconButton key={t.label} icon={t.icon} label={t.label} size={50} active={t.on} fill={t.on && (t.icon === 'heart' || t.icon === 'star')} onPress={t.press} />) : null}
        {isTV ? <IconButton icon="more" label="More" size={50} onPress={more} /> : null}
      </View>
      {!isTV ? (
        <View style={styles.toggles}>
          {toggles.map(t => (
            <Focusable key={t.label} onPress={t.press} accessibilityRole="button" accessibilityLabel={t.label} accessibilityState={{selected: t.on}} style={styles.toggle}>
              <Icon name={t.icon} size={22} color={t.on ? colors.accentStrong : colors.text} fill={t.on && (t.icon === 'heart' || t.icon === 'star') ? true : undefined} />
              <Text variant="micro" color={t.on ? colors.accentStrong : colors.textSecondary} style={{marginTop: 4}}>{t.label}</Text>
            </Focusable>
          ))}
          <Focusable onPress={more} accessibilityRole="button" accessibilityLabel="More actions" style={styles.toggle}>
            <Icon name="more" size={22} color={colors.text} />
            <Text variant="micro" tone="secondary" style={{marginTop: 4}}>More</Text>
          </Focusable>
        </View>
      ) : null}
    </View>
  );
}

export function Section({title, children, right}: {title: string; children: React.ReactNode; right?: React.ReactNode}) {
  const layout = useLayout();
  return (
    <View style={{marginTop: u(26)}}>
      <View style={{flexDirection: 'row', alignItems: 'center', paddingHorizontal: layout.gutter, marginBottom: u(8)}}>
        <Text variant="title" style={{flex: 1}}>{title}</Text>
        {right}
      </View>
      {children}
    </View>
  );
}

export function CastRow({credits, title = 'Cast & Crew'}: {credits: Credit[]; title?: string}) {
  const s = useServices();
  const ids = [...new Set(credits.map(c => c.personId))];
  const q = useQuery('people:' + ids.join(','), async () => Promise.all(ids.map(id => s.catalog.person(id))));
  const people = (q.data ?? []).filter(Boolean) as NonNullable<Awaited<ReturnType<Services['catalog']['person']>>>[];
  const ordered = [...credits.filter(c => c.job === 'Director'), ...credits.filter(c => c.department === 'Acting'), ...credits.filter(c => c.job && c.job !== 'Director')];
  const seen = new Set<string>();
  const cards = ordered.filter(c => !seen.has(c.personId + (c.job ?? '')) && seen.add(c.personId + (c.job ?? ''))).map(c => {
    const p = people.find(x => x.id === c.personId);
    return p ? personCard(p, c.character ?? c.job) : null;
  }).filter(Boolean) as ReturnType<typeof personCard>[];
  if (!cards.length) return null;
  return <Row title={title} items={cards} onPressItem={c => push('Person', {id: c.id})} />;
}

export function RelatedRow({id, title = 'More like this'}: {id: string; title?: string}) {
  const s = useServices();
  const q = useQuery('related:' + id, () => s.catalog.related(id));
  const items = useItems(q.data);
  const session = useSession();
  if (!q.data?.length) return null;
  return <Row title={title} items={items.cards(q.data)} onPressItem={c => { const it = items.lookup(c.id); if (it) openItem(it); }} onLongPressItem={c => { const it = items.lookup(c.id); if (it) openItemMenu(s, it, {origin: 'recommendation', manager: isManager(session.role)}); }} />;
}

export function ExtrasRow({item}: {item: Movie}) {
  if (!item.extras.length) return null;
  return (
    <Row
      title="Extras"
      items={item.extras.map(e => ({id: e.id, title: e.title, subtitle: `${e.kind} · ${Math.round(e.duration / 60)} min`, art: e.art, tint: item.tint, shape: 'landscape' as const}))}
      onPressItem={() => push('Player', {id: item.id, trailer: true})}
    />
  );
}

/** Facts, versions, audio/subtitles and media information. */
export function DetailsPanel({item}: {item: Movie | Episode}) {
  const layout = useLayout();
  const version = item.versions[0];
  const credits = item.credits;
  const directors = credits.filter(c => c.job === 'Director').map(c => c.name);
  const writers = credits.filter(c => c.department === 'Writing').map(c => c.name);
  const facts: [string, string][] = [];
  if (directors.length) facts.push(['Directed by', directors.join(', ')]);
  if (writers.length) facts.push(['Written by', writers.join(', ')]);
  if ('studio' in item && item.studio) facts.push(['Studio', item.studio]);
  if ('country' in item && item.country) facts.push(['Country', item.country]);
  if ('language' in item && item.language) facts.push(['Language', item.language]);
  if ('license' in item && item.license) facts.push(['License', item.license]);
  if ('place' in item && item.place) facts.push(['Place', item.place]);
  facts.push(['Audio', item.audioTracks.map(a => a.label).join(', ')]);
  if (item.subtitleTracks.length) facts.push(['Subtitles', item.subtitleTracks.map(a => a.label).join(', ')]);
  return (
    <Section title="Details">
      <View style={[styles.details, {paddingHorizontal: layout.gutter}, !layout.compact && {flexDirection: 'row', gap: u(30)}]}>
        <View style={{flex: 1}}>
          {facts.map(([k, v]) => (
            <View key={k} style={styles.fact}>
              <Text variant="eyebrow" tone="tertiary">{k}</Text>
              <Text variant="body" tone="secondary">{v}</Text>
            </View>
          ))}
        </View>
        <View style={{flex: 1}}>
          <Text variant="eyebrow" tone="tertiary" style={{marginBottom: u(6)}}>Media information</Text>
          {item.versions.map((v, i) => (
            <View key={v.id} style={[styles.mediaCard, i > 0 && {marginTop: u(8)}]}>
              <View style={{flexDirection: 'row', alignItems: 'center', gap: 8, marginBottom: 6}}>
                <Text variant="bodyStrong">{v.label}</Text>
                {v.hdr ? <Badge label={v.hdr} tone="amber" /> : null}
                {v.edition ? <Badge label={v.edition} /> : null}
              </View>
              {[['Container', v.container], ['Video', v.video], ['Audio', v.audio], ['Size', `${v.sizeGB} GB · ${v.bitrateMbps} Mb/s`]].map(([k, val]) => (
                <View key={k} style={{flexDirection: 'row'}}>
                  <Text variant="caption" tone="tertiary" style={{width: u(80)}}>{k}</Text>
                  <Text variant="caption" tone="secondary" style={{flex: 1}}>{val}</Text>
                </View>
              ))}
            </View>
          ))}
          {!version ? <Text variant="caption" tone="tertiary">This title’s file isn’t available right now.</Text> : null}
        </View>
      </View>
    </Section>
  );
}

/** Version and track pickers shown as chips under the synopsis. */
export function PlaybackChips({item}: {item: Movie | Episode}) {
  const [, force] = useState(0);
  const c = playbackChoice(item.id);
  const version = item.versions.find(v => v.id === c.version) ?? item.versions[0];
  const audio = item.audioTracks.find(a => a.id === c.audio) ?? item.audioTracks[0];
  const sub = c.subtitle === 'off' || !c.subtitle ? null : item.subtitleTracks.find(t => t.id === c.subtitle);
  const set = (patch: {version?: string; audio?: string; subtitle?: string}) => {
    choices.set(item.id, {...c, ...patch});
    force(n => n + 1);
  };
  const chip = (icon: IconName, label: string, onPress: () => void) => (
    <Focusable onPress={onPress} scale={1.06} accessibilityRole="button" accessibilityLabel={label}>
      {({focused}) => (
        <View style={[styles.chip, focused && isTV && {backgroundColor: colors.white}]}>
          <Icon name={icon} size={14} color={focused && isTV ? colors.actionInk : colors.textSecondary} />
          <Text variant="captionStrong" color={focused && isTV ? colors.actionInk : colors.textSecondary} style={{marginLeft: 6}}>{label}</Text>
          <Icon name="chevronDown" size={12} color={focused && isTV ? colors.actionInk : colors.textTertiary} />
        </View>
      )}
    </Focusable>
  );
  if (!version) return null;
  return (
    <View style={styles.chips}>
      {item.versions.length > 1 ? chip('layers', version.label, () => pickVersion(item.versions, version, v => set({version: v}))) : chip('monitor', version.label, () => pickVersion(item.versions, version, v => set({version: v})))}
      {chip('audio', audio?.label ?? 'Audio', () => ui.menu({title: 'Audio', actions: item.audioTracks.map(a => ({label: a.label, detail: [a.codec, a.channels].filter(Boolean).join(' · '), checked: a.id === audio?.id, onPress: () => set({audio: a.id})}))}))}
      {chip('subtitles', sub ? sub.label : 'Subtitles off', () => ui.menu({title: 'Subtitles', actions: [{label: 'Off', checked: !sub, onPress: () => set({subtitle: 'off'})}, ...item.subtitleTracks.map(t => ({label: t.label, detail: t.origin === 'embedded' ? 'In the file' : t.origin === 'sidecar' ? 'Beside the file' : 'Downloaded', checked: t.id === sub?.id, onPress: () => set({subtitle: t.id})}))]}))}
    </View>
  );
}

function pickVersion(versions: MediaVersion[], current: MediaVersion, onPick: (id: string) => void) {
  ui.menu({title: 'Versions', subtitle: 'Choose which file plays', actions: versions.map(v => ({label: v.label, detail: `${v.video} · ${v.audio} · ${v.sizeGB} GB`, checked: v.id === current.id, onPress: () => onPick(v.id)}))});
}

export function MoreButton({item, origin}: {item: Item; origin?: Origin}) {
  const s = useServices();
  const session = useSession();
  return <IconButton icon="more" label="More actions" onPress={() => openItemMenu(s, item, {origin, manager: isManager(session.role)})} />;
}

const styles = StyleSheet.create({
  primary: {flexDirection: 'row', flexWrap: 'wrap', gap: u(10), alignItems: 'center'},
  toggles: {flexDirection: 'row', justifyContent: 'space-between', marginTop: u(14), paddingHorizontal: u(4)},
  toggle: {alignItems: 'center', minWidth: 56, paddingVertical: 4},
  details: {},
  fact: {marginBottom: u(12)},
  mediaCard: {backgroundColor: 'rgba(16,33,45,0.7)', borderRadius: radius.md, padding: u(12), borderWidth: StyleSheet.hairlineWidth, borderColor: colors.lineSoft},
  chips: {flexDirection: 'row', flexWrap: 'wrap', gap: u(8), marginTop: u(12)},
  chip: {flexDirection: 'row', alignItems: 'center', gap: 4, height: u(32), paddingHorizontal: u(12), borderRadius: radius.pill, backgroundColor: 'rgba(237,245,250,0.08)', borderWidth: StyleSheet.hairlineWidth, borderColor: 'rgba(237,245,250,0.16)'},
});

export {ListRow};
