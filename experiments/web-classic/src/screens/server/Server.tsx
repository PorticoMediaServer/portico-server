import React, {useCallback, useEffect, useState} from 'react';
import {Link, useNavigate} from '@tanstack/react-router';
import {useAdmin} from '@/data/DataProvider';
import type {AdminAction, AdminPage} from '@/data/adminTypes';
import {useSession} from '@/app/session';
import {Button} from '@/ui/Button';
import {EmptyState, ErrorState, Skeleton} from '@/ui/Bits';
import {ConfirmDialog, Dialog, useToast} from '@/ui/Overlay';
import {Select} from '@/ui/Form';
import {Icon, iconNames, type IconName} from '@/ui/Icon';
import {useTitle} from '@/ui/hooks';
import {useRouteParams} from '../common';
import {ActionButton, BlockView, FieldRow, type Ctx} from './Blocks';
import {useDemo} from '@/app/demo';
import {FolderPicker} from './Blocks';
import s from './Server.module.css';

const icon = (name: string): IconName => ((iconNames as string[]).includes(name) ? (name as IconName) : 'dot');
const GROUPS = ['Console', 'Media', 'Live TV', 'Server'] as const;

/**
 * Manage server (owners and administrators). Every section is a page of
 * typed blocks from the admin service: stats, settings that save as they
 * change, lists with actions, tables, charts and timelines. Actions open a
 * form or a confirmation when they need one.
 */
export default function ServerScreen() {
  const {section = 'overview', id} = useRouteParams<{section?: string; id?: string}>();
  const admin = useAdmin();
  const session = useSession();
  const navigate = useNavigate();
  const toast = useToast();
  const [page, setPage] = useState<AdminPage>();
  const [error, setError] = useState<string>();
  const [form, setForm] = useState<{a: AdminAction; values: Record<string, unknown>}>();
  const [confirm, setConfirm] = useState<AdminAction>();
  const [busy, setBusy] = useState(false);
  const [picker, setPicker] = useState(false);
  useDemo('add-library', () => { const a = page?.actions?.find(x => x.form); if (a) setForm({a, values: Object.fromEntries(a.form!.fields.map(f => [f.key, (f as {value?: unknown}).value]))}); }, !!page);
  useDemo('folder-picker', () => setPicker(true), !!page);
  const meta = admin.sections.find(x => x.id === section);
  useTitle(page?.title ?? meta?.label ?? 'Manage server');
  const load = useCallback(() => { admin.load(section, id).then(p => { setPage(p); setError(undefined); }, e => setError(e instanceof Error ? e.message : String(e))); }, [admin, section, id]);
  useEffect(() => { setPage(undefined); load(); }, [load]);
  useEffect(() => admin.subscribe(load), [admin, load]);
  if (!session.isAdmin) return <EmptyState icon="lock" title="Only the server’s owner and administrators can manage it" body="Ask the owner if you need a change to the server." actions={[{label: 'Go to Home', to: '/'}]} />;
  if (!meta) return <EmptyState icon="console" title="That section doesn’t exist" actions={[{label: 'Overview', to: '/server/overview'}]} />;
  const href = (sec: string, param?: string) => (param ? `/server/${sec}/${encodeURIComponent(param)}` : `/server/${sec}`);
  const run = async (a: AdminAction, values?: Record<string, unknown>) => {
    setBusy(true);
    try { const msg = await admin.act(a.id, a.target, values); toast.show(msg ?? `${a.label}: done`, {icon: 'check'}); } catch (e) { toast.show(e instanceof Error ? e.message : 'That didn’t work.', {tone: 'error', icon: 'alert'}); } finally { setBusy(false); }
  };
  const ctx: Ctx = {
    href,
    onAction: a => {
      if (a.navigate) { void navigate({to: href(a.navigate.section, a.navigate.param) as never}); return; }
      if (a.confirm) { setConfirm(a); return; }
      if (a.form) { setForm({a, values: Object.fromEntries(a.form.fields.map(f => [f.key, (f as {value?: unknown}).value]))}); return; }
      void run(a);
    },
    onValue: (key, v) => { void admin.setValue(key, v).then(() => toast.show('Saved', {icon: 'check', duration: 1500})); },
  };
  return (
    <div className={s.layout}>
      <nav className={s.nav} aria-label="Manage server">
        <div className={s.navTitle}><Icon name="server" size={18} />{session.server?.name}</div>
        {GROUPS.map(g => (
          <React.Fragment key={g}>
            <div className={s.navGroup}>{g}</div>
            {admin.sections.filter(x => x.group === g).map(x => <Link key={x.id} to={href(x.id) as never} className={s.navItem} aria-current={x.id === section ? 'page' : undefined} activeProps={{}}><Icon name={icon(x.icon)} size={17} />{x.label}</Link>)}
          </React.Fragment>
        ))}
      </nav>
      <div className={s.main}>
        <div className={s.picker}><Select label="Section" value={section} onChange={v => void navigate({to: href(v) as never})} options={admin.sections.map(x => ({value: x.id, label: `${x.group} · ${x.label}`}))} /></div>
        <header className={s.head}>
          <div className={s.headText}>
            <div className={s.eyebrow}>{page?.back ? <Link to={href(page.back.section) as never} style={{display: 'inline-flex', alignItems: 'center', gap: 4}}><Icon name="back" size={14} />{page.back.label}</Link> : <>Manage server · {meta.group}</>}</div>
            <h1 className="t-title">{page?.title ?? meta.label}</h1>
            <p className={s.lede}>{page?.lede ?? meta.lede}</p>
          </div>
          {page?.actions?.length ? <div style={{display: 'flex', gap: 8, flexWrap: 'wrap'}}>{page.actions.map(a => <ActionButton key={a.id + a.label} a={a} ctx={ctx} size="md" />)}</div> : null}
        </header>
        {error ? <ErrorState body={error} onRetry={load} /> : !page ? <><Skeleton height={112} /><Skeleton height={220} /><Skeleton height={180} /></> : page.blocks.map((b, i) => <BlockView key={`${section}-${id ?? ''}-${i}`} b={b} ctx={ctx} />)}
      </div>
      {form ? (
        <Dialog open onOpenChange={o => !o && setForm(undefined)} title={form.a.form!.title} description={form.a.form!.lede} width={560} footer={<><Button variant="ghost" label="Cancel" onClick={() => setForm(undefined)} /><Button variant={form.a.tone === 'danger' ? 'dangerSolid' : 'primary'} label={form.a.form!.submit} busy={busy} onClick={async () => { await run(form.a, form.values); setForm(undefined); }} /></>}>
          <div style={{margin: '-4px -24px 0'}}>{form.a.form!.fields.map(f => <FieldRow key={f.key} stacked f={{...f, value: form.values[f.key]} as never} onValue={(k, v) => setForm(x => (x ? {...x, values: {...x.values, [k]: v}} : x))} />)}</div>
        </Dialog>
      ) : null}
      {picker ? <FolderPicker start="/media" onClose={() => setPicker(false)} onPick={p => { setPicker(false); toast.show(`Chose ${p}`, {icon: 'folder'}); }} /> : null}
      {confirm ? <ConfirmDialog open onOpenChange={o => !o && setConfirm(undefined)} title={confirm.confirm!.title} body={confirm.confirm!.body} action={confirm.confirm!.action} tone={confirm.tone === 'danger' ? 'danger' : 'primary'} typeToConfirm={confirm.confirm!.typeToConfirm} onConfirm={async () => { await run(confirm); setConfirm(undefined); }} /> : null}
    </div>
  );
}
