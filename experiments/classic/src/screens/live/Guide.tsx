import React, {useEffect, useMemo, useRef, useState} from 'react';
import {ScrollView, View} from 'react-native';
import type {Channel, Program} from '../../data/types';
import {Focusable} from '../../ui/Focusable';
import {Artwork} from '../../ui/Artwork';
import {Text} from '../../ui/Text';
import {Icon} from '../../ui/Icon';
import {colors} from '../../theme/tokens';
import {useMetrics} from '../../theme/layout';

const HALF = 30 * 60000;
const time = (t: number) => new Date(t).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'});

/**
 * The program guide grid: channels down the side, half-hour columns across,
 * a now line, and a cell per program. Works with touch (two-axis scroll) and
 * with the remote (the focus engine moves between cells).
 */
export function GuideGrid({channels, guide, from, to, onProgram, onChannel, height}: {channels: Channel[]; guide: Record<string, Program[]>; from: number; to: number; onProgram: (p: Program, c: Channel) => void; onChannel: (c: Channel) => void; height?: number}) {
  const m = useMetrics();
  const perMin = m.isTV ? 9 : m.isTablet ? 6 : 4.2;
  const rowH = m.isTV ? 96 : m.isPhone ? 66 : 74;
  const chanW = m.isTV ? 240 : m.isPhone ? 92 : 170;
  const width = ((to - from) / 60000) * perMin;
  const header = useRef<ScrollView>(null);
  const [now, setNow] = useState(Date.now());
  useEffect(() => { const t = setInterval(() => setNow(Date.now()), 30000); return () => clearInterval(t); }, []);
  const slots = useMemo(() => { const out: number[] = []; for (let t = from; t < to; t += HALF) out.push(t); return out; }, [from, to]);
  const nowX = ((now - from) / 60000) * perMin;
  return (
    <View style={{gap: 6}}>
      <View style={{flexDirection: 'row'}}>
        <View style={{width: chanW}} />
        <ScrollView ref={header} horizontal scrollEnabled={false} showsHorizontalScrollIndicator={false} contentOffset={{x: Math.max(0, nowX - perMin * 20), y: 0}}>
          <View style={{width, height: m.isTV ? 40 : 28, flexDirection: 'row'}}>
            {slots.map(s => <View key={s} style={{width: perMin * 30, paddingLeft: 8, justifyContent: 'center', borderLeftWidth: 1, borderColor: colors.lineSoft}}><Text variant="captionStrong" tone="tertiary">{time(s)}</Text></View>)}
          </View>
        </ScrollView>
      </View>
      <View style={{flexDirection: 'row', height}}>
        <ScrollView nestedScrollEnabled showsVerticalScrollIndicator={false} contentContainerStyle={{flexDirection: 'row'}} style={{flex: 1}}>
          <View style={{width: chanW, gap: 4}}>
            {channels.map(c => (
              <Focusable key={c.id} onPress={() => onChannel(c)} accessibilityLabel={`Channel ${c.number}, ${c.name}`}
                style={({focused}) => ({height: rowH, flexDirection: m.isPhone ? 'column' : 'row', alignItems: 'center', justifyContent: 'center', gap: m.isPhone ? 2 : 10, paddingHorizontal: 8, borderRadius: m.radius.small, backgroundColor: focused ? colors.focus : colors.raisedSoft, marginRight: 4})}>
                {({focused}) => (
                  <>
                    <Artwork source={c.logo} title={c.name} width={m.isTV ? 104 : m.isPhone ? 64 : 86} height={m.isTV ? 58 : m.isPhone ? 36 : 48} radius={4} resizeMode="contain" style={{backgroundColor: 'transparent'}} />
                    <View style={{alignItems: m.isPhone ? 'center' : 'flex-start', flex: m.isPhone ? undefined : 1}}>
                      <Text variant="micro" color={focused ? colors.accentInk : colors.textTertiary}>{c.number}{c.favorite ? ' ★' : ''}</Text>
                      {!m.isPhone ? <Text variant="micro" color={focused ? colors.accentInk : colors.textSecondary} numberOfLines={1}>{c.name}</Text> : null}
                    </View>
                  </>
                )}
              </Focusable>
            ))}
          </View>
          <ScrollView horizontal nestedScrollEnabled showsHorizontalScrollIndicator={false} scrollEventThrottle={16} onScroll={e => header.current?.scrollTo({x: e.nativeEvent.contentOffset.x, animated: false})} contentOffset={{x: Math.max(0, nowX - perMin * 20), y: 0}}>
            <View style={{width, gap: 4}}>
              {channels.map(c => (
                <View key={c.id} style={{height: rowH, width}}>
                  {(guide[c.id] ?? []).map(p => {
                    const x = Math.max(0, ((p.start - from) / 60000) * perMin);
                    const w = Math.min(width, ((p.end - from) / 60000) * perMin) - x - 4;
                    if (w <= 8) return null;
                    const onNow = p.start <= now && p.end > now;
                    return (
                      <Focusable key={p.id} onPress={() => onProgram(p, c)} accessibilityLabel={`${p.title}, ${time(p.start)} to ${time(p.end)}`}
                        style={({focused}) => ({position: 'absolute', left: x, width: w, top: 0, bottom: 0, borderRadius: m.radius.small, paddingHorizontal: m.isTV ? 14 : 8, justifyContent: 'center', backgroundColor: focused ? colors.focus : onNow ? colors.raisedHigh : colors.raised, borderWidth: 1, borderColor: focused ? colors.focus : onNow ? colors.lineStrong : colors.lineSoft, overflow: 'hidden'})}>
                        {({focused}) => (
                          <>
                            <View style={{flexDirection: 'row', alignItems: 'center', gap: 6}}>
                              {p.recording ? <Icon name="record" size={m.isTV ? 16 : 11} color={colors.record} /> : null}
                              {p.reminder ? <Icon name="bell" size={m.isTV ? 16 : 11} color={focused ? colors.accentInk : colors.amber} /> : null}
                              <Text variant={m.isTV ? 'bodyStrong' : 'captionStrong'} numberOfLines={1} color={focused ? colors.accentInk : colors.text} style={{flexShrink: 1}}>{p.title}</Text>
                            </View>
                            {w > 80 ? <Text variant="micro" numberOfLines={1} color={focused ? colors.accentInk : colors.textTertiary}>{[p.isNew ? 'New' : undefined, p.isLive ? 'Live' : undefined, p.episodeTitle, `${time(p.start)} – ${time(p.end)}`].filter(Boolean).join(' · ')}</Text> : null}
                          </>
                        )}
                      </Focusable>
                    );
                  })}
                </View>
              ))}
              {nowX > 0 && nowX < width ? <View pointerEvents="none" style={{position: 'absolute', left: nowX, top: 0, bottom: 0, width: 2, backgroundColor: colors.record}} /> : null}
            </View>
          </ScrollView>
        </ScrollView>
      </View>
    </View>
  );
}
