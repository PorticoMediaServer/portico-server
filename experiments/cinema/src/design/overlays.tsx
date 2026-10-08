import React, {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {Animated, Easing, KeyboardAvoidingView, Pressable, ScrollView, StyleSheet, TVFocusGuideView, View, type View as RNView} from 'react-native';
import {useSafeAreaInsets} from 'react-native-safe-area-context';
import {back} from '../nav/back';
import {Button, IconButton} from './Button';
import {TextField} from './Controls';
import {Focusable} from './Focusable';
import {focusMemory} from './focusMemory';
import {Icon, type IconName} from './Icon';
import {Text} from './Text';
import {colors, isTV, radius, u} from './tokens';
import {useLayout} from './useLayout';

/**
 * Overlays: sheets, action menus, confirmations and toasts, rendered above everything by one host.
 * TV sheets slide in from the right and trap focus; phones get bottom sheets. Back/Menu closes the
 * top overlay, then focus returns to whatever opened it.
 */

export type MenuAction = {label: string; icon?: IconName; onPress: () => void; destructive?: boolean; disabled?: boolean; detail?: string; checked?: boolean};
type SheetSpec = {kind: 'sheet'; title?: string; subtitle?: string; render: (close: () => void) => React.ReactNode; wide?: boolean; footer?: (close: () => void) => React.ReactNode};
type MenuSpec = {kind: 'menu'; title: string; subtitle?: string; actions: MenuAction[]};
type ConfirmSpec = {kind: 'confirm'; title: string; body?: string; confirmLabel: string; cancelLabel?: string; destructive?: boolean; typeToConfirm?: string; onConfirm: () => void; onCancel?: () => void};
type PromptSpec = {kind: 'prompt'; title: string; body?: string; fields: {key: string; label: string; initial?: string; placeholder?: string; secure?: boolean; help?: string; multiline?: boolean}[]; submitLabel: string; onSubmit: (values: Record<string, string>) => void | string | null};
type Spec = SheetSpec | MenuSpec | ConfirmSpec | PromptSpec;
type Entry = {id: number; spec: Spec; opener: RNView | null; closing?: boolean};
type Toast = {id: number; message: string; action?: {label: string; onPress: () => void}; tone?: 'info' | 'error'};

let entries: Entry[] = [];
let toasts: Toast[] = [];
let seq = 1;
const listeners = new Set<() => void>();
const emit = () => listeners.forEach(l => l());
const subscribe = (l: () => void) => {
  listeners.add(l);
  return () => listeners.delete(l);
};

function push(spec: Spec) {
  const entry = {id: seq++, spec, opener: isTV ? focusMemory.take() : null};
  entries = [...entries, entry];
  emit();
  back.sync();
  return entry.id;
}
const live = () => entries.filter(e => !e.closing);
function close(id?: number) {
  const open = live();
  const target = id === undefined ? open[open.length - 1] : open.find(e => e.id === id);
  if (!target) return;
  // Mark it closing so the frame can animate out; it is removed when the animation ends.
  entries = entries.map(e => (e === target ? {...e, closing: true} : e));
  emit();
  back.sync();
}
function finish(id: number) {
  const target = entries.find(e => e.id === id);
  if (!target) return;
  entries = entries.filter(e => e.id !== id);
  emit();
  if (isTV) focusMemory.restore(target.opener);
}

export const ui = {
  sheet: (s: Omit<SheetSpec, 'kind'>) => push({kind: 'sheet', ...s}),
  menu: (s: Omit<MenuSpec, 'kind'>) => push({kind: 'menu', ...s}),
  confirm: (s: Omit<ConfirmSpec, 'kind'>) => push({kind: 'confirm', ...s}),
  prompt: (s: Omit<PromptSpec, 'kind'>) => push({kind: 'prompt', ...s}),
  close,
  closeAll() {
    entries = [];
    emit();
    back.sync();
  },
  toast(message: string, opts?: {action?: {label: string; onPress: () => void}; tone?: 'info' | 'error'}) {
    const t = {id: seq++, message, ...opts};
    toasts = [...toasts.slice(-2), t];
    emit();
    setTimeout(() => {
      toasts = toasts.filter(x => x !== t);
      emit();
    }, opts?.action ? 6000 : 3600);
  },
  isOpen: () => live().length > 0,
};

back.register({active: () => live().length > 0, handle: () => close(), priority: 100});

function useEntries() {
  const e = useSyncExternalStore(subscribe, () => entries);
  const t = useSyncExternalStore(subscribe, () => toasts);
  return {entries: e, toasts: t};
}

export function OverlayHost() {
  const {entries: list, toasts: tl} = useEntries();
  return (
    <>
      {list.map(e => (
        <OverlayFrame key={e.id} entry={e} top={!e.closing && e === list.filter(x => !x.closing).slice(-1)[0]} />
      ))}
      <ToastStack toasts={tl} />
    </>
  );
}

function OverlayFrame({entry, top}: {entry: Entry; top: boolean}) {
  const layout = useLayout();
  const insets = useSafeAreaInsets();
  const anim = useRef(new Animated.Value(0)).current;
  useEffect(() => {
    Animated.timing(anim, {toValue: 1, duration: 280, easing: Easing.out(Easing.cubic), useNativeDriver: true}).start();
  }, [anim]);
  useEffect(() => {
    if (!entry.closing) return;
    Animated.timing(anim, {toValue: 0, duration: 190, easing: Easing.in(Easing.cubic), useNativeDriver: true}).start(() => finish(entry.id));
  }, [entry.closing, entry.id, anim]);
  const dismiss = () => close(entry.id);
  const spec = entry.spec;
  const centered = spec.kind === 'confirm' || spec.kind === 'prompt';
  const side = isTV && !centered;
  const panelWidth = side ? (spec.kind === 'sheet' && spec.wide ? 1100 : 780) : Math.min(layout.width, centered ? 520 : 680);
  const translate = side
    ? anim.interpolate({inputRange: [0, 1], outputRange: [panelWidth, 0]})
    : centered
      ? anim.interpolate({inputRange: [0, 1], outputRange: [24, 0]})
      : anim.interpolate({inputRange: [0, 1], outputRange: [layout.height * 0.6, 0]});
  return (
    <View style={StyleSheet.absoluteFill} pointerEvents={top ? 'auto' : 'none'}>
      <Animated.View style={[StyleSheet.absoluteFill, {backgroundColor: 'rgba(2,6,10,0.66)', opacity: anim}]}>
        {!isTV ? <Pressable style={StyleSheet.absoluteFill} onPress={dismiss} accessibilityLabel="Close" /> : null}
      </Animated.View>
      <KeyboardAvoidingView behavior="padding" enabled={!isTV} style={[StyleSheet.absoluteFill, side ? {alignItems: 'flex-end'} : centered ? {alignItems: 'center', justifyContent: 'center', padding: 20} : {justifyContent: 'flex-end', alignItems: 'center'}]} pointerEvents="box-none">
        <Animated.View
          style={[
            styles.panel,
            {width: panelWidth},
            side && {height: '100%', borderTopLeftRadius: radius.xl, borderBottomLeftRadius: radius.xl, paddingTop: 60, paddingBottom: 40},
            centered && {borderRadius: radius.xl, maxHeight: layout.height * 0.86},
            !side && !centered && {borderTopLeftRadius: radius.xl, borderTopRightRadius: radius.xl, maxHeight: layout.height * 0.9, paddingBottom: Math.max(insets.bottom, 16)},
            {opacity: centered ? anim : 1, transform: side ? [{translateX: translate}] : [{translateY: translate}]},
          ]}>
          <TVFocusGuideView autoFocus trapFocusLeft={isTV} trapFocusRight={isTV} trapFocusUp={isTV} trapFocusDown={isTV} style={{flexShrink: 1}}>
            {!isTV && !centered ? <View style={styles.grabber} /> : null}
            {spec.kind === 'sheet' ? <SheetBody spec={spec} onClose={dismiss} /> : null}
            {spec.kind === 'menu' ? <MenuBody spec={spec} onClose={dismiss} /> : null}
            {spec.kind === 'confirm' ? <ConfirmBody spec={spec} onClose={dismiss} /> : null}
            {spec.kind === 'prompt' ? <PromptBody spec={spec} onClose={dismiss} /> : null}
          </TVFocusGuideView>
        </Animated.View>
      </KeyboardAvoidingView>
    </View>
  );
}

function Header({title, subtitle, onClose}: {title?: string; subtitle?: string; onClose: () => void}) {
  if (!title) return null;
  return (
    <View style={styles.header}>
      <View style={{flex: 1}}>
        <Text variant={isTV ? 'title' : 'heading'} numberOfLines={2}>{title}</Text>
        {subtitle ? <Text variant="caption" tone="tertiary" numberOfLines={2} style={{marginTop: 2}}>{subtitle}</Text> : null}
      </View>
      {!isTV ? <IconButton icon="x" label="Close" onPress={onClose} size={34} /> : null}
    </View>
  );
}

function SheetBody({spec, onClose}: {spec: SheetSpec; onClose: () => void}) {
  return (
    <>
      <Header title={spec.title} subtitle={spec.subtitle} onClose={onClose} />
      <ScrollView style={{flexShrink: 1}} contentContainerStyle={{paddingHorizontal: u(16), paddingBottom: u(16)}} keyboardShouldPersistTaps="handled">
        {spec.render(onClose)}
      </ScrollView>
      {spec.footer ? <View style={{paddingHorizontal: u(16), paddingTop: u(8)}}>{spec.footer(onClose)}</View> : null}
    </>
  );
}

function MenuBody({spec, onClose}: {spec: MenuSpec; onClose: () => void}) {
  return (
    <>
      <Header title={spec.title} subtitle={spec.subtitle} onClose={onClose} />
      <ScrollView style={{flexShrink: 1}} contentContainerStyle={{paddingHorizontal: u(10), paddingBottom: u(10)}}>
        {spec.actions.map((a, i) => (
          <Focusable
            key={a.label + i}
            disabled={a.disabled}
            hasTVPreferredFocus={i === 0}
            scale={isTV ? 1.03 : 1}
            accessibilityRole="menuitem"
            accessibilityLabel={a.label}
            onPress={() => {
              onClose();
              setTimeout(a.onPress, isTV ? 120 : 30);
            }}>
            {({focused, pressed}) => {
              const lit = focused && isTV;
              const fg = lit ? colors.actionInk : a.destructive ? colors.danger : colors.text;
              return (
                <View style={[styles.menuRow, lit && {backgroundColor: colors.white}, pressed && !isTV && {backgroundColor: 'rgba(237,245,250,0.08)'}]}>
                  {a.icon ? <Icon name={a.icon} size={19} color={lit ? colors.actionInk : a.destructive ? colors.danger : colors.accentStrong} /> : <View style={{width: u(19)}} />}
                  <View style={{flex: 1, marginLeft: u(14)}}>
                    <Text variant="bodyStrong" color={fg}>{a.label}</Text>
                    {a.detail ? <Text variant="caption" color={lit ? 'rgba(3,18,28,0.7)' : colors.textTertiary}>{a.detail}</Text> : null}
                  </View>
                  {a.checked ? <Icon name="check" size={18} color={lit ? colors.actionInk : colors.accentStrong} /> : null}
                </View>
              );
            }}
          </Focusable>
        ))}
      </ScrollView>
    </>
  );
}

function ConfirmBody({spec, onClose}: {spec: ConfirmSpec; onClose: () => void}) {
  const [typed, setTyped] = useState('');
  const ok = !spec.typeToConfirm || typed.trim() === spec.typeToConfirm;
  return (
    <View style={{padding: u(20)}}>
      <Text variant={isTV ? 'title' : 'heading'}>{spec.title}</Text>
      {spec.body ? <Text variant="body" tone="secondary" style={{marginTop: u(8)}}>{spec.body}</Text> : null}
      {spec.typeToConfirm ? <TextField label={`Type ${spec.typeToConfirm} to confirm`} value={typed} onChangeText={setTyped} style={{marginTop: u(14)}} error={typed && !ok ? `That doesn’t match. Type ${spec.typeToConfirm} exactly as shown.` : null} /> : null}
      <View style={[styles.buttons, {marginTop: u(20)}]}>
        <Button label={spec.cancelLabel ?? 'Cancel'} variant="secondary" onPress={() => { onClose(); spec.onCancel?.(); }} hasTVPreferredFocus />
        <Button label={spec.confirmLabel} variant={spec.destructive ? 'danger' : 'primary'} disabled={!ok} onPress={() => { onClose(); spec.onConfirm(); }} />
      </View>
    </View>
  );
}

function PromptBody({spec, onClose}: {spec: PromptSpec; onClose: () => void}) {
  const [values, setValues] = useState<Record<string, string>>(Object.fromEntries(spec.fields.map(f => [f.key, f.initial ?? ''])));
  const [error, setError] = useState<string | null>(null);
  const submit = () => {
    const res = spec.onSubmit(values);
    if (typeof res === 'string') setError(res);
    else onClose();
  };
  return (
    <ScrollView contentContainerStyle={{padding: u(20)}} keyboardShouldPersistTaps="handled">
      <Text variant={isTV ? 'title' : 'heading'}>{spec.title}</Text>
      {spec.body ? <Text variant="body" tone="secondary" style={{marginTop: u(6), marginBottom: u(10)}}>{spec.body}</Text> : <View style={{height: u(12)}} />}
      {spec.fields.map((f, i) => (
        <TextField key={f.key} label={f.label} value={values[f.key]} placeholder={f.placeholder} secure={f.secure} help={f.help} multiline={f.multiline} autoFocus={i === 0 && !isTV} autoCapitalize={f.secure ? 'none' : 'sentences'} onChangeText={v => setValues(p => ({...p, [f.key]: v}))} onSubmit={submit} />
      ))}
      {error ? <Text variant="caption" tone="danger" style={{marginBottom: u(8)}}>{error}</Text> : null}
      <View style={styles.buttons}>
        <Button label="Cancel" variant="secondary" onPress={onClose} />
        <Button label={spec.submitLabel} variant="primary" onPress={submit} hasTVPreferredFocus={isTV} />
      </View>
    </ScrollView>
  );
}

function ToastStack({toasts: list}: {toasts: Toast[]}) {
  const insets = useSafeAreaInsets();
  if (!list.length) return null;
  return (
    <View pointerEvents="box-none" style={[styles.toasts, {bottom: isTV ? 60 : insets.bottom + 90}]}>
      {list.map(t => (
        <View key={t.id} style={[styles.toast, t.tone === 'error' && {borderColor: colors.danger}]}>
          <Icon name={t.tone === 'error' ? 'alertCircle' : 'checkCircle'} size={17} color={t.tone === 'error' ? colors.danger : colors.healthy} />
          <Text variant="captionStrong" style={{marginLeft: u(10), flexShrink: 1}}>{t.message}</Text>
          {t.action ? (
            <Focusable onPress={t.action.onPress} style={{marginLeft: u(12)}} accessibilityRole="button" accessibilityLabel={t.action.label}>
              {({focused}) => <Text variant="captionStrong" tone="accent" style={focused && isTV ? {color: colors.white, textDecorationLine: 'underline'} : undefined}>{t.action!.label}</Text>}
            </Focusable>
          ) : null}
        </View>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  panel: {backgroundColor: colors.overlay, borderWidth: StyleSheet.hairlineWidth, borderColor: colors.line, overflow: 'hidden', shadowColor: '#000', shadowOpacity: 0.5, shadowRadius: 40},
  grabber: {alignSelf: 'center', width: 40, height: 5, borderRadius: 3, backgroundColor: colors.lineStrong, marginTop: 8},
  header: {flexDirection: 'row', alignItems: 'flex-start', paddingHorizontal: u(20), paddingTop: u(16), paddingBottom: u(10), gap: u(12)},
  menuRow: {flexDirection: 'row', alignItems: 'center', paddingHorizontal: u(14), paddingVertical: u(12), borderRadius: radius.md, minHeight: u(50)},
  buttons: {flexDirection: 'row', justifyContent: 'flex-end', gap: u(10), flexWrap: 'wrap'},
  toasts: {position: 'absolute', left: 0, right: 0, alignItems: 'center', gap: 8},
  toast: {flexDirection: 'row', alignItems: 'center', backgroundColor: colors.overlay, borderRadius: radius.pill, paddingHorizontal: u(18), paddingVertical: u(12), borderWidth: 1, borderColor: colors.lineStrong, maxWidth: '92%', shadowColor: '#000', shadowOpacity: 0.4, shadowRadius: 20},
});
