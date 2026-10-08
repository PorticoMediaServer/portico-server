import React, {useEffect} from 'react';
import {ScrollView, StyleSheet, TVFocusGuideView, View, useWindowDimensions} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import type {ArtRef} from '../../data/types';
import {Backdrop} from '../../design/Backdrop';
import {glow} from '../../design/Glow';
import {ErrorState, Skeleton} from '../../design/States';
import {isTV, u} from '../../design/tokens';
import {useLayout} from '../../design/useLayout';
import {Header, useChromeInsets} from '../../nav/Screen';

/**
 * Immersive title page scaffold: full-bleed artwork behind everything, the identity block
 * anchored low on the first screen, and sections that scroll up over the dimming artwork.
 */
export function Stage({art, tint, children, below, loading, error, onRetry, title}: {art?: ArtRef; tint?: string; children?: React.ReactNode; below?: React.ReactNode; loading?: boolean; error?: Error | null; onRetry?: () => void; title?: string}) {
  const {height} = useWindowDimensions();
  const layout = useLayout();
  const chrome = useChromeInsets();
  const insets = useSafeAreaInsets();
  useEffect(() => {
    if (tint) glow.setTint(tint);
  }, [tint]);
  const stageH = isTV ? height - 40 : layout.compact ? Math.round(layout.width * 1.08) : Math.round(height * 0.62);
  if (error) return <View style={{flex: 1, justifyContent: 'center', paddingLeft: chrome.left}}><ErrorState onRetry={onRetry} /></View>;
  return (
    <View style={{flex: 1, paddingLeft: chrome.left}}>
      <View style={[StyleSheet.absoluteFill, {left: isTV ? 0 : chrome.left, height: isTV ? height : stageH + 120}]}>
        <Backdrop art={art} left={!layout.compact} bottom={1} top={layout.compact ? 0.55 : 0.3} />
      </View>
      <TVFocusGuideView autoFocus style={{flex: 1}}>
        <ScrollView contentContainerStyle={{paddingBottom: chrome.bottom + u(50)}} showsVerticalScrollIndicator={false}>
          <View style={{minHeight: layout.compact ? undefined : stageH, paddingTop: layout.compact ? stageH - u(150) : insets.top + (isTV ? 0 : 60), justifyContent: 'flex-end', paddingHorizontal: layout.gutter, paddingBottom: isTV ? 80 : u(10)}}>
            {loading ? (
              <View>
                <Skeleton width={u(280)} height={u(44)} />
                <Skeleton width={u(200)} height={u(16)} style={{marginTop: u(12)}} />
                <Skeleton width="90%" height={u(60)} style={{marginTop: u(12)}} />
                <Skeleton width={u(160)} height={u(48)} radius={99} style={{marginTop: u(16)}} />
              </View>
            ) : (
              <View style={{maxWidth: isTV ? 1500 : 760}}>{children}</View>
            )}
          </View>
          {!loading ? below : null}
        </ScrollView>
      </TVFocusGuideView>
      <View style={[StyleSheet.absoluteFill, {bottom: undefined, left: chrome.left}]} pointerEvents="box-none">
        <Header transparent title={title} />
      </View>
    </View>
  );
}
