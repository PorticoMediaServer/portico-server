import React, {useEffect, useState} from 'react';
import {Link} from '@tanstack/react-router';
import {useAdmin} from '@/data/DataProvider';
import type {AdminAction, AdminField, Block, FolderListing, ListItem, Tone} from '@/data/adminTypes';
import {Button, IconButton} from '@/ui/Button';
import {Artwork, Badge, Meter, Progress, Spinner} from '@/ui/Bits';
import {Checkbox, RadioCards, Segmented, Select, Switch, TextField} from '@/ui/Form';
import {Dialog, useToast} from '@/ui/Overlay';
import {Icon, iconNames, type IconName} from '@/ui/Icon';
import {cx} from '@/ui/cx';
import s from './Server.module.css';

export type Ctx = {onAction: (a: AdminAction) => void; onValue: (key: string, v: unknown) => void; href: (section: string, param?: string) => string};
const icon = (name?: string): IconName => (name && (iconNames as string[]).includes(name) ? (name as IconName) : 'dot');
const variantFor = (tone?: AdminAction['tone']) => (tone === 'primary' ? 'primary' : tone === 'danger' ? 'danger' : tone === 'ghost' ? 'ghost' : 'secondary');

export function ActionButton({a, ctx, size = 'sm'}: {a: AdminAction; ctx: Ctx; size?: 'sm' | 'md'}) {
  if (a.navigate) return <Button size={size} variant={variantFor(a.tone)} icon={a.icon ? icon(a.icon) : undefined} label={a.label} to={ctx.href(a.navigate.section, a.navigate.param)} />;
  return <Button size={size} variant={variantFor(a.tone)} icon={a.icon ? icon(a.icon) : undefined} label={a.label} onClick={() => ctx.onAction(a)} />;
}

function BlockFrame({title, lede, right, children}: {title?: string; lede?: string; right?: React.ReactNode; children: React.ReactNode}) {
  return (
    <section className={s.block}>
      {title || lede || right ? <div className={s.blockHead}><div style={{flex: '1 1 300px', minWidth: 0}}>{title ? <h2 className={s.blockTitle}>{title}</h2> : null}{lede ? <p className={s.blockLede}>{lede}</p> : null}</div>{right}</div> : null}
      {children}
    </section>
  );
}

function Spark({values, tone}: {values: number[]; tone?: Tone}) {
  const max = Math.max(...values, 1);
  const pts = values.map((v, i) => `${(i / (values.length - 1)) * 84},${30 - (v / max) * 26 - 2}`).join(' ');
  const color = tone === 'warn' ? 'var(--amber)' : tone === 'bad' ? 'var(--record)' : 'var(--accent)';
  return <svg className={s.spark} viewBox="0 0 84 30" aria-hidden><polyline points={`0,30 ${pts} 84,30`} fill={color} opacity="0.14" stroke="none" /><polyline points={pts} fill="none" stroke={color} strokeWidth="1.8" strokeLinejoin="round" strokeLinecap="round" /></svg>;
}

export function BlockView({b, ctx}: {b: Block; ctx: Ctx}) {
  switch (b.type) {
    case 'stats':
      return <div className={s.stats}>{b.items.map(it => <div key={it.label} className={cx(s.stat, it.tone && s[`statTone-${it.tone}`], it.spark && s.statSpark)}><span className={s.statLabel}>{it.label}</span><span className={s.statValue}>{it.value}</span>{it.sub ? <span className={s.statSub}>{it.sub}</span> : null}{it.spark ? <Spark values={it.spark} tone={it.tone} /> : null}</div>)}</div>;
    case 'notice':
      return (
        <div className={cx(s.notice, s[`notice-${b.tone}`])} role={b.tone === 'bad' ? 'alert' : 'status'}>
          <Icon name={b.tone === 'good' ? 'check' : b.tone === 'bad' || b.tone === 'warn' ? 'alert' : 'info'} size={20} className={s.noticeIcon} />
          <div style={{flex: 1, minWidth: 0}}><div className="t-strong">{b.title}</div><p className="c-2" style={{fontSize: 14, marginTop: 2}}>{b.body}</p>{b.actions?.length ? <div className={s.actions} style={{marginTop: 10}}>{b.actions.map(a => <ActionButton key={a.id + a.label} a={a} ctx={ctx} />)}</div> : null}</div>
        </div>
      );
    case 'settings':
      return <BlockFrame title={b.title} lede={b.lede}><div className={s.card}>{b.fields.map(f => <FieldRow key={f.key} f={f} onValue={ctx.onValue} />)}</div></BlockFrame>;
    case 'facts':
      return (
        <BlockFrame title={b.title} lede={b.lede}>
          <div className={cx(s.card, s.facts)}>{b.rows.map(r => <FactRow key={r.label} label={r.label} value={r.value} tone={r.tone} copy={r.copy} />)}</div>
        </BlockFrame>
      );
    case 'list':
      return (
        <BlockFrame title={b.title} lede={b.lede} right={<>{b.filters ? <Segmented label={b.title ?? 'Filter'} value={b.filters.value} onChange={v => ctx.onValue(b.filters!.key, v)} options={b.filters.options} /> : null}{b.actions?.length ? <div className={s.actions}>{b.actions.map(a => <ActionButton key={a.id + a.label} a={a} ctx={ctx} />)}</div> : null}</>}>
          <div className={s.card}>
            {b.items.length ? b.items.map(it => <ItemRow key={it.id} it={it} ctx={ctx} />) : <div style={{padding: '28px 16px', textAlign: 'center'}}><div className="t-strong">{b.empty?.title ?? 'Nothing here'}</div>{b.empty?.body ? <p className="c-3 t-caption" style={{marginTop: 4}}>{b.empty.body}</p> : null}</div>}
          </div>
        </BlockFrame>
      );
    case 'table':
      return (
        <BlockFrame title={b.title} lede={b.lede}>
          <div className={cx(s.card, s.tableWrap)}><table className={s.table}><thead><tr>{b.columns.map(c => <th key={c} scope="col">{c}</th>)}</tr></thead><tbody>{b.rows.map((r, i) => <tr key={i}>{r.map((c, j) => <td key={j}>{c}</td>)}</tr>)}</tbody></table></div>
        </BlockFrame>
      );
    case 'chart': return <Chart b={b} />;
    case 'meters':
      return (
        <BlockFrame title={b.title}>
          <div className={cx(s.card, s.meters)}>{b.items.map(m => <div key={m.label}><div className={s.meterHead}><span className="t-strong">{m.label}</span><span>{m.detail}</span></div><Meter value={m.value} tone={m.tone === 'bad' ? 'bad' : m.tone === 'warn' ? 'warn' : m.tone === 'good' ? 'good' : undefined} label={m.label} /></div>)}</div>
        </BlockFrame>
      );
    case 'actions':
      return <BlockFrame title={b.title} lede={b.lede}><div className={s.actions}>{b.actions.map(a => <ActionButton key={a.id + a.label} a={a} ctx={ctx} size="md" />)}</div></BlockFrame>;
    case 'timeline': return <Timeline b={b} />;
  }
}

function FactRow({label, value, tone, copy}: {label: string; value: string; tone?: Tone; copy?: boolean}) {
  const toast = useToast();
  return (
    <div className={s.fact}>
      <span className={s.factLabel}>{label}</span>
      <span className={s.factValue} style={tone === 'good' ? {color: 'var(--healthy)'} : tone === 'warn' ? {color: 'var(--amber)'} : tone === 'bad' ? {color: 'var(--danger)'} : undefined}>{value}</span>
      {copy ? <IconButton size="sm" variant="ghost" icon="copy" label={`Copy ${label}`} onClick={() => { void navigator.clipboard?.writeText(value).catch(() => {}); toast.show(`${label} copied`, {icon: 'copy'}); }} /> : <span />}
    </div>
  );
}

function ItemRow({it, ctx}: {it: ListItem; ctx: Ctx}) {
  const [open, setOpen] = useState(false);
  const title = it.open ? <Link to={ctx.href(it.open.section, it.open.param) as never}>{it.title}</Link> : it.title;
  return (
    <div className={s.item}>
      {it.image ? <Artwork src={it.image} title={it.title} shape={it.round ? 'circle' : 'poster'} className={s.itemImage} /> : <span className={cx(s.itemIcon, it.round && s.itemRound)}><Icon name={icon(it.icon)} size={18} /></span>}
      <div style={{minWidth: 0}}>
        <div className={s.itemTitle}>{title}{it.badge ? <Badge tone={it.badge.tone}>{it.badge.label}</Badge> : null}</div>
        {it.subtitle ? <p className={s.itemSub}>{it.subtitle}</p> : null}
        {it.progress !== undefined ? <Progress value={it.progress} className={s.itemProgress} /> : null}
      </div>
      <div className={s.itemRight}>
        {it.meta ? <span className={s.itemMeta}>{it.meta}</span> : null}
        {it.actions?.map(a => <ActionButton key={a.id + a.label} a={{...a, target: a.target ?? it.id}} ctx={ctx} />)}
        {it.detail?.length ? <IconButton size="sm" variant="ghost" icon={open ? 'chevronUp' : 'chevronDown'} label={open ? 'Hide details' : 'Show details'} onClick={() => setOpen(!open)} /> : null}
        {it.open && !it.actions?.length ? <Link to={ctx.href(it.open.section, it.open.param) as never} aria-label={`Open ${it.title}`} className="c-3" style={{display: 'flex'}}><Icon name="forward" size={16} /></Link> : null}
      </div>
      {open && it.detail ? <dl className={s.itemDetail}>{it.detail.map(d => <div key={d.label}><dt>{d.label}</dt><dd>{d.value}</dd></div>)}</dl> : null}
    </div>
  );
}

function Chart({b}: {b: Extract<Block, {type: 'chart'}>}) {
  const all = b.series.flatMap(x => x.values);
  const max = Math.max(...all, 1);
  const n = b.series[0]?.values.length ?? 0;
  const W = 600, H = 160, gap = 4;
  const bw = (W - gap * (n - 1)) / n / b.series.length;
  return (
    <BlockFrame title={b.title} lede={b.lede} right={b.series.length > 1 ? <span className="t-caption c-3" style={{display: 'flex', gap: 12}}>{b.series.map((x, i) => <span key={x.label} style={{display: 'inline-flex', alignItems: 'center', gap: 6}}><i style={{width: 10, height: 10, borderRadius: 3, background: i ? 'var(--healthy)' : 'var(--accent)', display: 'inline-block'}} />{x.label}</span>)}</span> : undefined}>
      <div className={cx(s.card, s.chart)}>
        <svg className={s.chartSvg} viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" role="img" aria-label={`${b.title}: ${b.series.map(x => `${x.label} from ${Math.round(Math.min(...x.values))} to ${Math.round(Math.max(...x.values))}${b.unit ?? ''}`).join('; ')}`}>
          {[0.25, 0.5, 0.75].map(f => <line key={f} x1="0" x2={W} y1={H * f} y2={H * f} stroke="var(--line-soft)" strokeWidth="1" vectorEffect="non-scaling-stroke" />)}
          {b.series.map((ser, si) => ser.values.map((v, i) => {
            const h = Math.max(2, (v / max) * (H - 8));
            return <rect key={`${si}-${i}`} x={i * ((W - gap * (n - 1)) / n + gap) + si * bw} y={H - h} width={Math.max(2, bw - 1)} height={h} rx="3" fill={si ? 'var(--healthy)' : ser.tone === 'warn' ? 'var(--amber)' : 'var(--accent)'} opacity={i === n - 1 ? 1 : 0.78}><title>{`${b.labels?.[i] ?? i + 1}: ${Math.round(v * 10) / 10}${b.unit ?? ''}`}</title></rect>;
          }))}
        </svg>
        {b.labels ? <div className={s.chartLabels} aria-hidden>{b.labels.map((l, i) => <span key={i}>{l}</span>)}</div> : null}
      </div>
    </BlockFrame>
  );
}

function Timeline({b}: {b: Extract<Block, {type: 'timeline'}>}) {
  const span = b.to - b.from;
  const hours = Array.from({length: 5}, (_, i) => Math.round(b.from + (span * i) / 4));
  const fmt = (h: number) => { const x = ((h % 24) + 24) % 24; return x === 0 ? '12 am' : x < 12 ? `${x} am` : x === 12 ? '12 pm' : `${x - 12} pm`; };
  return (
    <BlockFrame title={b.title} lede={b.lede}>
      <div className={cx(s.card, s.timeline)}>
        <div style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 8}}>
          {b.lanes.map(l => (
            <div key={l.label} className={s.lane}>
              <span className="t-caption c-2" style={{fontWeight: 650}}>{l.label}</span>
              <div className={s.laneTrack}>{l.spans.map((sp, i) => <span key={i} className={cx(s.span, s[`span-${sp.tone}`])} style={{left: `${((sp.start - b.from) / span) * 100}%`, width: `${((sp.end - sp.start) / span) * 100}%`}} title={sp.label}>{sp.label}</span>)}</div>
            </div>
          ))}
        </div>
        <div className={s.hours}><span /><div>{hours.map(h => <span key={h}>{fmt(h)}</span>)}</div></div>
      </div>
    </BlockFrame>
  );
}

/** One setting. Saved as soon as it changes; forms use the same control with local state. */
export function FieldRow({f, onValue, stacked}: {f: AdminField; onValue: (key: string, v: unknown) => void; stacked?: boolean}) {
  const [text, setText] = useState(f.kind === 'text' || f.kind === 'folder' ? f.value ?? '' : '');
  const [num, setNum] = useState(f.kind === 'number' ? f.value ?? 0 : 0);
  const [picking, setPicking] = useState(false);
  useEffect(() => { if (f.kind === 'number') setNum(f.value ?? 0); }, [f]);
  const label = <div><div className={s.fieldLabel}>{f.label}</div>{f.help ? <p className={s.fieldHelp}>{f.help}</p> : null}</div>;
  switch (f.kind) {
    case 'toggle': return <label className={cx(s.field, stacked && s.stacked)}>{label}<span className={s.fieldControl}><Switch checked={!!f.value} onChange={v => onValue(f.key, v)} label={f.label} /></span></label>;
    case 'number': return (
      <div className={cx(s.field, stacked && s.stacked)}>{label}<span className={s.fieldControl}>
        <span className={s.stepper}>
          <IconButton size="sm" variant="ghost" icon="minus" label={`Less ${f.label.toLowerCase()}`} disabled={f.min !== undefined && num <= f.min} onClick={() => { const v = Math.max(f.min ?? -Infinity, num - (f.step ?? 1)); setNum(v); onValue(f.key, v); }} />
          <input aria-label={f.label} inputMode="numeric" value={num} onChange={e => setNum(Number(e.target.value.replace(/[^\d.]/g, '')) || 0)} onBlur={() => onValue(f.key, Math.min(f.max ?? Infinity, Math.max(f.min ?? -Infinity, num)))} />
          {f.unit ? <span className={s.unit}>{f.unit}</span> : null}
          <IconButton size="sm" variant="ghost" icon="plus" label={`More ${f.label.toLowerCase()}`} disabled={f.max !== undefined && num >= f.max} onClick={() => { const v = Math.min(f.max ?? Infinity, num + (f.step ?? 1)); setNum(v); onValue(f.key, v); }} />
        </span>
      </span></div>
    );
    case 'choice':
      if (f.options.length <= 4 && f.options.some(o => o.help)) return <div className={cx(s.field, s.fieldWide, stacked && s.stacked)}>{label}<RadioCards label={f.label} value={f.value ?? ''} onChange={v => onValue(f.key, v)} options={f.options} /></div>;
      return <div className={cx(s.field, stacked && s.stacked)}>{label}<span className={s.fieldControl}><Select compact={!stacked} ariaLabel={f.label} value={f.value ?? ''} onChange={v => onValue(f.key, v)} options={f.options} /></span>{f.options.find(o => o.value === f.value)?.help ? <p className={s.fieldHelp} style={{gridColumn: '1 / -1', marginTop: -4}}>{f.options.find(o => o.value === f.value)!.help}</p> : null}</div>;
    case 'multi': return <div className={cx(s.field, s.fieldWide, stacked && s.stacked)}>{label}<div style={{display: 'flex', flexWrap: 'wrap', gap: '4px 18px'}}>{f.options.map(o => <Checkbox key={o.value} checked={(f.value ?? []).includes(o.value)} onChange={on => onValue(f.key, on ? [...(f.value ?? []), o.value] : (f.value ?? []).filter(x => x !== o.value))} label={o.label} />)}</div></div>;
    case 'folder': return (
      <div className={cx(s.field, stacked && s.stacked)}>{label}<span className={s.fieldControl}><TextField compact={!stacked} value={text} onChange={v => { setText(v); onValue(f.key, v); }} placeholder={f.placeholder} aria-label={f.label} className="grow" /><Button size={stacked ? 'md' : 'sm'} variant="secondary" icon="folder" label="Browse…" onClick={() => setPicking(true)} /></span>
        {picking ? <FolderPicker start={text || '/'} onClose={() => setPicking(false)} onPick={p => { setText(p); onValue(f.key, p); setPicking(false); }} /> : null}
      </div>
    );
    default: return <div className={cx(s.field, (f as {multiline?: boolean}).multiline && s.fieldWide, stacked && s.stacked)}>{label}<span className={s.fieldControl} style={{flex: 1}}><TextField compact={!stacked && !(f as {multiline?: boolean}).multiline} multiline={(f as {multiline?: boolean}).multiline} type={(f as {secure?: boolean}).secure ? 'password' : 'text'} value={text} onChange={v => { setText(v); onValue(f.key, v); }} placeholder={(f as {placeholder?: string}).placeholder} aria-label={f.label} className="grow" /></span></div>;
  }
}

/** Browse the server's folders to choose one (libraries, recordings, backups). */
export function FolderPicker({start, onPick, onClose}: {start: string; onPick: (path: string) => void; onClose: () => void}) {
  const admin = useAdmin();
  const [path, setPath] = useState(start);
  const [listing, setListing] = useState<FolderListing>();
  useEffect(() => { let alive = true; setListing(undefined); void admin.folders(path).then(l => alive && setListing(l)); return () => { alive = false; }; }, [admin, path]);
  const parts = (listing?.path ?? path).split('/').filter(Boolean);
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title="Choose a folder" description="Folders on the server. Portico needs permission to read the folder you choose." width={520} footer={<><Button variant="ghost" label="Cancel" onClick={onClose} /><Button variant="primary" label={`Use ${listing?.path ?? path}`} disabled={!listing} onClick={() => onPick(listing!.path)} /></>}>
      <div className={s.crumbs} aria-label="Folder path">
        <button type="button" onClick={() => setPath('/')}>Server</button>
        {parts.map((p, i) => <React.Fragment key={i}><Icon name="forward" size={12} className="c-3" /><button type="button" onClick={() => setPath(`/${parts.slice(0, i + 1).join('/')}`)}>{p}</button></React.Fragment>)}
      </div>
      {!listing ? <div style={{padding: 24, display: 'flex', justifyContent: 'center'}}><Spinner /></div> : (
        <div className={s.folders}>
          {listing.parent !== undefined ? <button type="button" className={s.folder} onClick={() => setPath(listing.parent!)}><Icon name="back" size={18} className="c-3" />Up one level</button> : null}
          {listing.entries.length ? listing.entries.map(e => (
            <button key={e.path} type="button" className={s.folder} disabled={!e.access} onClick={() => setPath(e.path)}>
              <Icon name="folder" size={18} style={{color: 'var(--accent-strong)'}} /><span style={{flex: 1}}>{e.name}</span>
              {e.link ? <Badge>Shortcut</Badge> : null}{!e.access ? <Badge tone="warn">No permission</Badge> : <Icon name="forward" size={14} className="c-3" />}
            </button>
          )) : <p className="c-3 t-caption" style={{padding: 12}}>This folder has no folders inside it.</p>}
        </div>
      )}
    </Dialog>
  );
}
