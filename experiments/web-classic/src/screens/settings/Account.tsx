import {useState} from 'react';
import {useClient, useQuery} from '@/data/DataProvider';
import type {Device, Profile} from '@/data/types';
import {useSession} from '@/app/session';
import {Button, IconButton} from '@/ui/Button';
import {Avatar, Badge, Skeleton} from '@/ui/Bits';
import {Checkbox, PinInput, Select, Switch, TextField} from '@/ui/Form';
import {ConfirmDialog, Dialog, Menu, useToast} from '@/ui/Overlay';
import {Icon, type IconName} from '@/ui/Icon';
import {Group, Row} from '@/ui/Page';
import {useDemo} from '@/app/demo';
import s from './Settings.module.css';

const ARTS = ['blue', 'violet', 'mint', 'coral', 'gold', 'slate', 'rose', 'sky'];

function Swatches({value, onChange}: {value: string; onChange: (v: string) => void}) {
  return <div className={s.swatches} role="radiogroup" aria-label="Colour">{ARTS.map(a => <button key={a} type="button" role="radio" aria-checked={value === a} aria-label={a} className={s.swatch} style={{background: `var(--art-${a})`}} onClick={() => onChange(a)} />)}</div>;
}

/** This profile: name, colour and PIN. */
export function AccountSection() {
  const session = useSession();
  const client = useClient();
  const toast = useToast();
  const p = session.profile!;
  const [name, setName] = useState(p.name);
  const [pinDialog, setPinDialog] = useState(false);
  const [removePin, setRemovePin] = useState(false);
  return (
    <>
      <Group title="This profile">
        <div style={{padding: 16, display: 'flex', gap: 20, alignItems: 'center', flexWrap: 'wrap'}}>
          <Avatar name={name || p.name} art={p.art} size={72} kids={p.kids} />
          <div style={{flex: '1 1 260px', display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 12}}>
            <form style={{display: 'flex', gap: 8, alignItems: 'flex-end'}} onSubmit={async e => { e.preventDefault(); await client.account.updateProfile(p.id, {name: name.trim()}); toast.show('Name saved', {icon: 'check'}); }}>
              <TextField label="Profile name" value={name} onChange={setName} className="grow" />
              <Button type="submit" variant="secondary" label="Save" disabled={!name.trim() || name.trim() === p.name} />
            </form>
            <div><div className="t-caption c-2" style={{marginBottom: 6, fontWeight: 650}}>Colour</div><Swatches value={p.art} onChange={async a => { await client.account.updateProfile(p.id, {art: a}); toast.show('Colour saved', {icon: 'check'}); }} /></div>
          </div>
        </div>
        <Row icon="lock" label="Profile PIN" help={p.pin ? 'A four-digit PIN is needed to open this profile.' : 'Anyone on this account can open this profile.'} trailing={p.pin ? <><Button size="sm" variant="secondary" label="Change" onClick={() => setPinDialog(true)} /><Button size="sm" variant="ghost" label="Remove" onClick={() => setRemovePin(true)} /></> : <Button size="sm" variant="secondary" label="Add a PIN" onClick={() => setPinDialog(true)} />} />
        <Row icon="user" label="Portico Account" help={`${session.session?.account.displayName} · ${session.session?.account.email}`} to="/account" />
      </Group>
      <PinDialog open={pinDialog} onOpenChange={setPinDialog} profile={p} />
      <ConfirmDialog open={removePin} onOpenChange={setRemovePin} title="Remove this profile’s PIN?" body="Anyone using this Portico Account can open the profile without it." action="Remove PIN" tone="danger" onConfirm={async () => { await client.account.updateProfile(p.id, {pinValue: null}); setRemovePin(false); toast.show('PIN removed', {icon: 'check'}); }} />
    </>
  );
}

function PinDialog({open, onOpenChange, profile}: {open: boolean; onOpenChange: (o: boolean) => void; profile: Profile}) {
  const client = useClient();
  const toast = useToast();
  const [pin, setPin] = useState('');
  const [confirm, setConfirm] = useState('');
  const [step, setStep] = useState<1 | 2>(1);
  const [error, setError] = useState<string>();
  const close = () => { setPin(''); setConfirm(''); setStep(1); setError(undefined); onOpenChange(false); };
  return (
    <Dialog open={open} onOpenChange={o => (o ? onOpenChange(o) : close())} title={profile.pin ? `Change ${profile.name}’s PIN` : `Add a PIN to ${profile.name}`} description={step === 1 ? 'Choose four digits.' : 'Enter the same four digits again.'} width={420}>
      <div style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 16, justifyItems: 'center', padding: '8px 0 12px'}}>
        {step === 1 ? <PinInput key="a" label="New PIN" value={pin} onChange={setPin} autoFocus onComplete={() => setStep(2)} /> : <PinInput key="b" label="Confirm PIN" value={confirm} onChange={v => { setConfirm(v); setError(undefined); }} autoFocus onComplete={async v => { if (v !== pin) { setError('Those didn’t match. Try again.'); setConfirm(''); return; } await client.account.updateProfile(profile.id, {pinValue: v}); toast.show('PIN saved', {icon: 'lock'}); close(); }} />}
        {error ? <p className="c-danger t-caption" role="alert">{error}</p> : null}
      </div>
    </Dialog>
  );
}

/** Everyone in the household: add, rename, colour, kids, PIN, limits, order and removal. */
export function ProfilesSection() {
  const session = useSession();
  const client = useClient();
  const toast = useToast();
  const profiles = useQuery(c => c.session.profiles(), [], ['account', 'session']);
  const [edit, setEdit] = useState<Profile | 'new'>();
  const [del, setDel] = useState<Profile>();
  const canManage = !!session.profile?.main || session.isAdmin;
  useDemo('profile-editor', () => { const p = profiles.data?.find(x => x.id === 'pr-nora'); if (p) setEdit(p); }, !!profiles.data);
  return (
    <>
      <Group title="Profiles" lede={`Up to eight people can have their own profile on this account, each with their own history, Watchlist and recommendations. ${canManage ? '' : 'Only the main profile can change other profiles.'}`}>
        {!profiles.data ? <div style={{padding: 16}}><Skeleton height={120} /></div> : profiles.data.map((p, i) => (
          <Row key={p.id} iconNode={<Avatar name={p.name} art={p.art} size={40} kids={p.kids} />} label={<>{p.name}{p.id === session.profile?.id ? <Badge tone="accent" className="" >You</Badge> : null}</>}
            help={[p.main ? 'Main profile' : p.kids ? 'Kids profile' : p.role === 'admin' ? 'Administrator' : 'Member', p.pin ? 'PIN' : null, p.limits ? `Up to ${p.limits.rating}` : null].filter(Boolean).join(' · ')}
            trailing={canManage ? <Menu label={`${p.name} options`} items={[
              {id: 'edit', label: 'Edit profile…', icon: 'edit', onSelect: () => setEdit(p)},
              {id: 'up', label: 'Move up', icon: 'chevronUp', disabled: p.main || i <= 1, onSelect: () => void client.account.moveProfile(p.id, -1)},
              {id: 'down', label: 'Move down', icon: 'chevronDown', disabled: p.main || i === profiles.data!.length - 1, onSelect: () => void client.account.moveProfile(p.id, 1)},
              {id: 'delete', label: 'Delete profile…', icon: 'trash', danger: true, disabled: p.main || p.id === session.profile?.id, separatorBefore: true, onSelect: () => setDel(p)},
            ]} trigger={<IconButton variant="ghost" icon="more" label={`${p.name} options`} />} /> : undefined} />
        ))}
        {canManage ? <Row icon="plus" label="Add a profile" help={`${profiles.data?.length ?? 0} of 8 profiles`} onClick={() => setEdit('new')} /> : null}
      </Group>
      {edit ? <ProfileEditor profile={edit === 'new' ? undefined : edit} onClose={() => setEdit(undefined)} /> : null}
      {del ? <ConfirmDialog open onOpenChange={o => !o && setDel(undefined)} title={`Delete ${del.name}?`} body={`${del.name}’s watch history, Watchlist, playlists and recommendations are deleted. This can’t be undone.`} action="Delete profile" tone="danger" typeToConfirm={del.name} onConfirm={async () => { await client.account.deleteProfile(del.id); setDel(undefined); toast.show(`Deleted ${del.name}`, {icon: 'trash'}); }} /> : null}
    </>
  );
}

function ProfileEditor({profile, onClose}: {profile?: Profile; onClose: () => void}) {
  const client = useClient();
  const toast = useToast();
  const [name, setName] = useState(profile?.name ?? '');
  const [art, setArt] = useState(profile?.art ?? 'sky');
  const [kids, setKids] = useState(profile?.kids ?? false);
  const [limits, setLimits] = useState(profile?.limits ?? {rating: kids ? 'PG' : 'R', unrated: !kids, downloads: true, liveTv: true, dvr: !kids, together: true});
  const [limited, setLimited] = useState(!!profile?.limits);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const save = async () => {
    setBusy(true);
    try {
      if (profile) await client.account.updateProfile(profile.id, {name: name.trim(), art, kids, limits: limited || kids ? limits : undefined});
      else { const p = await client.account.addProfile(name.trim(), art, kids); if (limited || kids) await client.account.updateProfile(p.id, {limits}); }
      toast.show(profile ? 'Profile saved' : `Added ${name.trim()}`, {icon: 'check'});
      onClose();
    } catch (e) { setError(e instanceof Error ? e.message : 'That didn’t save.'); } finally { setBusy(false); }
  };
  const L = (k: keyof typeof limits, label: string, help: string, icon: IconName) => <Row icon={icon} label={label} help={help} trailing={<Switch checked={limits[k] as boolean} onChange={v => setLimits({...limits, [k]: v})} label={label} />} />;
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={profile ? `Edit ${profile.name}` : 'Add a profile'} width={560} footer={<><Button variant="ghost" label="Cancel" onClick={onClose} /><Button variant="primary" label={profile ? 'Save' : 'Add profile'} busy={busy} disabled={!name.trim()} onClick={() => void save()} /></>}>
      <div style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 16}}>
        <div style={{display: 'flex', gap: 16, alignItems: 'center'}}><Avatar name={name || '?'} art={art} size={60} kids={kids} /><TextField label="Name" value={name} onChange={setName} autoFocus error={error} className="grow" /></div>
        <div><div className="t-caption c-2" style={{marginBottom: 6, fontWeight: 650}}>Colour</div><Swatches value={art} onChange={setArt} /></div>
        <Checkbox checked={kids} onChange={v => { setKids(v); if (v) setLimits({...limits, rating: 'PG', unrated: false, dvr: false}); }} label="Kids profile" help="A simpler Portico with only titles rated for children, and no settings or server management." disabled={profile?.main} />
        {!kids ? <Checkbox checked={limited} onChange={setLimited} label="Limit what this profile can watch and do" disabled={profile?.main} /> : null}
        {kids || limited ? (
          <Group>
            <Row icon="shield" label="Highest rating" help="Titles rated above this are hidden." trailing={<Select compact ariaLabel="Highest rating" value={limits.rating} onChange={v => setLimits({...limits, rating: v})} options={['G', 'PG', 'PG-13', 'R', 'NC-17'].map(r => ({value: r, label: r}))} />} />
            {L('unrated', 'Unrated titles', 'Show titles with no rating.', 'eye')}
            {L('downloads', 'Downloads', 'Allow downloading for offline viewing.', 'download')}
            {L('liveTv', 'Live TV', 'Allow watching live channels.', 'live')}
            {L('dvr', 'Recording', 'Allow scheduling recordings.', 'record')}
            {L('together', 'Watch Together', 'Allow joining watch parties.', 'together')}
          </Group>
        ) : null}
      </div>
    </Dialog>
  );
}

/** Password, two-step sign-in with recovery codes, and signing out everywhere. */
export function SecuritySection() {
  const client = useClient();
  const toast = useToast();
  const session = useSession();
  const acct = session.session!.account;
  const [pw, setPw] = useState(false);
  const [twoStep, setTwoStep] = useState<'enable' | 'disable' | 'codes'>();
  const [everywhere, setEverywhere] = useState(false);
  return (
    <>
      <Group title="Signing in" lede={`These protect your Portico Account (${acct.email}), which you use on every server.`}>
        <Row icon="key" label="Password" help="Last changed in March" trailing={<Button size="sm" variant="secondary" label="Change…" onClick={() => setPw(true)} />} />
        <Row icon="shield" label="Two-step sign-in" help={acct.twoStep ? `On · ${acct.recoveryCodesLeft} recovery codes left` : 'Off. Add a code from an authenticator app when you sign in.'} trailing={acct.twoStep ? <><Button size="sm" variant="secondary" label="Recovery codes" onClick={() => setTwoStep('codes')} /><Button size="sm" variant="ghost" label="Turn off" onClick={() => setTwoStep('disable')} /></> : <Button size="sm" variant="primary" label="Turn on…" onClick={() => setTwoStep('enable')} />} />
        <Row icon="link" label="Sign in with Apple or Google" help={acct.providers.length ? `Connected: ${acct.providers.map(p => (p === 'apple' ? 'Apple' : 'Google')).join(', ')}` : 'Not connected'} to="/account?tab=security" />
      </Group>
      <Group title="Everywhere">
        <Row icon="signOut" label="Sign out everywhere else" help="Every other device and browser signed in to your account has to sign in again." trailing={<Button size="sm" variant="danger" label="Sign out others…" onClick={() => setEverywhere(true)} />} />
      </Group>
      <PasswordDialog open={pw} onOpenChange={setPw} />
      {twoStep ? <TwoStepDialog mode={twoStep} onClose={() => setTwoStep(undefined)} /> : null}
      <ConfirmDialog open={everywhere} onOpenChange={setEverywhere} title="Sign out everywhere else?" body="This browser stays signed in. Everything else, including TVs, needs to sign in again." action="Sign out others" tone="danger" onConfirm={async () => { await client.account.signOutEverywhere(); setEverywhere(false); toast.show('Signed out of every other device', {icon: 'check'}); }} />
    </>
  );
}

function PasswordDialog({open, onOpenChange}: {open: boolean; onOpenChange: (o: boolean) => void}) {
  const client = useClient();
  const toast = useToast();
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  const strength = next.length >= 14 && /\d/.test(next) && /[^\w]/.test(next) ? 3 : next.length >= 10 ? 2 : next.length >= 8 ? 1 : 0;
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title="Change password" width={440} footer={<><Button variant="ghost" label="Cancel" onClick={() => onOpenChange(false)} /><Button variant="primary" label="Change password" busy={busy} onClick={async () => { setBusy(true); setError(undefined); try { await client.account.changePassword(current, next); toast.show('Password changed', {icon: 'check'}); onOpenChange(false); setCurrent(''); setNext(''); } catch (e) { setError(e instanceof Error ? e.message : 'That didn’t work.'); } finally { setBusy(false); } }} /></>}>
      <form style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 14}} onSubmit={e => e.preventDefault()}>
        <TextField type="password" label="Current password" value={current} onChange={setCurrent} autoComplete="current-password" autoFocus />
        <TextField type="password" label="New password" value={next} onChange={setNext} autoComplete="new-password" error={error} help={next ? ['Too short. Use at least 8 characters.', 'Fair. Longer is stronger.', 'Good.', 'Strong.'][strength] : 'At least 8 characters. A few unrelated words make a strong password.'} />
      </form>
    </Dialog>
  );
}

function TwoStepDialog({mode, onClose}: {mode: 'enable' | 'disable' | 'codes'; onClose: () => void}) {
  const client = useClient();
  const toast = useToast();
  const [code, setCode] = useState('');
  const [codes, setCodes] = useState<string[]>();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  if (codes) return (
    <Dialog open onOpenChange={o => !o && onClose()} title="Save your recovery codes" description="Each code works once, if you lose your authenticator. Keep them somewhere safe; they won’t be shown again." width={460} footer={<><Button variant="secondary" icon="copy" label="Copy" onClick={() => { void navigator.clipboard?.writeText(codes.join('\n')).catch(() => {}); toast.show('Codes copied', {icon: 'copy'}); }} /><Button variant="primary" label="I’ve saved them" onClick={onClose} /></>}>
      <div className={s.codes}>{codes.map(c => <span key={c}>{c}</span>)}</div>
    </Dialog>
  );
  if (mode === 'codes') return (
    <ConfirmDialog open onOpenChange={o => !o && onClose()} title="Make new recovery codes?" body="Your old recovery codes stop working." action="Make new codes" onConfirm={async () => setCodes(await client.account.recoveryCodes())} />
  );
  if (mode === 'disable') return (
    <ConfirmDialog open onOpenChange={o => !o && onClose()} title="Turn off two-step sign-in?" body="Signing in will need only your password." action="Turn off" tone="danger" onConfirm={async () => { await client.account.setTwoStep(false); toast.show('Two-step sign-in is off', {icon: 'shield'}); onClose(); }} />
  );
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title="Turn on two-step sign-in" width={480} footer={<><Button variant="ghost" label="Cancel" onClick={onClose} /><Button variant="primary" label="Turn on" busy={busy} disabled={code.length !== 6} onClick={async () => { setBusy(true); try { const r = await client.account.setTwoStep(true, code); setCodes(r.recoveryCodes); } catch (e) { setError(e instanceof Error ? e.message : 'That code didn’t work.'); } finally { setBusy(false); } }} /></>}>
      <div style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 16}}>
        <p className="c-2">1. In your authenticator app, add an account with this setup key:</p>
        <div className={s.codes} style={{gridTemplateColumns: '1fr', textAlign: 'center', letterSpacing: '0.12em'}}>JBSW Y3DP EHPK 3PXP</div>
        <p className="c-2">2. Enter the six-digit code it shows.</p>
        <div style={{display: 'grid', justifyItems: 'center', gap: 8}}><PinInput length={6} secret={false} label="Authenticator code" value={code} onChange={v => { setCode(v); setError(undefined); }} autoFocus />{error ? <p className="c-danger t-caption" role="alert">{error}</p> : null}</div>
      </div>
    </Dialog>
  );
}

const PLATFORM: Record<Device['platform'], {label: string; icon: IconName}> = {web: {label: 'Web browser', icon: 'laptop'}, ios: {label: 'iPhone or iPad', icon: 'phone'}, tvos: {label: 'Apple TV', icon: 'tvDevice'}, android: {label: 'Android', icon: 'phone'}, androidtv: {label: 'Android TV', icon: 'tvDevice'}};

/** Where this account is signed in: this browser, trusted devices, rename, approve and sign out. */
export function DevicesSection() {
  const client = useClient();
  const toast = useToast();
  const session = useSession();
  const devices = useQuery(c => c.account.devices(), [], ['account']);
  const [rename, setRename] = useState<Device>();
  const [out, setOut] = useState<Device>();
  const [name, setName] = useState('');
  return (
    <>
      <Group title="Signed-in devices" lede="Trusted devices skip two-step sign-in. Signing out a device ends its session right away.">
        {!devices.data ? <div style={{padding: 16}}><Skeleton height={160} /></div> : devices.data.map(d => (
          <Row key={d.id} icon={PLATFORM[d.platform].icon} label={<>{d.name}{d.current ? <> <Badge tone="accent">This browser</Badge></> : null}</>} help={`${d.app} · ${PLATFORM[d.platform].label} · ${d.current ? 'Active now' : `Last seen ${d.lastSeen.toLowerCase()}`}${d.location ? ` · ${d.location}` : ''}`}
            trailing={<>
              {d.state === 'waiting' ? <Badge tone="warn">Waiting for approval</Badge> : d.trusted ? <Badge tone="good">Trusted</Badge> : null}
              {d.state === 'waiting' && session.isAdmin ? <Button size="sm" variant="primary" label="Approve" onClick={async () => { await client.account.trustDevice(d.id, false); toast.show(`${d.name} can use Portico now`, {icon: 'check'}); }} /> : null}
              <Menu label={`${d.name} options`} items={[
                {id: 'rename', label: 'Rename…', icon: 'edit', onSelect: () => { setName(d.name); setRename(d); }},
                {id: 'trust', label: d.trusted ? 'Stop trusting' : 'Trust this device', icon: 'shield', onSelect: async () => { await client.account.trustDevice(d.id, !d.trusted); toast.show(d.trusted ? `${d.name} will need two-step sign-in` : `${d.name} is trusted`, {icon: 'shield'}); }},
                ...(d.current ? [] : [{id: 'out', label: 'Sign out…', icon: 'signOut' as const, danger: true, separatorBefore: true, onSelect: () => setOut(d)}]),
              ]} trigger={<IconButton variant="ghost" icon="more" label={`${d.name} options`} />} />
            </>} />
        ))}
      </Group>
      <Dialog open={!!rename} onOpenChange={o => !o && setRename(undefined)} title="Rename device" width={420} footer={<><Button variant="ghost" label="Cancel" onClick={() => setRename(undefined)} /><Button variant="primary" label="Save" disabled={!name.trim()} onClick={async () => { await client.account.renameDevice(rename!.id, name.trim()); setRename(undefined); toast.show('Renamed', {icon: 'check'}); }} /></>}>
        <TextField label="Name" value={name} onChange={setName} autoFocus />
      </Dialog>
      {out ? <ConfirmDialog open onOpenChange={o => !o && setOut(undefined)} title={`Sign out ${out.name}?`} body="It will need to sign in again to use Portico." action="Sign out" tone="danger" onConfirm={async () => { await client.account.signOutDevice(out.id); setOut(undefined); toast.show(`Signed out ${out.name}`, {icon: 'signOut'}); }} /> : null}
      <p className="t-caption c-3" style={{display: 'flex', gap: 6, alignItems: 'center'}}><Icon name="info" size={15} />To sign in a TV, choose Link a TV and enter the code it shows.</p>
    </>
  );
}
