import React, {forwardRef, useCallback, useEffect, useRef, useState} from 'react';
import {ActivityIndicator, Animated, Easing, Platform, ScrollView, TextInput, View, type StyleProp, type TextInputProps, type ViewStyle} from 'react-native';
import {colors, fonts, profileArt} from '../theme/tokens';
import {useMetrics} from '../theme/layout';
import {Focusable} from './Focusable';
import {Icon, type IconName} from './Icon';
import {Text} from './Text';
import {Button} from './Button';
import {alpha} from './color';

export function ProgressBar({value, height = 4, color = colors.accent, track = 'rgba(237,245,250,0.28)', style}: {value: number; height?: number; color?: string; track?: string; style?: StyleProp<ViewStyle>}) {
  return (
    <View style={[{height, borderRadius: height, backgroundColor: track, overflow: 'hidden'}, style]}>
      <View style={{width: `${Math.max(0, Math.min(1, value)) * 100}%`, height, backgroundColor: color, borderRadius: height}} />
    </View>
  );
}

export function Surface({children, style, tone = 'raised', padded = true}: {children?: React.ReactNode; style?: StyleProp<ViewStyle>; tone?: 'raised' | 'soft' | 'overlay'; padded?: boolean}) {
  const m = useMetrics();
  const bg = tone === 'raised' ? colors.raised : tone === 'soft' ? colors.raisedSoft : colors.overlay;
  return <View style={[{backgroundColor: bg, borderRadius: m.radius.panel, borderWidth: 1, borderColor: colors.lineSoft}, padded ? {padding: m.space.lg} : null, style]}>{children}</View>;
}

export function Divider({style}: {style?: StyleProp<ViewStyle>}) {
  return <View style={[{height: 1, backgroundColor: colors.lineSoft}, style]} />;
}

export function Badge({label, tone = 'neutral', icon}: {label: string; tone?: 'neutral' | 'good' | 'warn' | 'bad' | 'accent' | 'record'; icon?: IconName}) {
  const m = useMetrics();
  const c = tone === 'good' ? colors.healthy : tone === 'warn' ? colors.amber : tone === 'bad' ? colors.danger : tone === 'accent' ? colors.accentStrong : tone === 'record' ? colors.record : colors.textSecondary;
  return (
    <View style={{flexDirection: 'row', alignItems: 'center', gap: 4, alignSelf: 'flex-start', paddingHorizontal: m.isTV ? 12 : 8, paddingVertical: m.isTV ? 4 : 2, borderRadius: 999, backgroundColor: alpha(c.length === 7 ? c : '#bdccd6', 0.14), borderWidth: 1, borderColor: alpha(c.length === 7 ? c : '#bdccd6', 0.35)}}>
      {icon ? <Icon name={icon} size={m.isTV ? 16 : 12} color={c} /> : null}
      <Text variant="micro" color={c} numberOfLines={1}>{label}</Text>
    </View>
  );
}

/** A metadata pill (content rating, resolution) on detail pages. */
export function Pill({label}: {label: string}) {
  const m = useMetrics();
  return (
    <View style={{paddingHorizontal: m.isTV ? 10 : 6, paddingVertical: m.isTV ? 2 : 1, borderRadius: m.isTV ? 6 : 4, borderWidth: 1.5, borderColor: colors.lineControl}}>
      <Text variant="micro" tone="secondary" style={{fontFamily: fonts.semibold}}>{label}</Text>
    </View>
  );
}

export function Chip({label, selected, onPress, icon, count, onClear}: {label: string; selected?: boolean; onPress?: () => void; icon?: IconName; count?: number; onClear?: () => void}) {
  const m = useMetrics();
  return (
    <Focusable
      onPress={onPress}
      focusScale={m.isTV ? 1.06 : 1}
      accessibilityRole="button"
      accessibilityState={{selected}}
      style={({focused}) => ({height: m.controlSmall, paddingHorizontal: m.isTV ? 22 : 14, borderRadius: 999, flexDirection: 'row', alignItems: 'center', gap: 6, backgroundColor: focused ? colors.focus : selected ? colors.accent : colors.raisedHigh, borderWidth: 1, borderColor: focused ? colors.focus : selected ? colors.accent : colors.line})}
    >
      {({focused}) => {
        const c = focused || selected ? colors.accentInk : colors.text;
        return (
          <>
            {icon ? <Icon name={icon} size={m.iconSmall} color={c} /> : null}
            <Text variant="captionStrong" color={c} numberOfLines={1}>{label}</Text>
            {count !== undefined ? <Text variant="micro" color={c}>{count}</Text> : null}
            {onClear && selected ? <Icon name="close" size={m.iconSmall - 2} color={c} /> : null}
          </>
        );
      }}
    </Focusable>
  );
}

/** Segmented control, and on TV a row of focusable segments. */
export function Segmented<T extends string>({options, value, onChange, style}: {options: {value: T; label: string; icon?: IconName}[]; value: T; onChange: (v: T) => void; style?: StyleProp<ViewStyle>}) {
  const m = useMetrics();
  return (
    <View style={[{flexDirection: 'row', backgroundColor: colors.raisedSoft, borderRadius: m.radius.control, padding: 3, alignSelf: 'flex-start', borderWidth: 1, borderColor: colors.lineSoft}, style]}>
      {options.map(o => (
        <Focusable key={o.value} onPress={() => onChange(o.value)} accessibilityRole="tab" accessibilityState={{selected: o.value === value}} pageFocus={o.value === value}
          style={({focused}) => ({height: m.controlSmall - 6, paddingHorizontal: m.isTV ? 22 : 14, borderRadius: m.radius.control - 3, flexDirection: 'row', alignItems: 'center', gap: 6, backgroundColor: focused ? colors.focus : o.value === value ? colors.raisedBright : 'transparent'})}>
          {({focused}) => (
            <>
              {o.icon ? <Icon name={o.icon} size={m.iconSmall} color={focused ? colors.accentInk : colors.text} /> : null}
              <Text variant="captionStrong" color={focused ? colors.accentInk : o.value === value ? colors.text : colors.textSecondary}>{o.label}</Text>
            </>
          )}
        </Focusable>
      ))}
    </View>
  );
}

/** Text tabs with an underline, used for library views and detail sections. */
/** When a TV tab last switched the view. A switch can rebuild the whole page (a library's Browse
 * grid carries its tabs in the grid's header), which destroys the focused tab; the tabs that mount
 * right after then hand focus to the tab that was chosen. Left to tvOS, focus landed on the first
 * tab, whose focus switched the page straight back (Movies → Browse → Discover). */
let lastTabSwitch = 0;
/** Focus must rest on a TV tab this long before it switches the view. */
const TAB_DWELL_MS = 250;

export function Tabs<T extends string>({tabs, value, onChange, style}: {tabs: {value: T; label: string; count?: number}[]; value: T; onChange: (v: T) => void; style?: StyleProp<ViewStyle>}) {
  const m = useMetrics();
  const Wrap = m.isPhone ? ScrollView : View;
  const [mountedAfterSwitch] = useState(() => Date.now() - lastTabSwitch < 1500);
  const pending = useRef<ReturnType<typeof setTimeout> | null>(null);
  const cancel = () => { if (pending.current !== null) clearTimeout(pending.current); pending.current = null; };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => cancel, []);
  const select = (v: T) => { lastTabSwitch = Date.now(); onChange(v); };
  const focusSelect = (v: T) => {
    cancel();
    pending.current = setTimeout(() => { pending.current = null; select(v); }, TAB_DWELL_MS);
  };
  return (
    <Wrap {...(m.isPhone ? {horizontal: true, showsHorizontalScrollIndicator: false, contentContainerStyle: [{flexDirection: 'row', gap: 4}, style]} : {style: [{flexDirection: 'row', gap: m.isTV ? 10 : 4, flexWrap: 'wrap'}, style]})}>
      {tabs.map(t => {
        const active = t.value === value;
        return (
          <Focusable key={t.value} onPress={() => { cancel(); select(t.value); }} accessibilityRole="tab" accessibilityState={{selected: active}} pageFocus={active}
            hasTVPreferredFocus={m.isTV && active && mountedAfterSwitch}
            onFocus={() => m.isTV && !active && focusSelect(t.value)} onBlur={cancel}
            style={({focused}) => ({paddingHorizontal: m.isTV ? 22 : 12, height: m.isTV ? 56 : 38, justifyContent: 'center', borderRadius: m.isTV ? 28 : 19, backgroundColor: focused ? colors.focus : active ? colors.raisedBright : 'transparent'})}>
            {({focused}) => (
              <View style={{flexDirection: 'row', alignItems: 'center', gap: 6}}>
                <Text variant={active ? 'bodyStrong' : 'body'} color={focused ? colors.accentInk : active ? colors.text : colors.textSecondary}>{t.label}</Text>
                {t.count !== undefined ? <Text variant="micro" color={focused ? colors.accentInk : colors.textTertiary}>{t.count}</Text> : null}
              </View>
            )}
          </Focusable>
        );
      })}
    </Wrap>
  );
}

/** A switch that works with a remote: the whole row toggles. */
export function Toggle({value, onChange, label, help, disabled, icon}: {value: boolean; onChange: (v: boolean) => void; label: string; help?: string; disabled?: boolean; icon?: IconName}) {
  const m = useMetrics();
  const x = useRef(new Animated.Value(value ? 1 : 0)).current;
  useEffect(() => { Animated.timing(x, {toValue: value ? 1 : 0, duration: 160, useNativeDriver: false}).start(); }, [value, x]);
  const w = m.isTV ? 64 : 46;
  const h = m.isTV ? 36 : 28;
  return (
    <Focusable onPress={() => onChange(!value)} disabled={disabled} accessibilityRole="switch" accessibilityState={{checked: value, disabled}} accessibilityLabel={label}
      style={({focused}) => ({flexDirection: 'row', alignItems: 'center', gap: m.space.md, paddingVertical: m.isTV ? 16 : 10, paddingHorizontal: m.isTV ? 22 : 0, borderRadius: m.radius.control, backgroundColor: focused ? colors.raisedBright : 'transparent'})}>
      {({focused}) => (
        <>
          {icon ? <Icon name={icon} size={m.icon} color={colors.textSecondary} /> : null}
          <View style={{flexGrow: 1, flexShrink: 1}}>
            <Text variant="body">{label}</Text>
            {help ? <Text variant="caption" tone="tertiary">{help}</Text> : null}
          </View>
          <Animated.View style={{width: w, height: h, borderRadius: h, padding: 3, backgroundColor: x.interpolate({inputRange: [0, 1], outputRange: [colors.track, colors.accent]}), borderWidth: focused ? 2 : 0, borderColor: colors.focus}}>
            <Animated.View style={{width: h - 6, height: h - 6, borderRadius: h, backgroundColor: colors.text, transform: [{translateX: x.interpolate({inputRange: [0, 1], outputRange: [0, w - h - (focused ? 4 : 0)]})}]}} />
          </Animated.View>
        </>
      )}
    </Focusable>
  );
}

export const TextField = forwardRef<TextInput, TextInputProps & {label?: string; help?: string; error?: string; icon?: IconName; style?: StyleProp<ViewStyle>}>(function TextField({label, help, error, icon, style, ...rest}, ref) {
  const m = useMetrics();
  const [focused, setFocused] = useState(false);
  const input = useRef<TextInput | null>(null);
  const setRef = useCallback((v: TextInput | null) => {
    input.current = v;
    if (typeof ref === 'function') ref(v); else if (ref) ref.current = v;
  }, [ref]);
  if (m.isTV) {
    // tvOS draws its own white pill around a focused text field. Show Portico's
    // field instead and open the system keyboard from it on select.
    const shown = rest.value ? (rest.secureTextEntry ? '•'.repeat(rest.value.length) : rest.value) : '';
    return (
      <View style={[{gap: 6}, style]}>
        {label ? <Text variant="captionStrong" tone="secondary">{label}</Text> : null}
        <Focusable onPress={() => input.current?.focus()} disabled={rest.editable === false} hasTVPreferredFocus={rest.autoFocus} accessibilityRole="search" accessibilityLabel={label ?? rest.placeholder}
          style={({focused: f}) => ({flexDirection: 'row', alignItems: 'center', gap: 12, minHeight: rest.multiline ? 140 : m.control, borderRadius: m.radius.control, backgroundColor: f ? colors.raisedHigh : colors.raisedSoft, borderWidth: f ? 3 : 1.5, borderColor: error ? colors.danger : f ? colors.focus : colors.lineControl, paddingHorizontal: 20})}>
          {icon ? <Icon name={icon} size={m.iconSmall + 2} color={colors.textTertiary} /> : null}
          <Text variant="body" color={shown ? colors.text : colors.placeholder} numberOfLines={rest.multiline ? 4 : 1} style={{flex: 1}}>{shown || rest.placeholder || ' '}</Text>
          <Icon name="edit" size={m.iconSmall} color={colors.textTertiary} />
        </Focusable>
        <TextInput ref={setRef} {...rest} multiline={false} autoFocus={false} focusable={false} style={{position: 'absolute', width: 1, height: 1, opacity: 0}} keyboardAppearance="dark" />
        {error ? <Text variant="caption" tone="danger">{error}</Text> : help ? <Text variant="caption" tone="tertiary">{help}</Text> : null}
      </View>
    );
  }
  return (
    <View style={[{gap: 6}, style]}>
      {label ? <Text variant="captionStrong" tone="secondary">{label}</Text> : null}
      <View style={{flexDirection: 'row', alignItems: 'center', gap: 8, minHeight: m.control, borderRadius: m.radius.control, backgroundColor: focused ? colors.raisedHigh : colors.raisedSoft, borderWidth: focused ? 2 : 1.5, borderColor: error ? colors.danger : focused ? (m.isTV ? colors.focus : colors.accent) : colors.lineControl, paddingHorizontal: m.isTV ? 20 : 12}}>
        {icon ? <Icon name={icon} size={m.iconSmall + 2} color={colors.textTertiary} /> : null}
        <TextInput
          ref={setRef}
          placeholderTextColor={colors.placeholder}
          selectionColor={colors.accent}
          keyboardAppearance="dark"
          onFocus={e => { setFocused(true); rest.onFocus?.(e); }}
          onBlur={e => { setFocused(false); rest.onBlur?.(e); }}
          {...rest}
          style={{flex: 1, color: colors.text, fontFamily: fonts.regular, fontSize: m.type.body.size, paddingVertical: m.isTV ? 14 : Platform.OS === 'ios' ? 10 : 6, minHeight: rest.multiline ? (m.isTV ? 140 : 90) : undefined, textAlignVertical: rest.multiline ? 'top' : 'center'}}
        />
      </View>
      {error ? <Text variant="caption" tone="danger">{error}</Text> : help ? <Text variant="caption" tone="tertiary">{help}</Text> : null}
    </View>
  );
});

export function Avatar({name, art = 'blue', size = 40, focused}: {name: string; art?: string; size?: number; focused?: boolean}) {
  const bg = (profileArt as Record<string, string>)[art] ?? profileArt.blue;
  return (
    <View style={{width: size, height: size, borderRadius: size / 2, backgroundColor: bg, alignItems: 'center', justifyContent: 'center', borderWidth: focused ? 3 : 0, borderColor: colors.focus}}>
      <Text style={{fontFamily: fonts.bold, fontSize: size * 0.42, lineHeight: size * 0.55, color: '#f3f8fb'}}>{name.slice(0, 1).toUpperCase()}</Text>
    </View>
  );
}

export function Skeleton({width, height, radius = 8, style}: {width: number | `${number}%`; height: number; radius?: number; style?: StyleProp<ViewStyle>}) {
  const o = useRef(new Animated.Value(0.45)).current;
  useEffect(() => {
    const loop = Animated.loop(Animated.sequence([Animated.timing(o, {toValue: 0.9, duration: 700, easing: Easing.inOut(Easing.quad), useNativeDriver: true}), Animated.timing(o, {toValue: 0.45, duration: 700, easing: Easing.inOut(Easing.quad), useNativeDriver: true})]));
    loop.start();
    return () => loop.stop();
  }, [o]);
  return <Animated.View style={[{width, height, borderRadius: radius, backgroundColor: colors.raisedHigh, opacity: o}, style]} />;
}

export function Loading({label}: {label?: string}) {
  const m = useMetrics();
  return (
    <View style={{alignItems: 'center', justifyContent: 'center', padding: m.space.xl, gap: m.space.sm}}>
      <ActivityIndicator color={colors.accentStrong} size={m.isTV ? 'large' : 'small'} />
      {label ? <Text variant="caption" tone="tertiary">{label}</Text> : null}
    </View>
  );
}

export function EmptyState({icon = 'sparkle', title, body, action, secondary, compact}: {icon?: IconName; title: string; body?: string; action?: {label: string; onPress: () => void; icon?: IconName}; secondary?: {label: string; onPress: () => void}; compact?: boolean}) {
  const m = useMetrics();
  return (
    <View style={{alignItems: 'center', justifyContent: 'center', paddingVertical: compact ? m.space.lg : m.space.xl * 1.5, paddingHorizontal: m.space.lg, gap: m.space.sm}}>
      <View style={{width: m.isTV ? 96 : 64, height: m.isTV ? 96 : 64, borderRadius: 48, backgroundColor: colors.raisedHigh, alignItems: 'center', justifyContent: 'center', marginBottom: m.space.sm}}>
        <Icon name={icon} size={m.isTV ? 44 : 28} color={colors.accentStrong} />
      </View>
      <Text variant="heading" center>{title}</Text>
      {body ? <Text variant="body" tone="secondary" center style={{maxWidth: m.isTV ? 820 : 420}}>{body}</Text> : null}
      <View style={{flexDirection: 'row', gap: m.space.sm, marginTop: m.space.md}}>
        {action ? <Button label={action.label} icon={action.icon} variant="primary" onPress={action.onPress} /> : null}
        {secondary ? <Button label={secondary.label} variant="secondary" onPress={secondary.onPress} /> : null}
      </View>
    </View>
  );
}

export function Notice({tone = 'info', title, body, action, icon, style}: {tone?: 'info' | 'warn' | 'error' | 'success'; title?: string; body: string; action?: {label: string; onPress: () => void}; icon?: IconName; style?: StyleProp<ViewStyle>}) {
  const m = useMetrics();
  const c = tone === 'error' ? colors.danger : tone === 'warn' ? colors.amber : tone === 'success' ? colors.healthy : colors.accentStrong;
  return (
    <View style={[{flexDirection: 'row', gap: m.space.md, alignItems: 'center', padding: m.space.md, borderRadius: m.radius.panel, backgroundColor: alpha(c, 0.1), borderWidth: 1, borderColor: alpha(c, 0.35)}, style]}>
      <Icon name={icon ?? (tone === 'error' || tone === 'warn' ? 'alert' : tone === 'success' ? 'check' : 'info')} size={m.icon} color={c} />
      <View style={{flex: 1, gap: 2}}>
        {title ? <Text variant="bodyStrong">{title}</Text> : null}
        <Text variant="caption" tone="secondary">{body}</Text>
      </View>
      {action ? <Button label={action.label} size="sm" variant="secondary" onPress={action.onPress} /> : null}
    </View>
  );
}

/** Settings-style row: icon, label, help, trailing value or chevron. */
export function ListRow({icon, title, subtitle, value, onPress, trailing, chevron = true, danger, selected, disabled, badge, left, hasTVPreferredFocus, onLongPress, onFocus}: {icon?: IconName; title: string; subtitle?: string; value?: string; onPress?: () => void; trailing?: React.ReactNode; chevron?: boolean; danger?: boolean; selected?: boolean; disabled?: boolean; badge?: string | number; left?: React.ReactNode; hasTVPreferredFocus?: boolean; onLongPress?: () => void; onFocus?: () => void}) {
  const m = useMetrics();
  const body = (focused: boolean) => (
    <>
      {left ?? (icon ? (
        <View style={{width: m.isTV ? 52 : 34, height: m.isTV ? 52 : 34, borderRadius: m.isTV ? 14 : 9, backgroundColor: focused ? colors.accentDeep : danger ? alpha(colors.danger, 0.14) : colors.raisedHigh, alignItems: 'center', justifyContent: 'center'}}>
          <Icon name={icon} size={m.iconSmall + (m.isTV ? 4 : 2)} color={danger ? colors.danger : focused ? colors.text : colors.accentStrong} />
        </View>
      ) : null)}
      <View style={{flex: 1, gap: 1}}>
        <Text variant="body" color={danger ? colors.danger : colors.text} numberOfLines={1}>{title}</Text>
        {subtitle ? <Text variant="caption" tone="tertiary" numberOfLines={2}>{subtitle}</Text> : null}
      </View>
      {badge !== undefined ? <View style={{minWidth: m.isTV ? 32 : 22, height: m.isTV ? 32 : 22, borderRadius: 16, paddingHorizontal: 6, backgroundColor: colors.accent, alignItems: 'center', justifyContent: 'center'}}><Text variant="micro" color={colors.accentInk}>{badge}</Text></View> : null}
      {value ? <Text variant="caption" tone="secondary" numberOfLines={1} style={{maxWidth: m.isTV ? 420 : 170}}>{value}</Text> : null}
      {trailing}
      {selected ? <Icon name="check" size={m.icon} color={colors.accentStrong} /> : onPress && chevron ? <Icon name="forward" size={m.iconSmall} color={colors.textTertiary} /> : null}
    </>
  );
  const pad = {flexDirection: 'row' as const, alignItems: 'center' as const, gap: m.space.md, paddingVertical: m.isTV ? 16 : 11, paddingHorizontal: m.isTV ? 22 : 14, minHeight: m.isTV ? 84 : 54};
  if (!onPress) return <View style={pad}>{body(false)}</View>;
  return (
    <Focusable onPress={onPress} onLongPress={onLongPress} onFocus={onFocus} disabled={disabled} hasTVPreferredFocus={hasTVPreferredFocus} accessibilityRole="button" accessibilityLabel={title} style={({focused}) => [pad, {borderRadius: m.radius.control, backgroundColor: focused ? colors.raisedBright : 'transparent'}]}>
      {({focused}) => body(focused)}
    </Focusable>
  );
}

/** Groups ListRows in a rounded surface, like iOS Settings. */
export function Group({title, footer, children, style}: {title?: string; footer?: string; children: React.ReactNode; style?: StyleProp<ViewStyle>}) {
  const m = useMetrics();
  const kids = React.Children.toArray(children).filter(Boolean);
  return (
    <View style={[{gap: m.space.xs}, style]}>
      {title ? <Text variant="label" tone="tertiary" style={{marginLeft: m.isTV ? 22 : 14, marginBottom: 2}}>{title}</Text> : null}
      <View style={{backgroundColor: colors.raised, borderRadius: m.radius.panel, borderWidth: 1, borderColor: colors.lineSoft, padding: m.isTV ? 8 : 2}}>
        {kids.map((c, i) => (
          <React.Fragment key={i}>
            {i > 0 ? <View style={{height: 1, backgroundColor: colors.lineSoft, marginLeft: m.isTV ? 22 : 14}} /> : null}
            {c}
          </React.Fragment>
        ))}
      </View>
      {footer ? <Text variant="caption" tone="tertiary" style={{marginHorizontal: m.isTV ? 22 : 14}}>{footer}</Text> : null}
    </View>
  );
}

export function SectionTitle({title, action, subtitle, style}: {title: string; subtitle?: string; action?: {label: string; onPress: () => void}; style?: StyleProp<ViewStyle>}) {
  const m = useMetrics();
  return (
    <View style={[{flexDirection: 'row', alignItems: 'flex-end', justifyContent: 'space-between', gap: m.space.md}, style]}>
      <View style={{flex: 1}}>
        <Text variant="heading" numberOfLines={1}>{title}</Text>
        {subtitle ? <Text variant="caption" tone="tertiary" numberOfLines={1}>{subtitle}</Text> : null}
      </View>
      {action ? <Button label={action.label} variant="ghost" size="sm" iconRight="forward" onPress={action.onPress} /> : null}
    </View>
  );
}

/** Number stepper that works with touch and the remote's left/right. */
export function Stepper({value, onChange, min = 0, max = 100, step = 1, unit, label}: {value: number; onChange: (v: number) => void; min?: number; max?: number; step?: number; unit?: string; label: string}) {
  const m = useMetrics();
  return (
    <View style={{flexDirection: 'row', alignItems: 'center', gap: m.space.sm}} accessibilityLabel={label}>
      <Button icon="minus" size="sm" variant="secondary" accessibilityLabel={`Decrease ${label}`} onPress={() => onChange(Math.max(min, value - step))} disabled={value <= min} />
      <Text variant="bodyStrong" style={{minWidth: m.isTV ? 110 : 72, textAlign: 'center'}}>{value}{unit ? ` ${unit}` : ''}</Text>
      <Button icon="plus" size="sm" variant="secondary" accessibilityLabel={`Increase ${label}`} onPress={() => onChange(Math.min(max, value + step))} disabled={value >= max} />
    </View>
  );
}

/** Four-digit PIN pad; works with touch and the remote. */
export function PinPad({length = 4, onComplete, error}: {length?: number; onComplete: (pin: string) => void; error?: string}) {
  const m = useMetrics();
  const [pin, setPin] = useState('');
  const shake = useRef(new Animated.Value(0)).current;
  useEffect(() => {
    if (!error) return;
    setPin('');
    Animated.sequence([10, -10, 8, -8, 0].map(v => Animated.timing(shake, {toValue: v, duration: 50, useNativeDriver: true}))).start();
  }, [error, shake]);
  const press = (d: string) => {
    const next = (pin + d).slice(0, length);
    setPin(next);
    if (next.length === length) setTimeout(() => onComplete(next), 120);
  };
  const key = m.isTV ? 96 : 68;
  const keys = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '', '0', '⌫'];
  return (
    <View style={{alignItems: 'center', gap: m.space.lg}}>
      <Animated.View style={{flexDirection: 'row', gap: m.space.md, transform: [{translateX: shake}]}} accessibilityLabel={`${pin.length} of ${length} digits entered`}>
        {Array.from({length}, (_, i) => <View key={i} style={{width: m.isTV ? 26 : 16, height: m.isTV ? 26 : 16, borderRadius: 13, borderWidth: 2, borderColor: error ? colors.danger : colors.textSecondary, backgroundColor: i < pin.length ? colors.text : 'transparent'}} />)}
      </Animated.View>
      {error ? <Text variant="caption" tone="danger">{error}</Text> : null}
      <View style={{width: key * 3 + m.space.md * 2, flexDirection: 'row', flexWrap: 'wrap', gap: m.space.md}}>
        {keys.map((k, i) => (k === '' ? <View key={i} style={{width: key, height: key}} /> : (
          <Focusable key={i} onPress={() => (k === '⌫' ? setPin(p => p.slice(0, -1)) : press(k))} focusScale={1.1} accessibilityLabel={k === '⌫' ? 'Delete digit' : k} hasTVPreferredFocus={k === '1'}
            style={({focused}) => ({width: key, height: key, borderRadius: key / 2, alignItems: 'center', justifyContent: 'center', backgroundColor: focused ? colors.focus : colors.raisedHigh})}>
            {({focused}) => (k === '⌫' ? <Icon name="back" color={focused ? colors.accentInk : colors.text} size={m.icon} /> : <Text variant="title" color={focused ? colors.accentInk : colors.text}>{k}</Text>)}
          </Focusable>
        )))}
      </View>
    </View>
  );
}
