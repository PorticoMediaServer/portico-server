import {useEffect, useState} from 'react';
import {useDataVersion, useQuery, useServices} from '../../data/hooks';
import type {DeviceSession, Profile, ProfileLimits} from '../../data/types';
import {useSession} from '../../app/session';
import {Panel, Row} from '../../ui/Layout';
import {Button} from '../../ui/Button';
import {Checkbox, Field, Input, PasswordInput, Select, Switch} from '../../ui/Controls';
import {Badge, Notice, Skeleton, toast} from '../../ui/Feedback';
import {Icon, type IconName} from '../../ui/Icon';
import {Avatar} from '../../ui/Media';
import {ConfirmDialog, Dialog, DialogClose, Menu} from '../../ui/Overlay';
import {cx} from '../../ui/cx';
import {relativeTime} from '../shared/items';

const COLORS = ['#409fd4', '#d4668f', '#3fae78', '#e8bc6a', '#8a5fd0', '#e0763a', '#2fa4a0', '#6f8796'];
const RATINGS = ['G', 'PG', 'PG-13', 'R', 'NC-17'];

/** Profiles on this server: add, edit (name, colour, picture, kids), PIN, limits, reorder, delete. */
export function ProfilesSection() {
  const services = useServices();
  const session = useSession();
  const version = useDataVersion();
  const profiles = useQuery(`profiles.all.${version}`, () => services.account.profiles());
  const [edit, setEdit] = useState<Profile | 'new' | null>(null);
  const [limits, setLimits] = useState<Profile | null>(null);
  const [pin, setPin] = useState<Profile | null>(null);
  const [del, setDel] = useState<Profile | null>(null);
  const main = session.profile?.main;
  const list = profiles.data ?? [];
  return (
    <div className="stack">
      {!main ? <Notice title="Only the main profile manages profiles">Switch to {list.find(p => p.main)?.name ?? 'the main profile'} to add, edit or limit profiles.</Notice> : null}
      <div className="profile-grid">
        {!profiles.data ? [0, 1, 2, 3].map(i => <Skeleton key={i} h={180} r={20} />) : list.map((p, i) => (
          <div key={p.id} className={cx('profile-card', p.id === session.profileId && 'is-current')}>
            <Avatar name={p.name} color={p.color} size={76} />
            <strong>{p.name}</strong>
            <span className="faint small">{p.main ? 'Main profile' : p.kids ? `Kids · up to ${p.limits.maxRating ?? 'any rating'}` : p.limits.maxRating ? `Up to ${p.limits.maxRating}` : 'No limits'}{p.pin ? ' · PIN' : ''}</span>
            <div className="profile-badges">{p.id === session.profileId ? <Badge tone="accent">You</Badge> : null}{p.kids ? <Badge>Kids</Badge> : null}{p.pin ? <Badge icon="lock">PIN</Badge> : null}</div>
            {main ? (
              <Menu label={`Options for ${p.name}`} trigger={<button type="button" className="btn btn-secondary btn-sm"><Icon name="edit" size={14} /><span className="btn-label">Edit</span></button>} items={[
                {id: 'edit', label: 'Name, color and picture…', icon: 'edit', onSelect: () => setEdit(p)},
                {id: 'pin', label: p.pin ? 'Change or remove PIN…' : 'Set a PIN…', icon: 'lock', onSelect: () => setPin(p)},
                ...(!p.main ? [{id: 'limits', label: 'Limits…', icon: 'shield' as IconName, onSelect: () => setLimits(p)}] : []),
                {id: 'up', label: 'Move earlier', icon: 'chevronLeft', disabled: i === 0, separatorBefore: true, onSelect: () => void services.account.moveProfile(p.id, -1)},
                {id: 'down', label: 'Move later', icon: 'chevronRight', disabled: i === list.length - 1, onSelect: () => void services.account.moveProfile(p.id, 1)},
                ...(!p.main ? [{id: 'delete', label: 'Delete profile…', icon: 'trash' as IconName, danger: true, separatorBefore: true, onSelect: () => setDel(p)}] : []),
              ]} />
            ) : null}
          </div>
        ))}
        {main && profiles.data && list.length < 8 ? (
          <button type="button" className="profile-card is-add" onClick={() => setEdit('new')}><span className="profile-plus"><Icon name="plus" size={30} /></span><strong>Add profile</strong><span className="faint small">Up to 8 on this server</span></button>
        ) : null}
      </div>
      {edit ? <ProfileEditor profile={edit === 'new' ? null : edit} onClose={() => setEdit(null)} /> : null}
      {limits ? <LimitsDialog profile={limits} onClose={() => setLimits(null)} /> : null}
      {pin ? <PinDialog profile={pin} onClose={() => setPin(null)} /> : null}
      <ConfirmDialog open={!!del} onOpenChange={o => !o && setDel(null)} danger title={`Delete ${del?.name}?`} body={`${del?.name}’s watch history, Watchlist, playlists and recommendations are deleted. This can’t be undone.`} confirmLabel="Delete profile" typeToConfirm={del?.name} onConfirm={async () => { if (del) await services.account.deleteProfile(del.id); toast('Profile deleted'); }} />
    </div>
  );
}

function ProfileEditor({profile, onClose}: {profile: Profile | null; onClose: () => void}) {
  const services = useServices();
  const [name, setName] = useState(profile?.name ?? '');
  const [color, setColor] = useState(profile?.color ?? COLORS[2]!);
  const [kids, setKids] = useState(profile?.kids ?? false);
  const [busy, setBusy] = useState(false);
  const save = async () => {
    if (!name.trim()) return;
    setBusy(true);
    if (profile) await services.account.saveProfile({...profile, name: name.trim(), color, kids, initials: name.trim()[0]!.toUpperCase()});
    else await services.account.addProfile(name.trim(), color, kids);
    setBusy(false);
    toast(profile ? 'Profile saved' : `Added ${name.trim()}`);
    onClose();
  };
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={profile ? `Edit ${profile.name}` : 'Add a profile'} size="sm" footer={<><DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose><Button variant="primary" label={profile ? 'Save' : 'Add profile'} disabled={!name.trim()} busy={busy} onClick={() => void save()} /></>}>
      <form className="stack" onSubmit={e => { e.preventDefault(); void save(); }}>
        <div className="profile-preview"><Avatar name={name || '?'} color={color} size={88} /><div className="hstack"><Button size="sm" variant="secondary" icon="image" label="Add picture" onClick={() => toast('Pictures up to 2 MB, JPEG or PNG')} /></div></div>
        <Field label="Name" error={!name.trim() && name !== (profile?.name ?? '') ? 'Enter a name.' : undefined}><Input value={name} onChange={e => setName(e.target.value)} maxLength={24} autoFocus /></Field>
        <div className="field"><span className="field-label">Color</span>
          <div className="swatches" role="radiogroup" aria-label="Color">{COLORS.map(c => <button key={c} type="button" role="radio" aria-checked={c === color} aria-label={c} className={cx('swatch', c === color && 'is-on')} style={{background: c}} onClick={() => setColor(c)} />)}</div>
        </div>
        {!profile?.main ? <label className="row" style={{padding: 0, border: 0}}><span className="row-text"><span className="row-label">Kids profile</span><span className="row-help">Only titles rated for children, and a simpler Home.</span></span><Switch checked={kids} onChange={setKids} label="Kids profile" /></label> : null}
      </form>
    </Dialog>
  );
}

function PinDialog({profile, onClose}: {profile: Profile; onClose: () => void}) {
  const services = useServices();
  const [pin, setPin] = useState('');
  const [repeat, setRepeat] = useState('');
  const mismatch = repeat.length === 4 && pin !== repeat;
  const save = async (value: string | null) => {
    await services.account.saveProfile({...profile, pin: value});
    toast(value ? 'PIN saved. It’s asked for when someone switches to this profile.' : 'PIN removed');
    onClose();
  };
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={profile.pin ? `Change ${profile.name}’s PIN` : `Set a PIN for ${profile.name}`} description="Four digits, asked for when someone switches to this profile." size="sm" footer={<>
      {profile.pin ? <Button variant="danger" label="Remove PIN" onClick={() => void save(null)} /> : null}
      <span style={{flex: 1}} />
      <DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose>
      <Button variant="primary" label="Save PIN" disabled={pin.length !== 4 || pin !== repeat} onClick={() => void save(pin)} />
    </>}>
      <div className="stack">
        <Field label="New PIN"><Input inputMode="numeric" value={pin} onChange={e => setPin(e.target.value.replace(/\D/g, '').slice(0, 4))} className="pin-input" autoFocus autoComplete="off" /></Field>
        <Field label="Repeat PIN" error={mismatch ? 'The PINs don’t match.' : undefined}><Input inputMode="numeric" value={repeat} onChange={e => setRepeat(e.target.value.replace(/\D/g, '').slice(0, 4))} className="pin-input" autoComplete="off" /></Field>
      </div>
    </Dialog>
  );
}

function LimitsDialog({profile, onClose}: {profile: Profile; onClose: () => void}) {
  const services = useServices();
  const [l, setL] = useState<ProfileLimits>(profile.limits);
  const [labels, setLabels] = useState(profile.limits.hiddenLabels.join(', '));
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={`Limits for ${profile.name}`} description="What this profile can see and do on this server." size="md" footer={<><DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose><Button variant="primary" label="Save limits" onClick={() => void services.account.saveProfile({...profile, limits: {...l, hiddenLabels: labels.split(',').map(s => s.trim()).filter(Boolean)}}).then(() => { toast('Limits saved'); onClose(); })} /></>}>
      <div className="stack">
        <Field label="Highest rating" help="Titles rated above this are hidden."><Select value={l.maxRating ?? 'none'} onChange={v => setL({...l, maxRating: v === 'none' ? null : v})} options={[{value: 'none', label: 'No limit'}, ...RATINGS.map(r => ({value: r, label: r}))]} /></Field>
        <label className="row" style={{padding: 0, border: 0}}><span className="row-text"><span className="row-label">Allow unrated titles</span><span className="row-help">Home videos and anything without a rating.</span></span><Switch checked={l.allowUnrated} onChange={v => setL({...l, allowUnrated: v})} label="Allow unrated titles" /></label>
        <div className="field"><span className="field-label">Features</span>
          <div className="stack" style={{gap: 4}}>
            <Checkbox checked={l.downloads} onChange={v => setL({...l, downloads: v})} label="Downloads" />
            <Checkbox checked={l.liveTv} onChange={v => setL({...l, liveTv: v})} label="Live TV" />
            <Checkbox checked={l.dvr} onChange={v => setL({...l, dvr: v})} label="Recording" />
            <Checkbox checked={l.together} onChange={v => setL({...l, together: v})} label="Watch Together" />
          </div>
        </div>
        <Field label="Hide titles with these labels" help="Separate with commas, like Horror, Mature." optional><Input value={labels} onChange={e => setLabels(e.target.value)} /></Field>
      </div>
    </Dialog>
  );
}

/** Signing in to this server: password and two-step verification (server accounts). */
export function SecuritySection() {
  const services = useServices();
  const version = useDataVersion();
  const account = useQuery(`account.${version}`, () => services.account.account());
  const [pw, setPw] = useState(false);
  const [twoStep, setTwoStep] = useState(false);
  const [codes, setCodes] = useState(false);
  const a = account.data;
  return (
    <Panel title="Signing in" padded={false}>
      <Row icon="key" label="Password" help="Last changed 4 months ago"><Button size="sm" variant="secondary" label="Change…" onClick={() => setPw(true)} /></Row>
      <Row icon="shieldCheck" label="Two-step verification" help={a?.twoStep ? `On · ${a.recoveryCodesLeft} recovery codes left` : 'Off · Add a code from an authenticator app when you sign in'}>
        {a?.twoStep ? <><Button size="sm" variant="ghost" label="Recovery codes" onClick={() => setCodes(true)} /><Button size="sm" variant="secondary" label="Turn off" onClick={() => void services.account.updateAccount({twoStep: false}).then(() => toast('Two-step verification is off'))} /></> : <Button size="sm" variant="primary" label="Turn on…" onClick={() => setTwoStep(true)} />}
      </Row>
      {pw ? <PasswordDialog onClose={() => setPw(false)} /> : null}
      {twoStep ? <TwoStepDialog onClose={() => setTwoStep(false)} /> : null}
      <ConfirmDialog open={codes} onOpenChange={setCodes} title="Make new recovery codes?" body="Your old codes stop working. Save the new ones somewhere safe; each works once if you lose your authenticator." confirmLabel="Make new codes" onConfirm={() => services.account.updateAccount({recoveryCodesLeft: 10}).then(() => toast('10 new recovery codes made. Download them from your Portico Account.'))} />
    </Panel>
  );
}

export function PasswordDialog({onClose}: {onClose: () => void}) {
  const [cur, setCur] = useState('');
  const [next, setNext] = useState('');
  const [again, setAgain] = useState('');
  const weak = next.length > 0 && next.length < 10;
  const strength = Math.min(4, Math.floor(next.length / 4) + (/[0-9]/.test(next) ? 1 : 0) + (/[^a-z0-9]/i.test(next) ? 1 : 0));
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title="Change password" size="sm" footer={<><DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose><Button variant="primary" label="Change password" disabled={!cur || next.length < 10 || next !== again} onClick={() => { toast('Password changed. Other devices stay signed in.'); onClose(); }} /></>}>
      <div className="stack">
        <Field label="Current password"><PasswordInput value={cur} onChange={e => setCur(e.target.value)} autoComplete="current-password" autoFocus /></Field>
        <Field label="New password" help={weak ? undefined : 'At least 10 characters. A few unrelated words works well.'} error={weak ? 'Use at least 10 characters.' : undefined}><PasswordInput value={next} onChange={e => setNext(e.target.value)} autoComplete="new-password" /></Field>
        {next ? <div className="strength" aria-label={`Strength: ${['Very weak', 'Weak', 'Fair', 'Good', 'Strong'][strength]}`}>{[0, 1, 2, 3].map(i => <span key={i} className={cx(i < strength && `is-${strength}`)} />)}<em>{['Very weak', 'Weak', 'Fair', 'Good', 'Strong'][strength]}</em></div> : null}
        <Field label="Repeat new password" error={again && again !== next ? 'The passwords don’t match.' : undefined}><PasswordInput value={again} onChange={e => setAgain(e.target.value)} autoComplete="new-password" /></Field>
      </div>
    </Dialog>
  );
}

export function TwoStepDialog({onClose}: {onClose: () => void}) {
  const services = useServices();
  const [code, setCode] = useState('');
  const [error, setError] = useState('');
  const [done, setDone] = useState(false);
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={done ? 'Two-step verification is on' : 'Turn on two-step verification'} size="md" footer={done ? <Button variant="primary" label="Done" onClick={onClose} /> : <><DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose><Button variant="primary" label="Turn on" disabled={code.length < 6} onClick={() => { if (code === '000000') return setError('That code didn’t work. Enter the current code from your authenticator app.'); void services.account.updateAccount({twoStep: true, recoveryCodesLeft: 10}).then(() => setDone(true)); }} /></>}>
      {done ? (
        <div className="stack">
          <p className="muted">Save these recovery codes somewhere safe. Each one works once if you lose your authenticator.</p>
          <div className="recovery-codes">{['7KQ2-M9XP', 'D4RT-8WLN', 'H2VB-6ZQC', 'P9MF-3TKE', 'Y6GS-1RJA', 'B8NC-5HUW', 'L3XD-7QPV', 'E5TA-2MGZ', 'W1KJ-9FRB', 'Q7UH-4SDN'].map(c => <code key={c}>{c}</code>)}</div>
          <Button variant="secondary" icon="download" label="Download codes" onClick={() => toast('Saved portico-recovery-codes.txt')} />
        </div>
      ) : (
        <div className="twostep">
          <div className="qr" aria-label="QR code for your authenticator app"><QRMock /></div>
          <div className="stack">
            <p className="muted">Scan this with an authenticator app, or add it by hand with the key below, then enter the 6-digit code it shows.</p>
            <code className="setup-key">JBSW Y3DP EHPK 3PXP</code>
            <Field label="Code from the app" error={error || undefined}><Input inputMode="numeric" value={code} onChange={e => { setCode(e.target.value.replace(/\D/g, '').slice(0, 6)); setError(''); }} className="pin-input" autoFocus autoComplete="one-time-code" /></Field>
          </div>
        </div>
      )}
    </Dialog>
  );
}

/** A decorative QR-like matrix for the fixture build (a real build renders the otpauth URL). */
export function QRMock({seed = 'portico', size = 29}: {seed?: string; size?: number}) {
  let h = 2166136261;
  for (let i = 0; i < seed.length; i++) h = Math.imul(h ^ seed.charCodeAt(i), 16777619);
  const cells: boolean[] = [];
  for (let i = 0; i < size * size; i++) { h = Math.imul(h ^ (h >>> 13), 1274126177); cells.push((h >>> 7) % 2 === 0); }
  const finder = (x: number, y: number) => [[0, 0], [size - 7, 0], [0, size - 7]].some(([fx, fy]) => x >= fx! && x < fx! + 7 && y >= fy! && y < fy! + 7);
  const inFinder = (x: number, y: number) => [[0, 0], [size - 7, 0], [0, size - 7]].map(([fx, fy]) => [x - fx!, y - fy!]).find(([dx, dy]) => dx! >= 0 && dx! < 7 && dy! >= 0 && dy! < 7);
  return (
    <svg viewBox={`0 0 ${size + 2} ${size + 2}`} role="img" aria-label="QR code" shapeRendering="crispEdges">
      <rect width={size + 2} height={size + 2} fill="#fff" />
      {cells.map((on, i) => {
        const x = i % size;
        const y = Math.floor(i / size);
        const f = inFinder(x, y);
        const dark = f ? (f[0] === 0 || f[0] === 6 || f[1] === 0 || f[1] === 6 || (f[0]! >= 2 && f[0]! <= 4 && f[1]! >= 2 && f[1]! <= 4)) : on && !finder(x, y);
        return dark ? <rect key={i} x={x + 1} y={y + 1} width={1} height={1} fill="#071018" /> : null;
      })}
    </svg>
  );
}

/** Devices signed in to this server: approve waiting ones, trust, rename, sign out. */
export function DevicesSection() {
  const services = useServices();
  const version = useDataVersion();
  const sessions = useQuery(`sessions.${version}`, () => services.account.sessions());
  const [out, setOut] = useState<DeviceSession | 'all' | null>(null);
  const [renaming, setRenaming] = useState<DeviceSession | null>(null);
  const [name, setName] = useState('');
  useEffect(() => { if (renaming) setName(renaming.name); }, [renaming]);
  const icon = (p: DeviceSession['platform']): IconName => (p === 'web' ? 'monitor' : p === 'ios' ? 'phone' : 'tv');
  return (
    <Panel title="Where you’re signed in" subtitle="Browsers and apps using this server as you." actions={<Button size="sm" variant="ghost" label="Sign out everywhere else" onClick={() => setOut('all')} />} padded={false}>
      {!sessions.data ? <Row label={<Skeleton w={220} h={16} />} /> : sessions.data.map(s => (
        <Row key={s.id} icon={icon(s.platform)} label={<>{s.name}{s.current ? <Badge tone="accent" className="ml">This device</Badge> : null}{s.waiting ? <Badge tone="warn" className="ml">Waiting for approval</Badge> : null}</>} help={`${s.app} · ${s.location ?? 'Unknown location'} · ${s.current ? 'Active now' : `Last used ${relativeTime(s.lastUsed).toLowerCase()}`}${s.trusted ? ' · Trusted' : ''}`}>
          {s.waiting ? <><Button size="sm" variant="primary" label="Approve" onClick={() => void services.account.updateSession(s.id, {waiting: false, trusted: true}).then(() => toast(`${s.name} can use this server now`))} /><Button size="sm" variant="ghost" label="Deny" onClick={() => void services.account.signOutSession(s.id).then(() => toast('Denied'))} /></> : null}
          {!s.waiting ? (
            <Menu label={`Options for ${s.name}`} trigger={<button type="button" className="track-more is-shown" aria-label={`Options for ${s.name}`}><Icon name="more" size={18} /></button>} items={[
              {id: 'rename', label: 'Rename…', icon: 'edit', onSelect: () => setRenaming(s)},
              {id: 'trust', label: s.trusted ? 'Stop trusting' : 'Trust this device', icon: 'shieldCheck', onSelect: () => void services.account.updateSession(s.id, {trusted: !s.trusted}).then(() => toast(s.trusted ? 'It will ask for two-step codes again' : 'Trusted: no two-step codes on this device'))},
              ...(!s.current ? [{id: 'out', label: 'Sign out', icon: 'signOut' as IconName, danger: true, separatorBefore: true, onSelect: () => setOut(s)}] : []),
            ]} />
          ) : null}
        </Row>
      ))}
      <ConfirmDialog open={!!out} onOpenChange={o => !o && setOut(null)} danger title={out === 'all' ? 'Sign out everywhere else?' : `Sign out ${out ? out.name : ''}?`} body={out === 'all' ? 'Every other browser and app signed in as you needs to sign in again. This one stays signed in.' : 'It needs to sign in again to use this server.'} confirmLabel="Sign out" onConfirm={async () => { if (out === 'all') await services.account.signOutOthers(); else if (out) await services.account.signOutSession(out.id); toast('Signed out'); }} />
      <Dialog open={!!renaming} onOpenChange={o => !o && setRenaming(null)} title="Rename device" size="sm" footer={<><DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose><Button variant="primary" label="Rename" disabled={!name.trim()} onClick={() => renaming && void services.account.updateSession(renaming.id, {name: name.trim()}).then(() => { toast('Renamed'); setRenaming(null); })} /></>}>
        <Field label="Name"><Input value={name} onChange={e => setName(e.target.value)} autoFocus maxLength={40} /></Field>
      </Dialog>
    </Panel>
  );
}
