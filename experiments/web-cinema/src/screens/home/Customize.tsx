import {useEffect, useState} from 'react';
import {useQuery, useServices} from '../../data/hooks';
import type {HomeLayoutEntry, Row} from '../../data/types';
import {Dialog, ConfirmDialog, DialogClose} from '../../ui/Overlay';
import {Button} from '../../ui/Button';
import {Switch} from '../../ui/Controls';
import {Skeleton, toast} from '../../ui/Feedback';
import {Icon} from '../../ui/Icon';
import {cx} from '../../ui/cx';

/** Customize Home: show or hide rows, reorder them, add a saved view as a row, reset. */
export function CustomizeHome({onClose}: {onClose: () => void}) {
  const services = useServices();
  const layout = useQuery('home.layout', () => services.catalog.homeLayout());
  const rows = useQuery('home.rows.all', async () => {
    const all = await Promise.all((await services.catalog.homeLayout()).map(l => services.catalog.row(l.rowId)));
    return all.filter(Boolean) as Row[];
  });
  const views = useQuery('lists.views', () => services.lists.savedViews());
  const [draft, setDraft] = useState<HomeLayoutEntry[] | null>(null);
  const [resetting, setResetting] = useState(false);
  const [busy, setBusy] = useState(false);
  useEffect(() => { if (layout.data && !draft) setDraft(layout.data); }, [layout.data, draft]);
  const byId = new Map((rows.data ?? []).map(r => [r.id, r]));
  const name = (id: string) => {
    const r = byId.get(id);
    if (!r) { const v = views.data?.find(x => 'view-' + x.id === id); return v ? v.title : id; }
    return r.subject ? `${r.title} ${r.subject}` : r.title;
  };
  const move = (i: number, d: number) => setDraft(l => { if (!l) return l; const n = [...l]; const [x] = n.splice(i, 1); n.splice(i + d, 0, x!); return n; });
  const onHome = new Set((draft ?? []).map(d => d.rowId));
  const addable = (views.data ?? []).filter(v => !onHome.has('view-' + v.id));
  const save = async () => {
    if (!draft) return;
    setBusy(true);
    await services.catalog.setHomeLayout(draft);
    setBusy(false);
    toast('Home updated');
    onClose();
  };
  return (
    <Dialog open onOpenChange={o => !o && onClose()} kind="sheet" size="md" title="Customize Home" description="Choose which rows appear on Home and in what order." footer={
      <>
        <Button variant="ghost" label="Reset to default" onClick={() => setResetting(true)} />
        <span style={{flex: 1}} />
        <DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose>
        <Button variant="primary" label="Save" busy={busy} onClick={() => void save()} />
      </>
    }>
      {!draft ? <div className="stack">{Array.from({length: 8}, (_, i) => <Skeleton key={i} h={52} />)}</div> : (
        <ol className="cust-list">
          {draft.map((e, i) => (
            <li key={e.rowId} className={cx('cust-row', !e.visible && 'is-off')}>
              <span className="cust-grip" aria-hidden><Icon name="grip" size={16} /></span>
              <span className="cust-name">{name(e.rowId)}{e.alwaysOn ? <span className="cust-always">Always on Home</span> : null}</span>
              <span className="cust-tools">
                <button type="button" className="track-more is-shown" disabled={i === 0} onClick={() => move(i, -1)} aria-label={`Move ${name(e.rowId)} up`}><Icon name="chevronUp" size={16} /></button>
                <button type="button" className="track-more is-shown" disabled={i === draft.length - 1} onClick={() => move(i, 1)} aria-label={`Move ${name(e.rowId)} down`}><Icon name="chevronDown" size={16} /></button>
                {e.rowId.startsWith('view-') ? <button type="button" className="track-more is-shown" onClick={() => setDraft(draft.filter(x => x.rowId !== e.rowId))} aria-label={`Remove ${name(e.rowId)} from Home`}><Icon name="close" size={16} /></button> : null}
                <Switch checked={e.visible} disabled={e.alwaysOn} onChange={v => setDraft(draft.map(x => (x.rowId === e.rowId ? {...x, visible: v} : x)))} label={`Show ${name(e.rowId)} on Home`} />
              </span>
            </li>
          ))}
        </ol>
      )}
      <section className="cust-add">
        <h3 className="eyebrow">Add a saved view</h3>
        {addable.length ? (
          <div className="chip-row" style={{marginTop: 10}}>{addable.map(v => <button key={v.id} type="button" className="chip" onClick={() => setDraft([...(draft ?? []), {rowId: 'view-' + v.id, visible: true}])}><Icon name="plus" size={14} />{v.title}</button>)}</div>
        ) : <p className="faint small" style={{marginTop: 8}}>No saved views to add. Save one from a library’s filters.</p>}
      </section>
      <ConfirmDialog open={resetting} onOpenChange={setResetting} title="Reset Home to the default layout?" body="Your row order and hidden rows are cleared." confirmLabel="Reset" onConfirm={async () => { await services.catalog.setHomeLayout(null as never); setDraft(null); layout.reload(); toast('Home reset'); }} />
    </Dialog>
  );
}
