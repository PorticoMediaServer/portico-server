import {useEffect, useState} from 'react';
import {useClient, useQuery} from '@/data/DataProvider';
import type {HomeLayoutRow} from '@/data/types';
import {usePrefs} from '@/app/prefs';
import {Button, IconButton} from '@/ui/Button';
import {Checkbox, Switch} from '@/ui/Form';
import {ConfirmDialog, Dialog, useToast} from '@/ui/Overlay';
import {Icon} from '@/ui/Icon';
import {Skeleton} from '@/ui/Bits';
import s from './Home.module.css';

/** Show, hide and reorder Home rows (drag, or the arrow buttons), plus the featured billboard. Saved per profile. */
export function CustomizeHome({open, onOpenChange}: {open: boolean; onOpenChange: (o: boolean) => void}) {
  const client = useClient();
  const toast = useToast();
  const {prefs, update} = usePrefs();
  const layout = useQuery(c => (open ? c.home.layout() : Promise.resolve(undefined)), [open]);
  const [rows, setRows] = useState<HomeLayoutRow[]>([]);
  const [drag, setDrag] = useState<number>();
  const [busy, setBusy] = useState(false);
  const [reset, setReset] = useState(false);
  useEffect(() => { if (layout.data) setRows(layout.data.map(r => ({...r}))); }, [layout.data]);
  const move = (i: number, d: number) => setRows(list => { const j = i + d; if (j < 0 || j >= list.length) return list; const n = [...list]; [n[i], n[j]] = [n[j]!, n[i]!]; return n; });
  const save = async () => { setBusy(true); await client.home.saveLayout(rows); setBusy(false); onOpenChange(false); toast.show('Home updated', {icon: 'check'}); };
  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange} title="Customize Home" description="Choose the rows on your Home and their order. Only this profile sees these changes." width={560} footerSplit footer={<>
        <Button variant="ghost" icon="refresh" label="Reset to default" onClick={() => setReset(true)} />
        <span style={{display: 'flex', gap: 8}}><Button variant="ghost" label="Cancel" onClick={() => onOpenChange(false)} /><Button variant="primary" label="Save" busy={busy} onClick={() => void save()} /></span>
      </>}>
        <label style={{display: 'flex', alignItems: 'center', gap: 12, padding: '4px 0 14px', borderBottom: '1px solid var(--line-soft)', marginBottom: 10}}>
          <Switch checked={prefs.homeHero} onChange={v => update({homeHero: v})} label="Featured billboard" />
          <span><span className="t-strong">Featured billboard</span><span className="t-caption c-3" style={{display: 'block'}}>The large rotating picture at the top of Home</span></span>
        </label>
        {!layout.data ? <div style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 8}}>{Array.from({length: 8}, (_, i) => <Skeleton key={i} height={40} />)}</div> : (
          <div className={s.custom} role="list" aria-label="Home rows">
            {rows.map((r, i) => (
              <div key={r.id} role="listitem" className={`${s.customRow} ${drag === i ? s.dragging : ''}`} data-hidden={r.hidden} draggable onDragStart={() => setDrag(i)} onDragEnd={() => setDrag(undefined)} onDragOver={e => { e.preventDefault(); if (drag !== undefined && drag !== i) { move(drag, i - drag); setDrag(i); } }}>
                <Icon name="menu" size={18} className={s.grip} />
                <span className={s.customTitle}>{r.title}{r.savedView ? <span className="t-caption c-3"> · Saved view</span> : null}</span>
                {r.required ? <span className="t-caption c-3">Always shown</span> : <Checkbox checked={!r.hidden} onChange={v => setRows(list => list.map(x => (x.id === r.id ? {...x, hidden: !v} : x)))} label={<span className="sr-only">Show {r.title}</span>} />}
                <IconButton size="sm" variant="ghost" icon="chevronUp" label={`Move ${r.title} up`} disabled={i === 0} onClick={() => move(i, -1)} />
                <IconButton size="sm" variant="ghost" icon="chevronDown" label={`Move ${r.title} down`} disabled={i === rows.length - 1} onClick={() => move(i, 1)} />
              </div>
            ))}
          </div>
        )}
      </Dialog>
      <ConfirmDialog open={reset} onOpenChange={setReset} title="Reset Home?" body="Home goes back to Portico’s default rows and order. Saved views you pinned to Home are unpinned." action="Reset" onConfirm={async () => { await client.home.resetLayout(); update({homeHero: true}); setReset(false); layout.refresh(); toast.show('Home reset to default', {icon: 'check'}); }} />
    </>
  );
}
