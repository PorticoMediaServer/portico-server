import React, {useEffect, useState} from 'react';
import {ScrollView, View} from 'react-native';
import Svg, {Path, Rect, Line} from 'react-native-svg';
import type {AdminAction, AdminField, Block, FolderListing, ListItem, Tone} from '../../data/adminTypes';
import {useAdmin} from '../../data/DataProvider';
import {Button} from '../../ui/Button';
import {Badge, Chip, ListRow, ProgressBar, Stepper, Surface, TextField, Toggle} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {Artwork} from '../../ui/Artwork';
import {Focusable} from '../../ui/Focusable';
import {Icon, type IconName} from '../../ui/Icon';
import {ChoiceSheet, Sheet} from '../../ui/Sheet';
import {colors} from '../../theme/tokens';
import {useMetrics} from '../../theme/layout';

export const toneColor = (t?: Tone) => (t === 'good' ? colors.healthy : t === 'warn' ? colors.amber : t === 'bad' ? colors.danger : t === 'accent' ? colors.accentStrong : colors.textSecondary);
const badgeTone = (t: Tone) => (t === 'neutral' ? 'neutral' : t) as 'neutral' | 'good' | 'warn' | 'bad' | 'accent';

type Ctx = {onAction: (a: AdminAction) => void; onValue: (key: string, v: unknown) => void; onOpen: (section: string, param?: string) => void};

/** Renders one admin block. Interactive on every device; the TV gets the same controls with focus. */
export function BlockView({block, ctx}: {block: Block; ctx: Ctx}) {
  const m = useMetrics();
  switch (block.type) {
    case 'stats': return (
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: m.gap}}>
        {block.items.map(s => (
          <Surface key={s.label} style={{flexGrow: 1, flexBasis: m.isPhone ? '45%' : m.isTV ? 260 : 200, gap: 4}}>
            <Text variant="caption" tone="tertiary">{s.label}</Text>
            <Text variant="title" color={s.tone ? toneColor(s.tone) : colors.text}>{s.value}</Text>
            {s.sub ? <Text variant="caption" tone="secondary" numberOfLines={1}>{s.sub}</Text> : null}
            {s.spark ? <Spark values={s.spark} /> : null}
          </Surface>
        ))}
      </View>
    );
    case 'notice': return (
      <View style={{padding: m.space.md, borderRadius: m.radius.panel, backgroundColor: `${toneColor(block.tone)}1a`, borderWidth: 1, borderColor: `${toneColor(block.tone)}55`, gap: 8}}>
        <View style={{flexDirection: 'row', gap: 10, alignItems: 'center'}}><Icon name={block.tone === 'good' ? 'check' : block.tone === 'neutral' ? 'info' : 'alert'} color={toneColor(block.tone)} size={m.icon} /><Text variant="bodyStrong" style={{flex: 1}}>{block.title}</Text></View>
        <Text variant="caption" tone="secondary">{block.body}</Text>
        {block.actions?.length ? <View style={{flexDirection: 'row', gap: 8}}>{block.actions.map(a => <ActionButton key={a.id + a.label} a={a} ctx={ctx} />)}</View> : null}
      </View>
    );
    case 'settings': return (
      <Section title={block.title} lede={block.lede}>
        <Surface padded={false} style={{padding: m.isTV ? 8 : 4}}>
          {block.fields.map((f, i) => <View key={f.key} style={{borderTopWidth: i ? 1 : 0, borderColor: colors.lineSoft}}><FieldView f={f} ctx={ctx} /></View>)}
        </Surface>
      </Section>
    );
    case 'facts': return (
      <Section title={block.title} lede={block.lede}>
        <Surface style={{gap: m.isTV ? 12 : 8}}>
          {block.rows.map(r => (
            <View key={r.label} style={{flexDirection: m.isPhone ? 'column' : 'row', gap: m.isPhone ? 2 : m.space.md}}>
              <Text variant="caption" tone="tertiary" style={{width: m.isPhone ? undefined : m.isTV ? 320 : 220}}>{r.label}</Text>
              <Text variant="captionStrong" color={r.tone ? toneColor(r.tone) : colors.text} style={{flex: 1}}>{r.value}</Text>
            </View>
          ))}
        </Surface>
      </Section>
    );
    case 'list': return <ListBlock block={block} ctx={ctx} />;
    case 'table': return (
      <Section title={block.title} lede={block.lede}>
        <ScrollView horizontal showsHorizontalScrollIndicator={false}>
          <Surface padded={false} style={{minWidth: m.isPhone ? 560 : '100%'}}>
            <View style={{flexDirection: 'row', paddingHorizontal: 14, paddingVertical: 10, borderBottomWidth: 1, borderColor: colors.line}}>{block.columns.map((c, i) => <Text key={c} variant="label" tone="tertiary" style={{flex: i === 0 ? 2.2 : 1}}>{c}</Text>)}</View>
            {block.rows.map((r, j) => <View key={j} style={{flexDirection: 'row', paddingHorizontal: 14, paddingVertical: m.isTV ? 14 : 10, borderTopWidth: j ? 1 : 0, borderColor: colors.lineSoft}}>{r.map((c, i) => <Text key={i} variant="caption" tone={i === 0 ? 'primary' : 'secondary'} style={{flex: i === 0 ? 2.2 : 1}} numberOfLines={1}>{c}</Text>)}</View>)}
          </Surface>
        </ScrollView>
      </Section>
    );
    case 'chart': return (
      <Section title={block.title} lede={block.lede}>
        <Surface><Bars values={block.series[0]!.values} labels={block.labels} unit={block.unit} /></Surface>
      </Section>
    );
    case 'meters': return (
      <Section title={block.title}>
        <Surface style={{gap: m.space.md}}>
          {block.items.map(it => (
            <View key={it.label} style={{gap: 6}}>
              <View style={{flexDirection: 'row', justifyContent: 'space-between', gap: 8}}><Text variant="captionStrong" style={{flex: 1}}>{it.label}</Text><Text variant="caption" tone="secondary">{it.detail}</Text></View>
              <ProgressBar value={it.value} height={m.isTV ? 10 : 7} color={it.tone === 'warn' ? colors.amber : it.tone === 'bad' ? colors.danger : colors.accent} track={colors.track} />
            </View>
          ))}
        </Surface>
      </Section>
    );
    case 'actions': return (
      <Section title={block.title} lede={block.lede}>
        <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 8}}>{block.actions.map(a => <ActionButton key={a.id + a.label} a={a} ctx={ctx} />)}</View>
      </Section>
    );
    case 'timeline': {
      const span = block.to - block.from;
      const now = Date.now();
      return (
        <Section title={block.title} lede={block.lede}>
          <Surface style={{gap: 10}}>
            {block.lanes.map(l => (
              <View key={l.label} style={{flexDirection: m.isPhone ? 'column' : 'row', gap: m.isPhone ? 4 : 12, alignItems: m.isPhone ? 'stretch' : 'center'}}>
                <Text variant="caption" tone="secondary" style={{width: m.isPhone ? undefined : m.isTV ? 280 : 180}}>{l.label}</Text>
                <View style={{flex: 1, height: m.isTV ? 40 : 28, backgroundColor: colors.raisedSoft, borderRadius: 6, overflow: 'hidden'}}>
                  {l.spans.map(s => <View key={s.label} style={{position: 'absolute', left: `${Math.max(0, ((s.start - block.from) / span) * 100)}%`, width: `${Math.min(100, ((s.end - s.start) / span) * 100)}%`, top: 3, bottom: 3, borderRadius: 5, backgroundColor: `${toneColor(s.tone)}55`, borderWidth: 1, borderColor: toneColor(s.tone), justifyContent: 'center', paddingHorizontal: 6}}><Text variant="micro" numberOfLines={1}>{s.label}</Text></View>)}
                  <View style={{position: 'absolute', left: `${((now - block.from) / span) * 100}%`, top: 0, bottom: 0, width: 2, backgroundColor: colors.record}} />
                </View>
              </View>
            ))}
          </Surface>
        </Section>
      );
    }
  }
}

function Section({title, lede, children}: {title?: string; lede?: string; children: React.ReactNode}) {
  const m = useMetrics();
  return (
    <View style={{gap: m.space.sm}}>
      {title ? <Text variant="heading">{title}</Text> : null}
      {lede ? <Text variant="caption" tone="tertiary" style={{maxWidth: 900}}>{lede}</Text> : null}
      {children}
    </View>
  );
}

function ListBlock({block, ctx}: {block: Extract<Block, {type: 'list'}>; ctx: Ctx}) {
  const m = useMetrics();
  const [open, setOpen] = useState<string>();
  return (
    <Section title={block.title} lede={block.lede}>
      {block.filters ? <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 6}}>{block.filters.options.map(o => <Chip key={o.value} label={o.label} selected={block.filters!.value === o.value} onPress={() => ctx.onValue(block.filters!.key, o.value)} />)}</View> : null}
      <Surface padded={false} style={{padding: m.isTV ? 8 : 4}}>
        {block.items.length ? block.items.map((it, i) => (
          <View key={it.id} style={{borderTopWidth: i ? 1 : 0, borderColor: colors.lineSoft}}>
            <ItemRow it={it} expanded={open === it.id} onPress={it.open ? () => ctx.onOpen(it.open!.section, it.open!.param) : it.actions?.length || it.detail ? () => setOpen(o => (o === it.id ? undefined : it.id)) : undefined} />
            {open === it.id ? (
              <View style={{paddingHorizontal: m.isTV ? 22 : 14, paddingBottom: 12, gap: 10}}>
                {it.detail ? <View style={{gap: 4}}>{it.detail.map(d => <View key={d.label} style={{flexDirection: 'row', gap: 12}}><Text variant="caption" tone="tertiary" style={{width: m.isTV ? 180 : 100}}>{d.label}</Text><Text variant="caption">{d.value}</Text></View>)}</View> : null}
                {it.actions?.length ? <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 8}}>{it.actions.map(a => <ActionButton key={a.id + a.label} a={{...a, target: a.target ?? it.id}} ctx={ctx} />)}</View> : null}
              </View>
            ) : null}
          </View>
        )) : block.empty ? <View style={{padding: m.space.lg, gap: 4}}><Text variant="bodyStrong">{block.empty.title}</Text><Text variant="caption" tone="tertiary">{block.empty.body}</Text></View> : null}
      </Surface>
      {block.actions?.length ? <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 8}}>{block.actions.map(a => <ActionButton key={a.id + a.label} a={a} ctx={ctx} />)}</View> : null}
    </Section>
  );
}

function ItemRow({it, onPress, expanded}: {it: ListItem; onPress?: () => void; expanded: boolean}) {
  const m = useMetrics();
  const left = it.image ? <Artwork source={it.image} title={it.title} width={m.isTV ? 64 : 42} height={m.isTV ? 64 : 42} radius={it.round ? 40 : 6} resizeMode="contain" style={{backgroundColor: 'transparent'}} /> : it.icon ? (
    <View style={{width: m.isTV ? 52 : 34, height: m.isTV ? 52 : 34, borderRadius: m.isTV ? 14 : 9, backgroundColor: colors.raisedHigh, alignItems: 'center', justifyContent: 'center'}}><Icon name={it.icon as IconName} size={m.iconSmall + 2} color={colors.accentStrong} /></View>
  ) : undefined;
  return (
    <ListRow left={left} title={it.title} subtitle={[it.subtitle, it.meta].filter(Boolean).join('\n')} onPress={onPress} chevron={!!it.open}
      trailing={
        <View style={{alignItems: 'flex-end', gap: 6, maxWidth: m.isPhone ? 110 : 240}}>
          {it.badge ? <Badge label={it.badge.label} tone={badgeTone(it.badge.tone)} /> : null}
          {it.progress !== undefined ? <ProgressBar value={it.progress} style={{width: m.isTV ? 160 : 80}} track={colors.track} /> : null}
          {!it.open && (it.actions?.length || it.detail) ? <Icon name={expanded ? 'chevronUp' : 'chevronDown'} size={m.iconSmall} color={colors.textTertiary} /> : null}
        </View>
      } />
  );
}

export function ActionButton({a, ctx}: {a: AdminAction; ctx: Ctx}) {
  return <Button size="sm" label={a.label} icon={a.icon as IconName | undefined} variant={a.tone === 'primary' ? 'primary' : a.tone === 'danger' ? 'danger' : a.tone === 'ghost' ? 'ghost' : 'secondary'} onPress={() => ctx.onAction(a)} />;
}

export function FieldView({f, ctx, local}: {f: AdminField; ctx: Ctx; local?: boolean}) {
  const m = useMetrics();
  const [text, setText] = useState(f.kind === 'text' ? f.value ?? '' : '');
  const [pick, setPick] = useState(false);
  const pad = {paddingHorizontal: m.isTV ? 0 : 14, paddingVertical: 4};
  if (f.kind === 'toggle') return <View style={{paddingHorizontal: m.isTV ? 0 : 14}}><Toggle label={f.label} help={f.help} value={!!f.value} onChange={v => ctx.onValue(f.key, v)} /></View>;
  if (f.kind === 'number') return (
    <View style={[pad, {flexDirection: m.isPhone ? 'column' : 'row', alignItems: m.isPhone ? 'flex-start' : 'center', gap: 10, paddingVertical: 12, paddingHorizontal: m.isTV ? 22 : 14}]}>
      <View style={{flex: 1}}><Text variant="body">{f.label}</Text>{f.help ? <Text variant="caption" tone="tertiary">{f.help}</Text> : null}</View>
      <Stepper label={f.label} value={f.value ?? 0} onChange={v => ctx.onValue(f.key, v)} min={f.min} max={f.max} step={f.step} unit={f.unit} />
    </View>
  );
  if (f.kind === 'choice') {
    const cur = f.options.find(o => o.value === f.value);
    return (
      <>
        <ListRow title={f.label} subtitle={f.help ?? cur?.help} value={cur?.label ?? f.value} onPress={() => setPick(true)} />
        <ChoiceSheet visible={pick} onClose={() => setPick(false)} title={f.label} subtitle={f.help} options={f.options.map(o => ({value: o.value, label: o.label, subtitle: o.help}))} value={f.value} onChange={v => ctx.onValue(f.key, v)} />
      </>
    );
  }
  if (f.kind === 'multi') return (
    <View style={{paddingHorizontal: m.isTV ? 22 : 14, paddingVertical: 12, gap: 8}}>
      <Text variant="body">{f.label}</Text>
      {f.help ? <Text variant="caption" tone="tertiary">{f.help}</Text> : null}
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 6}}>{f.options.map(o => { const on = (f.value ?? []).includes(o.value); return <Chip key={o.value} label={o.label} selected={on} onPress={() => ctx.onValue(f.key, on ? (f.value ?? []).filter(x => x !== o.value) : [...(f.value ?? []), o.value])} />; })}</View>
    </View>
  );
  if (f.kind === 'folder') {
    const current = local ? f.value ?? '' : text;
    const set = (v: string) => { setText(v); ctx.onValue(f.key, v); };
    return (
      <View style={{paddingHorizontal: m.isTV ? 22 : 14, paddingVertical: 12, gap: 8}}>
        <TextField label={f.label} help={f.help} placeholder={f.placeholder} value={current} autoCapitalize="none" autoCorrect={false} onChangeText={v => { setText(v); if (local) ctx.onValue(f.key, v); }} onEndEditing={() => !local && ctx.onValue(f.key, text)} />
        <View><Button size="sm" icon="folder" label="Browse the server…" onPress={() => setPick(true)} /></View>
        {pick ? <FolderPicker start={current || undefined} onClose={() => setPick(false)} onChoose={p => { set(p); setPick(false); }} /> : null}
      </View>
    );
  }
  return (
    <View style={{paddingHorizontal: m.isTV ? 22 : 14, paddingVertical: 12}}>
      <TextField label={f.label} help={f.help} placeholder={f.placeholder} value={local ? f.value : text} secureTextEntry={f.secure} multiline={f.multiline} autoCapitalize="none" autoCorrect={false}
        onChangeText={v => { setText(v); if (local) ctx.onValue(f.key, v); }} onEndEditing={() => !local && ctx.onValue(f.key, text)} />
    </View>
  );
}

function Spark({values}: {values: number[]}) {
  const w = 160;
  const h = 34;
  const max = Math.max(...values, 1);
  const d = values.map((v, i) => `${i === 0 ? 'M' : 'L'}${(i / (values.length - 1)) * w} ${h - (v / max) * (h - 4)}`).join(' ');
  return <Svg width="100%" height={h} viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none"><Path d={d} stroke={colors.accentStrong} strokeWidth={2} fill="none" /><Path d={`${d} L${w} ${h} L0 ${h} Z`} fill={colors.accent} fillOpacity={0.12} /></Svg>;
}

function Bars({values, labels, unit}: {values: number[]; labels?: string[]; unit?: string}) {
  const m = useMetrics();
  const h = m.isTV ? 200 : 140;
  const max = Math.max(...values, 1);
  return (
    <View style={{gap: 6}}>
      <Svg width="100%" height={h} viewBox={`0 0 ${values.length * 20} ${h}`} preserveAspectRatio="none">
        {[0.25, 0.5, 0.75].map(g => <Line key={g} x1={0} x2={values.length * 20} y1={h * g} y2={h * g} stroke={colors.lineSoft} strokeWidth={1} />)}
        {values.map((v, i) => <Rect key={i} x={i * 20 + 3} y={h - (v / max) * (h - 8)} width={14} height={(v / max) * (h - 8)} rx={3} fill={i === values.length - 1 ? colors.accentStrong : colors.accentDeep} />)}
      </Svg>
      {labels ? <View style={{flexDirection: 'row'}}>{labels.map((l, i) => <Text key={i} variant="micro" tone="tertiary" style={{flex: 1, textAlign: 'center'}}>{l}</Text>)}</View> : null}
      <Text variant="caption" tone="tertiary">Peak {Math.round(max)} {unit} · today {Math.round(values[values.length - 1]!)} {unit}</Text>
    </View>
  );
}

export type {Ctx};
export {Focusable};

/** Browses folders on the server so an owner can pick one instead of typing a path. */
export function FolderPicker({start, onClose, onChoose}: {start?: string; onClose: () => void; onChoose: (path: string) => void}) {
  const admin = useAdmin();
  const m = useMetrics();
  const [path, setPath] = useState(start);
  const [listing, setListing] = useState<FolderListing>();
  useEffect(() => {
    let alive = true;
    void admin.folders(path).then(l => alive && setListing(l));
    return () => { alive = false; };
  }, [admin, path]);
  return (
    <Sheet visible onClose={onClose} title="Choose a folder" subtitle={listing ? `On the server · ${listing.path}` : 'On the server'} size="md"
      footer={<><Button label="Cancel" variant="ghost" onPress={onClose} /><Button label="Use this folder" variant="primary" disabled={!listing || listing.path === '/'} onPress={() => listing && onChoose(listing.path)} hasTVPreferredFocus /></>}>
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 6}}>
        {listing?.roots.map(r => <Chip key={r.path} label={r.path} selected={listing.path === r.path} onPress={() => setPath(r.path)} />)}
      </View>
      <View style={{gap: 2}}>
        {listing?.parent !== undefined ? <ListRow icon="back" title="Up one level" subtitle={listing.parent} chevron={false} onPress={() => setPath(listing.parent)} /> : null}
        {listing?.entries.map(e => (
          <ListRow key={e.path} icon="folder" title={e.name} subtitle={e.path} disabled={!e.access} badge={undefined}
            trailing={!e.access ? <Badge label="No access" tone="bad" /> : e.link ? <Badge label="Link" /> : undefined}
            onPress={() => setPath(e.path)} />
        ))}
        {listing && !listing.entries.length ? <Text variant="caption" tone="tertiary" style={{paddingHorizontal: m.isTV ? 22 : 12, paddingVertical: 10}}>No folders inside this one.</Text> : null}
      </View>
    </Sheet>
  );
}
