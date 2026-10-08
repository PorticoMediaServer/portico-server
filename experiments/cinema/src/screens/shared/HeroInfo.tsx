import React from 'react';
import {StyleSheet, View} from 'react-native';
import type {Item} from '../../data/types';
import {Badge} from '../../design/Controls';
import {Icon} from '../../design/Icon';
import {Text} from '../../design/Text';
import {colors, isTV, u} from '../../design/tokens';
import {code, fmtRuntime} from './cards';
import {plural} from '../../design/format';

export function metaLine(item: Item, lookup?: (id: string) => Item | undefined): string[] {
  switch (item.kind) {
    case 'movie':
    case 'homevideo':
      return [String(item.year ?? ''), item.contentRating !== 'NR' ? item.contentRating : '', item.runtime ? fmtRuntime(item.runtime) : '', item.genres.slice(0, 2).join(', ')].filter(Boolean);
    case 'show':
      return [item.yearEnd ? (item.yearEnd === item.year ? String(item.year) : `${item.year}–${item.yearEnd}`) : `${item.year}–`, item.contentRating, `${item.seasonIds.filter(s => !s.endsWith('-s0')).length} ${item.seasonIds.length === 1 ? 'Season' : 'Seasons'}`, item.genres.slice(0, 2).join(', ')].filter(Boolean);
    case 'episode':
      return [lookup?.(item.showId)?.title ?? '', code(item.seasonNumber, item.number), fmtRuntime(item.runtime)].filter(Boolean);
    case 'album':
      return [item.artistName, String(item.year), `${plural(item.trackIds.length, 'song', 'songs')}`];
    case 'audiobook':
      return [item.author, `${Math.round(item.duration / 3600)} hr`, item.genres[0] ?? ''].filter(Boolean);
    default:
      return [String(item.year ?? '')];
  }
}

/** Title block over a backdrop: eyebrow, display title, facts, synopsis. */
export function HeroInfo({item, eyebrow, lookup, children, compact, maxWidth}: {item: Item; eyebrow?: string; lookup?: (id: string) => Item | undefined; children?: React.ReactNode; compact?: boolean; maxWidth?: number}) {
  const meta = metaLine(item, lookup);
  const has4k = 'versions' in item && item.versions.some(v => v.resolution === '4K');
  const hdr = 'versions' in item && item.versions.find(v => v.hdr)?.hdr;
  const score = 'criticScore' in item ? item.criticScore : undefined;
  const title = item.kind === 'episode' ? item.title : item.title;
  const long = title.length > 22;
  return (
    <View style={[{maxWidth: maxWidth ?? (isTV ? 1000 : undefined)}]}>
      {eyebrow ? <Text variant="eyebrow" color="rgba(237,245,250,0.82)" style={[{marginBottom: u(6)}, styles.shadow]}>{eyebrow}</Text> : null}
      <Text variant={compact ? 'display' : 'hero'} numberOfLines={2} style={[long && !compact && {fontSize: isTV ? 70 : 32, lineHeight: isTV ? 76 : 36}, styles.shadow]}>{title}</Text>
      <View style={styles.meta}>
        {score ? (
          <View style={styles.score}>
            <Icon name="award" size={13} color={colors.amber} />
            <Text variant="captionStrong" tone="amber" style={{marginLeft: 4}}>{score}%</Text>
          </View>
        ) : null}
        {meta.map((m, i) => (
          <Text key={i} variant="captionStrong" tone="secondary">{i > 0 || score ? '·  ' : ''}{m}</Text>
        ))}
        {has4k ? <Badge label="4K" tone="neutral" /> : null}
        {hdr ? <Badge label={hdr === 'Dolby Vision' ? 'Dolby Vision' : 'HDR'} tone="neutral" /> : null}
      </View>
      {item.synopsis && !compact ? <Text variant="body" tone="secondary" numberOfLines={isTV ? 3 : 3} style={[{marginTop: u(8), maxWidth: isTV ? 900 : undefined}, styles.shadow]}>{item.synopsis}</Text> : null}
      {children}
    </View>
  );
}

const styles = StyleSheet.create({
  meta: {flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', gap: u(6), marginTop: u(8)},
  score: {flexDirection: 'row', alignItems: 'center'},
  shadow: {textShadowColor: 'rgba(0,0,0,0.6)', textShadowRadius: 12, textShadowOffset: {width: 0, height: 2}},
});
