import React, {useMemo} from 'react';
import {KeyboardAvoidingView, Platform, ScrollView, TVFocusGuideView, View} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import Svg, {Defs, LinearGradient, RadialGradient, Rect, Stop} from 'react-native-svg';
import {colors} from '../../theme/tokens';
import {useMetrics} from '../../theme/layout';
import {Artwork} from '../../ui/Artwork';
import {Text} from '../../ui/Text';
import {Wordmark} from '../../shell/Brand';
import {catalog, posterFor} from '../../data/fixtures/catalog';
import {IconButton} from '../../ui/Button';

/** Posters for the landing wall (public-domain and open-movie titles first). */
function useWall(count: number) {
  return useMemo(() => catalog.movies.filter(m => m.real).concat(catalog.movies.filter(m => !m.real)).slice(0, count).map(m => ({id: m.id, img: posterFor(m.id), title: m.title})), [count]);
}

/**
 * The frame for everything before the library: a slowly lit poster wall
 * under the blue glow, and a centred panel.
 */
export function AuthFrame({children, title, lede, onBack, wide, wall = true}: {children: React.ReactNode; title?: string; lede?: string; onBack?: () => void; wide?: boolean; wall?: boolean}) {
  const m = useMetrics();
  const insets = useSafeAreaInsets();
  const cols = m.isTV ? 10 : m.isPhone ? 4 : 7;
  const pw = Math.ceil(m.width / cols);
  const rows = Math.ceil(m.height / (pw * 1.5)) + 1;
  const posters = useWall(cols * rows);
  const panelW = wide ? (m.isTV ? 1300 : 760) : m.isTV ? 900 : 480;
  const body = (
    <ScrollView contentContainerStyle={{flexGrow: 1, alignItems: 'center', justifyContent: 'center', paddingTop: insets.top + 24, paddingBottom: insets.bottom + 24, paddingHorizontal: 16}} keyboardShouldPersistTaps="handled">
      <View style={{width: '100%', maxWidth: panelW, backgroundColor: 'rgba(19,38,52,0.94)', borderRadius: m.radius.sheet, borderWidth: 1, borderColor: colors.line, padding: m.isTV ? 56 : 24, gap: m.isTV ? 28 : 16}}>
        <View style={{flexDirection: 'row', alignItems: 'center', gap: 12}}>
          {onBack && !m.isTV ? <IconButton icon="back" label="Back" onPress={onBack} size={36} variant="secondary" /> : null}
          <Wordmark height={m.isTV ? 44 : 28} />
        </View>
        {title ? <View style={{gap: 6}}><Text variant="title">{title}</Text>{lede ? <Text variant="body" tone="secondary">{lede}</Text> : null}</View> : null}
        {children}
      </View>
    </ScrollView>
  );
  return (
    <View style={{flex: 1, backgroundColor: colors.ink}}>
      {wall ? (
        <View style={{position: 'absolute', left: 0, right: 0, top: 0, bottom: 0, flexDirection: 'row', flexWrap: 'wrap', opacity: 0.32, transform: [{rotate: '-4deg'}, {scale: 1.12}]}}>
          {posters.map(p => <Artwork key={p.id} source={p.img} title={p.title} width={pw} height={pw * 1.5} radius={0} fade={false} />)}
        </View>
      ) : null}
      <Svg width={m.width} height={m.height} style={{position: 'absolute'}} pointerEvents="none">
        <Defs>
          <LinearGradient id="af" x1="0" y1="0" x2="0" y2="1"><Stop offset="0" stopColor={colors.ink} stopOpacity={0.55} /><Stop offset="1" stopColor={colors.ink} stopOpacity={0.95} /></LinearGradient>
          <RadialGradient id="ag" cx="14%" cy="0%" rx="80%" ry="70%"><Stop offset="0" stopColor={colors.raisedBright} stopOpacity={0.9} /><Stop offset="0.4" stopColor={colors.ambient} stopOpacity={0.5} /><Stop offset="1" stopColor={colors.ink} stopOpacity={0} /></RadialGradient>
        </Defs>
        <Rect x={0} y={0} width={m.width} height={m.height} fill="url(#af)" />
        <Rect x={0} y={0} width={m.width} height={m.height} fill="url(#ag)" />
      </Svg>
      <KeyboardAvoidingView style={{flex: 1}} behavior={Platform.OS === 'ios' && !m.isTV ? 'padding' : undefined}>
        {m.isTV ? <TVFocusGuideView autoFocus style={{flex: 1}}>{body}</TVFocusGuideView> : body}
      </KeyboardAvoidingView>
    </View>
  );
}
