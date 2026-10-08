import React, {useRef, useState} from 'react';
import {StyleSheet, TextInput, View, type KeyboardTypeOptions, type StyleProp, type ViewStyle} from 'react-native';
import {Focusable} from './Focusable';
import {Icon, type IconName} from './Icon';
import {Text} from './Text';
import {colors, fonts, isTV, radius, u} from './tokens';

/** Labelled text field. On TV, selecting it opens the system keyboard. */
export function TextField({label, value, onChangeText, placeholder, help, error, secure, keyboardType, autoFocus, multiline, maxLength, onSubmit, style, autoCapitalize = 'none', icon}: {label?: string; value: string; onChangeText: (v: string) => void; placeholder?: string; help?: string; error?: string | null; secure?: boolean; keyboardType?: KeyboardTypeOptions; autoFocus?: boolean; multiline?: boolean; maxLength?: number; onSubmit?: () => void; style?: StyleProp<ViewStyle>; autoCapitalize?: 'none' | 'sentences' | 'words'; icon?: IconName}) {
  const [focused, setFocused] = useState(false);
  const ref = useRef<TextInput>(null);
  return (
    <View style={[{marginBottom: u(14)}, style]}>
      {label ? <Text variant="captionStrong" tone="secondary" style={{marginBottom: u(6)}}>{label}</Text> : null}
      <View style={[styles.field, isTV && styles.fieldTV, multiline && {height: u(110), alignItems: 'flex-start', paddingTop: u(10)}, focused && !isTV && styles.fieldFocused, !!error && {borderColor: colors.danger, borderWidth: isTV ? 2 : 1}]}>
        {icon ? <View style={{marginRight: u(8)}}><Icon name={icon} size={16} color={colors.textTertiary} /></View> : null}
        <TextInput
          ref={ref}
          value={value}
          onChangeText={onChangeText}
          placeholder={placeholder}
          placeholderTextColor={colors.placeholder}
          secureTextEntry={secure}
          keyboardType={keyboardType}
          autoFocus={autoFocus}
          multiline={multiline}
          maxLength={maxLength}
          autoCapitalize={autoCapitalize}
          autoCorrect={false}
          keyboardAppearance="dark"
          onSubmitEditing={onSubmit}
          onFocus={() => setFocused(true)}
          onBlur={() => setFocused(false)}
          style={[styles.input, multiline && {textAlignVertical: 'top', height: '100%'}]}
        />
      </View>
      {error ? <Text variant="caption" tone="danger" style={{marginTop: u(4)}}>{error}</Text> : help ? <Text variant="caption" tone="tertiary" style={{marginTop: u(4)}}>{help}</Text> : null}
    </View>
  );
}

/** A settings-style row: icon, label, help, and a trailing control or value. */
export function ListRow({icon, label, help, value, onPress, right, destructive, chevron = !!onPress, disabled, hasTVPreferredFocus, compact}: {icon?: IconName; label: string; help?: string; value?: string; onPress?: () => void; right?: React.ReactNode; destructive?: boolean; chevron?: boolean; disabled?: boolean; hasTVPreferredFocus?: boolean; compact?: boolean}) {
  const body = (focused: boolean) => {
    const lit = focused && isTV;
    const fg = lit ? colors.actionInk : destructive ? colors.danger : colors.text;
    return (
      <View style={[styles.row, compact && {minHeight: u(44)}, lit && styles.rowLit]}>
        {icon ? <View style={[styles.rowIcon, lit && {backgroundColor: 'rgba(3,18,28,0.1)'}]}><Icon name={icon} size={17} color={lit ? colors.actionInk : destructive ? colors.danger : colors.accentStrong} /></View> : null}
        <View style={{flex: 1, minWidth: '46%'}}>
          <Text variant="bodyStrong" color={fg} numberOfLines={2}>{label}</Text>
          {help ? <Text variant="caption" color={lit ? 'rgba(3,18,28,0.7)' : colors.textTertiary} numberOfLines={3}>{help}</Text> : null}
        </View>
        {value ? <Text variant="caption" color={lit ? colors.actionInk : colors.textSecondary} style={{marginLeft: u(10), flexShrink: 1}} numberOfLines={1}>{value}</Text> : null}
        {right}
        {chevron ? <View style={{marginLeft: u(6)}}><Icon name="chevronRight" size={16} color={lit ? colors.actionInk : colors.textTertiary} /></View> : null}
      </View>
    );
  };
  if (!onPress) return body(false);
  return (
    <Focusable onPress={onPress} disabled={disabled} scale={isTV ? 1.02 : 1} accessibilityRole="button" accessibilityLabel={label} hasTVPreferredFocus={hasTVPreferredFocus}>
      {({focused, pressed}) => <View style={pressed && !isTV ? {opacity: 0.7} : null}>{body(focused)}</View>}
    </Focusable>
  );
}

/** A switch row. The whole row toggles, so it's one focus stop on TV. */
export function ToggleRow({label, help, value, onChange, icon, disabled}: {label: string; help?: string; value: boolean; onChange: (v: boolean) => void; icon?: IconName; disabled?: boolean}) {
  return (
    <Focusable onPress={() => onChange(!value)} disabled={disabled} scale={isTV ? 1.02 : 1} accessibilityRole="switch" accessibilityLabel={label} accessibilityState={{checked: value}}>
      {({focused}) => {
        const lit = focused && isTV;
        return (
          <View style={[styles.row, lit && styles.rowLit]}>
            {icon ? <View style={styles.rowIcon}><Icon name={icon} size={17} color={lit ? colors.actionInk : colors.accentStrong} /></View> : null}
            <View style={{flex: 1}}>
              <Text variant="bodyStrong" color={lit ? colors.actionInk : colors.text}>{label}</Text>
              {help ? <Text variant="caption" color={lit ? 'rgba(3,18,28,0.7)' : colors.textTertiary}>{help}</Text> : null}
            </View>
            <View style={[styles.switch, value && styles.switchOn]}>
              <View style={[styles.knob, value && styles.knobOn]} />
            </View>
          </View>
        );
      }}
    </Focusable>
  );
}

/** Segmented choice. */
export function Segmented<T extends string>({options, value, onChange, style}: {options: {value: T; label: string; icon?: IconName}[]; value: T; onChange: (v: T) => void; style?: StyleProp<ViewStyle>}) {
  return (
    <View style={[styles.segments, style]}>
      {options.map(o => {
        const on = o.value === value;
        return (
          <Focusable key={o.value} onPress={() => onChange(o.value)} scale={1.05} accessibilityRole="tab" accessibilityState={{selected: on}} accessibilityLabel={o.label}>
            {({focused}) => {
              const lit = focused && isTV;
              return (
                <View style={[styles.segment, on && styles.segmentOn, lit && {backgroundColor: colors.white}]}>
                  {o.icon ? <Icon name={o.icon} size={15} color={lit || on ? colors.actionInk : colors.textSecondary} /> : null}
                  <Text variant="captionStrong" color={lit || on ? colors.actionInk : colors.textSecondary} style={o.icon ? {marginLeft: u(6)} : undefined}>{o.label}</Text>
                </View>
              );
            }}
          </Focusable>
        );
      })}
    </View>
  );
}

/** A pill chip used for filters, tabs and tags. */
export function Chip({label, active, onPress, icon, count, hasTVPreferredFocus}: {label: string; active?: boolean; onPress?: () => void; icon?: IconName; count?: number; hasTVPreferredFocus?: boolean}) {
  return (
    <Focusable onPress={onPress} scale={1.08} hasTVPreferredFocus={hasTVPreferredFocus} accessibilityRole="button" accessibilityState={{selected: active}} accessibilityLabel={label}>
      {({focused}) => {
        const lit = focused && isTV;
        const fg = lit || active ? colors.actionInk : colors.text;
        return (
          <View style={[styles.chip, active && {backgroundColor: colors.accentStrong, borderColor: colors.accentStrong}, lit && {backgroundColor: colors.white, borderColor: colors.white}]}>
            {icon ? <Icon name={icon} size={14} color={fg} /> : null}
            <Text variant="captionStrong" color={fg} style={icon ? {marginLeft: u(6)} : undefined}>{label}</Text>
            {count !== undefined ? <Text variant="caption" color={lit || active ? 'rgba(3,18,28,0.7)' : colors.textTertiary} style={{marginLeft: u(6)}}>{count}</Text> : null}
          </View>
        );
      }}
    </Focusable>
  );
}

/** Section card with a title, used across settings and admin. */
export function Panel({title, lede, children, right, style}: {title?: string; lede?: string; children: React.ReactNode; right?: React.ReactNode; style?: StyleProp<ViewStyle>}) {
  return (
    <View style={[styles.panel, style]}>
      {title ? (
        <View style={styles.panelHead}>
          <View style={{flex: 1}}>
            <Text variant="heading">{title}</Text>
            {lede ? <Text variant="caption" tone="tertiary" style={{marginTop: 2}}>{lede}</Text> : null}
          </View>
          {right}
        </View>
      ) : null}
      {children}
    </View>
  );
}

/** Stepper for numeric settings (works with a remote). */
export function Stepper({label, help, value, onChange, min = 0, max = 999, step = 1, unit}: {label: string; help?: string; value: number; onChange: (v: number) => void; min?: number; max?: number; step?: number; unit?: string}) {
  return (
    <View style={styles.row}>
      <View style={{flex: 1}}>
        <Text variant="bodyStrong">{label}</Text>
        {help ? <Text variant="caption" tone="tertiary">{help}</Text> : null}
      </View>
      <StepButton icon="minus" label={`Decrease ${label}`} onPress={() => onChange(Math.max(min, value - step))} />
      <Text variant="bodyStrong" style={{minWidth: u(64), textAlign: 'center'}}>{value}{unit ? ` ${unit}` : ''}</Text>
      <StepButton icon="plus" label={`Increase ${label}`} onPress={() => onChange(Math.min(max, value + step))} />
    </View>
  );
}

function StepButton({icon, label, onPress}: {icon: IconName; label: string; onPress: () => void}) {
  return (
    <Focusable onPress={onPress} scale={1.12} accessibilityRole="button" accessibilityLabel={label}>
      {({focused}) => (
        <View style={[styles.step, focused && isTV && {backgroundColor: colors.white}]}>
          <Icon name={icon} size={15} color={focused && isTV ? colors.actionInk : colors.text} />
        </View>
      )}
    </Focusable>
  );
}

/** Progress / usage bar. */
export function Meter({value, color = colors.accentStrong, height = 6, style}: {value: number; color?: string; height?: number; style?: StyleProp<ViewStyle>}) {
  return (
    <View style={[{height: u(height), borderRadius: 99, backgroundColor: colors.trackRest, overflow: 'hidden'}, style]}>
      <View style={{width: `${Math.max(0, Math.min(1, value)) * 100}%`, height: '100%', backgroundColor: color, borderRadius: 99}} />
    </View>
  );
}

export function Badge({label, tone = 'accent'}: {label: string; tone?: 'accent' | 'amber' | 'danger' | 'healthy' | 'neutral' | 'record'}) {
  const bg = {accent: 'rgba(92,179,226,0.18)', amber: 'rgba(232,188,106,0.18)', danger: 'rgba(243,139,136,0.18)', healthy: 'rgba(134,207,179,0.18)', neutral: 'rgba(237,245,250,0.1)', record: 'rgba(241,121,121,0.2)'}[tone];
  const fg = {accent: colors.accentStrong, amber: colors.amber, danger: colors.danger, healthy: colors.healthy, neutral: colors.textSecondary, record: colors.record}[tone];
  return (
    <View style={{backgroundColor: bg, borderRadius: u(6), paddingHorizontal: u(7), paddingVertical: u(2), alignSelf: 'flex-start'}}>
      <Text variant="micro" color={fg}>{label.toUpperCase()}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  field: {flexDirection: 'row', alignItems: 'center', height: u(46), borderRadius: radius.md, borderWidth: 1, borderColor: colors.lineControl, backgroundColor: 'rgba(7,16,24,0.6)', paddingHorizontal: u(12)},
  fieldFocused: {borderColor: colors.accentStrong, borderWidth: 2},
  // tvOS draws its own field (dark when idle, white when focused), so the frame steps aside.
  fieldTV: {backgroundColor: 'transparent', borderWidth: 0, paddingHorizontal: 0, height: 66, borderRadius: 12},
  input: {flex: 1, color: colors.text, fontFamily: fonts.medium, fontSize: isTV ? 28 : 16, padding: 0},
  row: {flexDirection: 'row', alignItems: 'center', minHeight: u(56), paddingVertical: u(10), paddingHorizontal: u(14), borderRadius: radius.md, gap: u(10)},
  rowLit: {backgroundColor: colors.white},
  rowIcon: {width: u(32), height: u(32), borderRadius: u(9), backgroundColor: 'rgba(92,179,226,0.12)', alignItems: 'center', justifyContent: 'center'},
  switch: {width: u(46), height: u(28), borderRadius: u(14), backgroundColor: colors.trackRest, padding: u(3)},
  switchOn: {backgroundColor: colors.accent},
  knob: {width: u(22), height: u(22), borderRadius: u(11), backgroundColor: colors.white},
  knobOn: {transform: [{translateX: u(18)}]},
  segments: {flexDirection: 'row', backgroundColor: 'rgba(237,245,250,0.08)', borderRadius: radius.pill, padding: u(3), alignSelf: 'flex-start', gap: u(2)},
  segment: {flexDirection: 'row', alignItems: 'center', paddingHorizontal: u(14), height: u(32), borderRadius: radius.pill},
  segmentOn: {backgroundColor: colors.accentStrong},
  chip: {flexDirection: 'row', alignItems: 'center', height: u(34), paddingHorizontal: u(14), borderRadius: radius.pill, borderWidth: 1, borderColor: 'rgba(237,245,250,0.18)', backgroundColor: 'rgba(237,245,250,0.06)'},
  panel: {backgroundColor: 'rgba(16,33,45,0.72)', borderRadius: radius.lg, padding: u(14), marginBottom: u(16), borderWidth: StyleSheet.hairlineWidth, borderColor: colors.lineSoft},
  panelHead: {flexDirection: 'row', alignItems: 'flex-start', marginBottom: u(10), paddingHorizontal: u(4)},
  step: {width: u(34), height: u(34), borderRadius: u(17), backgroundColor: 'rgba(237,245,250,0.1)', alignItems: 'center', justifyContent: 'center'},
});
