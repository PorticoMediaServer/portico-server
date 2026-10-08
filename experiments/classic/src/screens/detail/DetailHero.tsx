import React, {useState} from 'react';
import {View} from 'react-native';
import {useMetrics} from '../../theme/layout';
import {colors} from '../../theme/tokens';
import type {Img} from '../../data/types';
import {Artwork} from '../../ui/Artwork';
import {Scrim} from '../../ui/Canvas';
import {Text} from '../../ui/Text';
import {IconButton} from '../../ui/Button';
import {Focusable} from '../../ui/Focusable';
import {usePrefs} from '../../app/prefs';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {PHONE_HEADER} from '../../ui/Page';
import {back} from '../../nav/nav';

/**
 * Detail page header: backdrop with the canvas scrim, poster (or album art),
 * eyebrow, title, facts, synopsis and the action row.
 */
export function DetailHero({backdrop, poster, posterShape = 'poster', eyebrow, title, facts, synopsis, children, round}: {backdrop?: Img; poster?: Img; posterShape?: 'poster' | 'square' | 'wide'; eyebrow?: string; title: string; facts?: React.ReactNode; synopsis?: string; children?: React.ReactNode; round?: boolean}) {
  const m = useMetrics();
  const prefs = usePrefs();
  const insets = useSafeAreaInsets();
  const [w, setW] = useState(m.width);
  const [expanded, setExpanded] = useState(false);
  const bh = m.isTV ? Math.round(m.height * 0.8) : m.isPhone ? Math.round(m.width * 0.78) : Math.round(Math.min(m.height * 0.6, 560));
  const pw = posterShape === 'poster' ? (m.isTV ? 300 : m.isTablet ? 200 : 116) : posterShape === 'square' ? (m.isTV ? 380 : m.isTablet ? 240 : 150) : (m.isTV ? 420 : 280);
  const ph = posterShape === 'poster' ? pw * 1.5 : posterShape === 'square' ? pw : pw * 0.5625;
  const phone = m.isPhone;
  return (
    <View onLayout={e => setW(e.nativeEvent.layout.width)} style={{marginTop: m.isTV ? -48 : 0}}>
      <View style={{height: bh}}>
        {backdrop && prefs.showBackdrops ? <Artwork source={backdrop} title={title} width={w} height={bh} radius={0} /> : null}
        <Scrim width={w} height={bh} left={!phone} top={phone} strength={1.08} />
        {!m.isTV ? <View style={{position: 'absolute', left: m.gutter, top: phone ? insets.top + PHONE_HEADER + 6 : 20}}><IconButton icon="back" label="Back" variant="secondary" onPress={back} size={phone ? 36 : 40} /></View> : null}
      </View>
      <View style={{marginTop: phone ? -bh * 0.42 : -(bh * (m.isTV ? 0.72 : 0.62)), paddingHorizontal: m.gutter, flexDirection: phone ? 'column' : 'row', gap: phone ? m.space.md : m.space.xl, alignItems: phone ? 'flex-start' : 'flex-end'}}>
        {poster ? <Artwork source={poster} title={title} width={pw} height={ph} radius={round ? pw / 2 : m.radius.card} round={round} style={{shadowColor: '#000', shadowOpacity: 0.55, shadowRadius: 24, shadowOffset: {width: 0, height: 10}, borderWidth: 1, borderColor: 'rgba(237,245,250,0.08)'}} /> : null}
        <View style={{flex: phone ? undefined : 1, gap: m.isTV ? 12 : 8, maxWidth: m.isTV ? 1100 : 760, alignSelf: 'stretch'}}>
          {eyebrow ? <Text variant="label" tone="accent" numberOfLines={1}>{eyebrow}</Text> : null}
          <Text variant="display" numberOfLines={3}>{title}</Text>
          {facts}
          {synopsis ? (
            <Focusable onPress={() => setExpanded(e => !e)} accessibilityLabel={expanded ? 'Show less' : 'Read more'} style={({focused}) => ({borderRadius: 10, padding: m.isTV ? 8 : 0, marginHorizontal: m.isTV ? -8 : 0, backgroundColor: focused ? colors.raisedHigh : 'transparent'})}>
              <Text variant="body" tone="secondary" numberOfLines={expanded ? undefined : m.isTV ? 3 : 4}>{synopsis}</Text>
            </Focusable>
          ) : null}
          {children}
        </View>
      </View>
    </View>
  );
}

export function Facts({items, children}: {items: (string | undefined | false)[]; children?: React.ReactNode}) {
  const m = useMetrics();
  return (
    <View style={{flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', gap: m.isTV ? 14 : 8}}>
      {children}
      <Text variant="bodyStrong" tone="secondary">{items.filter(Boolean).join('  ·  ')}</Text>
    </View>
  );
}

/**
 * The row of title actions. Wide screens show them all; a phone keeps the
 * primary button wide and at most three icon actions before More, since the
 * More menu carries every action that does not fit.
 */
export function ActionRow({children}: {children: React.ReactNode}) {
  const m = useMetrics();
  const kids = React.Children.toArray(children).filter(React.isValidElement) as React.ReactElement<Record<string, unknown>>[];
  if (!m.isPhone) return <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: m.space.sm, marginTop: m.space.sm}}>{kids}</View>;
  const [primary, ...rest] = kids;
  const more = rest.length ? rest[rest.length - 1] : undefined;
  const middle = rest.slice(0, -1).slice(0, 3);
  return (
    <View style={{flexDirection: 'row', gap: 8, marginTop: m.space.sm, alignItems: 'center'}}>
      {primary ? React.cloneElement(primary, {style: {flex: 1}}) : null}
      {middle.map(c => React.cloneElement(c, {label: undefined}))}
      {more}
    </View>
  );
}
