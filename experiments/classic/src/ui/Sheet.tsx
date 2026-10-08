import React, {useEffect, useRef, useState} from 'react';
import {Animated, Easing, KeyboardAvoidingView, Modal, Platform, Pressable, ScrollView, TVFocusGuideView, View, useWindowDimensions, type StyleProp, type ViewStyle} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {colors} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import {Text} from './Text';
import {Button, IconButton} from './Button';
import {Icon, type IconName} from './Icon';
import {Focusable} from './Focusable';
import {FocusGate} from './focusGate';
import {TextField} from './Controls';
import type {Img} from '../data/types';
import {Artwork} from './Artwork';

type SheetProps = {
  visible: boolean;
  onClose: () => void;
  title?: string;
  subtitle?: string;
  children?: React.ReactNode;
  footer?: React.ReactNode;
  size?: 'sm' | 'md' | 'lg';
  header?: React.ReactNode;
  scroll?: boolean;
  side?: boolean;
  contentStyle?: StyleProp<ViewStyle>;
};

/**
 * Sheets and dialogs. A bottom sheet on iPhone, a centred panel on iPad and
 * TV (or a side panel for player options). Focus is trapped inside on TV and
 * Menu closes it.
 */
export function Sheet({visible, onClose, title, subtitle, children, footer, size = 'md', header, scroll = true, side, contentStyle}: SheetProps) {
  const m = useMetrics();
  const insets = useSafeAreaInsets();
  const {height} = useWindowDimensions();
  const [mounted, setMounted] = useState(visible);
  const t = useRef(new Animated.Value(0)).current;
  useEffect(() => {
    if (visible) setMounted(true);
    Animated.timing(t, {toValue: visible ? 1 : 0, duration: visible ? 260 : 180, easing: Easing.out(Easing.cubic), useNativeDriver: true}).start(({finished}) => {
      if (finished && !visible) setMounted(false);
    });
  }, [visible, t]);
  if (!mounted) return null;
  const bottom = m.isPhone && !side;
  const widths = {sm: m.isTV ? 760 : 440, md: m.isTV ? 980 : 560, lg: m.isTV ? 1320 : 760};
  const panelStyle: StyleProp<ViewStyle> = side
    ? {position: 'absolute', right: 0, top: 0, bottom: 0, width: m.isTV ? 640 : Math.min(420, m.width), backgroundColor: colors.overlay, borderLeftWidth: 1, borderColor: colors.line, paddingTop: m.isTV ? 56 : insets.top + 12}
    : bottom
      ? {position: 'absolute', left: 0, right: 0, bottom: 0, maxHeight: height * 0.9, backgroundColor: colors.overlay, borderTopLeftRadius: m.radius.sheet, borderTopRightRadius: m.radius.sheet, paddingBottom: Math.max(insets.bottom, 12), borderWidth: 1, borderColor: colors.line}
      : {width: Math.min(widths[size], m.width - 32), maxHeight: height * (m.isTV ? 0.86 : 0.88), backgroundColor: colors.overlay, borderRadius: m.radius.sheet, borderWidth: 1, borderColor: colors.line, overflow: 'hidden'};
  const translate = side
    ? {transform: [{translateX: t.interpolate({inputRange: [0, 1], outputRange: [80, 0]})}]}
    : bottom
      ? {transform: [{translateY: t.interpolate({inputRange: [0, 1], outputRange: [height * 0.4, 0]})}]}
      : {transform: [{scale: t.interpolate({inputRange: [0, 1], outputRange: [0.96, 1]})}]};
  const body = (
    <>
      {bottom ? <View style={{alignSelf: 'center', width: 40, height: 5, borderRadius: 3, backgroundColor: colors.lineStrong, marginTop: 8}} /> : null}
      {header}
      {title ? (
        <View style={{flexDirection: 'row', alignItems: 'flex-start', gap: m.space.md, paddingHorizontal: m.space.lg, paddingTop: m.space.lg, paddingBottom: m.space.sm}}>
          <View style={{flex: 1, gap: 4}}>
            <Text variant="heading">{title}</Text>
            {subtitle ? <Text variant="body" tone="secondary">{subtitle}</Text> : null}
          </View>
          {m.isTV ? null : <IconButton icon="close" label="Close" onPress={onClose} size={34} variant="secondary" />}
        </View>
      ) : null}
      {scroll ? (
        <ScrollView style={{flexGrow: 0}} contentContainerStyle={[{paddingHorizontal: m.space.lg, paddingBottom: m.space.lg, paddingTop: title ? m.space.xs : m.space.lg, gap: m.space.md}, contentStyle]} keyboardShouldPersistTaps="handled">
          {children}
        </ScrollView>
      ) : <View style={[{paddingHorizontal: m.space.lg, paddingBottom: m.space.lg, gap: m.space.md, flexShrink: 1}, contentStyle]}>{children}</View>}
      {footer ? <View style={{flexDirection: 'row', flexWrap: 'wrap', justifyContent: 'flex-end', gap: m.space.sm, paddingHorizontal: m.space.lg, paddingVertical: m.space.md, borderTopWidth: 1, borderColor: colors.lineSoft}}>{footer}</View> : null}
    </>
  );
  return (
    <Modal visible transparent animationType="none" onRequestClose={onClose} supportedOrientations={['portrait', 'landscape']}>
      <FocusGate>
      <KeyboardAvoidingView behavior={Platform.OS === 'ios' && !m.isTV ? 'padding' : undefined} style={{flex: 1}}>
        <Animated.View style={{flex: 1, backgroundColor: 'rgba(3,8,12,0.72)', opacity: t, alignItems: 'center', justifyContent: bottom ? 'flex-end' : 'center'}}>
          {m.isTV ? null : <Pressable accessibilityLabel="Close" style={{position: 'absolute', left: 0, right: 0, top: 0, bottom: 0}} onPress={onClose} />}
          <Animated.View style={[panelStyle, translate]}>
            {m.isTV ? <TVFocusGuideView autoFocus trapFocusUp trapFocusDown trapFocusLeft trapFocusRight style={{flexShrink: 1}}>{body}</TVFocusGuideView> : body}
          </Animated.View>
        </Animated.View>
      </KeyboardAvoidingView>
      </FocusGate>
    </Modal>
  );
}

export type MenuItem = {id: string; label: string; icon?: IconName; subtitle?: string; destructive?: boolean; disabled?: boolean; selected?: boolean; separatorBefore?: boolean; hidden?: boolean};

/** An action menu (long-press menus, More buttons, profile menu). */
export function ActionMenu({visible, onClose, title, subtitle, image, items, onSelect, footer, children}: {visible: boolean; onClose: () => void; title?: string; subtitle?: string; image?: Img; items: MenuItem[]; onSelect: (id: string) => void; footer?: React.ReactNode; children?: React.ReactNode}) {
  const m = useMetrics();
  const shown = items.filter(i => !i.hidden);
  const header = title ? (
    <View style={{flexDirection: 'row', gap: m.space.md, alignItems: 'center', padding: m.space.lg, paddingBottom: m.space.sm}}>
      {image ? <Artwork source={image} title={title} width={m.isTV ? 72 : 48} height={m.isTV ? 108 : 72} radius={6} /> : null}
      <View style={{flex: 1}}>
        <Text variant="subheading" numberOfLines={2}>{title}</Text>
        {subtitle ? <Text variant="caption" tone="tertiary" numberOfLines={1}>{subtitle}</Text> : null}
      </View>
    </View>
  ) : undefined;
  return (
    <Sheet visible={visible} onClose={onClose} header={header} size="sm" contentStyle={{gap: 0, paddingHorizontal: m.isTV ? 16 : 8}}>
      {children}
      {shown.map((it, i) => (
        <View key={it.id}>
          {it.separatorBefore && i > 0 ? <View style={{height: 1, backgroundColor: colors.lineSoft, marginVertical: 6, marginHorizontal: 12}} /> : null}
          <Focusable onPress={() => onSelect(it.id)} disabled={it.disabled} hasTVPreferredFocus={i === 0} accessibilityRole="menuitem" accessibilityLabel={it.label}
            style={({focused}) => ({flexDirection: 'row', alignItems: 'center', gap: m.space.md, paddingVertical: m.isTV ? 16 : 12, paddingHorizontal: m.isTV ? 20 : 12, borderRadius: m.radius.control, backgroundColor: focused ? colors.focus : 'transparent'})}>
            {({focused}) => {
              const c = focused ? colors.accentInk : it.destructive ? colors.danger : colors.text;
              return (
                <>
                  {it.icon ? <Icon name={it.icon} size={m.icon} color={c} /> : null}
                  <View style={{flex: 1}}>
                    <Text variant="body" color={c}>{it.label}</Text>
                    {it.subtitle ? <Text variant="caption" color={focused ? colors.accentInk : colors.textTertiary}>{it.subtitle}</Text> : null}
                  </View>
                  {it.selected ? <Icon name="check" size={m.icon} color={focused ? colors.accentInk : colors.accentStrong} /> : null}
                </>
              );
            }}
          </Focusable>
        </View>
      ))}
      {footer}
    </Sheet>
  );
}

/** A yes/no confirmation, optionally asking the viewer to type a word first. */
export function ConfirmDialog({visible, onClose, title, body, confirmLabel, destructive, onConfirm, typeToConfirm, busy, error, children}: {visible: boolean; onClose: () => void; title: string; body?: string; confirmLabel: string; destructive?: boolean; onConfirm: () => void; typeToConfirm?: string; busy?: boolean; error?: string; children?: React.ReactNode}) {
  const [typed, setTyped] = useState('');
  useEffect(() => { if (!visible) setTyped(''); }, [visible]);
  const blocked = !!typeToConfirm && typed.trim() !== typeToConfirm;
  return (
    <Sheet visible={visible} onClose={onClose} title={title} subtitle={body} size="sm" footer={<><Button label="Cancel" variant="ghost" onPress={onClose} /><Button label={confirmLabel} variant={destructive ? 'danger' : 'primary'} onPress={onConfirm} disabled={blocked} loading={busy} hasTVPreferredFocus={!typeToConfirm} /></>}>
      {children}
      {typeToConfirm ? <TextField label={`Type ${typeToConfirm} to confirm`} value={typed} onChangeText={setTyped} autoCapitalize="none" autoCorrect={false} /> : null}
      {error ? <Text variant="caption" tone="danger">{error}</Text> : null}
    </Sheet>
  );
}

/** Choose one option from a list. */
export function ChoiceSheet<T extends string | number>({visible, onClose, title, subtitle, options, value, onChange}: {visible: boolean; onClose: () => void; title: string; subtitle?: string; options: {value: T; label: string; subtitle?: string; icon?: IconName; disabled?: boolean}[]; value?: T; onChange: (v: T) => void}) {
  return <ActionMenu visible={visible} onClose={onClose} title={title} subtitle={subtitle} items={options.map(o => ({id: String(o.value), label: o.label, subtitle: o.subtitle, icon: o.icon, selected: o.value === value, disabled: o.disabled}))} onSelect={id => { const o = options.find(x => String(x.value) === id); if (o) onChange(o.value); onClose(); }} />;
}
