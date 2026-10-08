import React, {useCallback, useEffect, useMemo, useState} from 'react';
import {IdentityClient, deviceAwaitingApproval, type Device, type ProfileRestrictions, type RatingSystem, type TwoFactorEnrolment, type TwoFactorState} from '../../../../packages/client-core/src/index';
import {useSession} from '../../app/session';
import {browserInstallation} from '../../bridge/profile-trust';
import {Badge, Button, ConfirmDialog, Dialog, Input, Loading, Notice, PasswordInput, QR, Select, SettingsGroup, SettingsRow, Switch, Text, relative, type IconName} from '../../ui';
import {defaultI18n} from '@i18n';
import {errorText} from '../../app/errors';

const t = defaultI18n.t;

/** Every failure is worded by the shared presenter (X-04); a wrong password gets its own words. */
const say = (e: unknown, context: 'account' | 'devices' | 'profiles', operation: 'load' | 'save' | 'action' = 'action') => {
  const status = (e as {status?: number})?.status;
  return operation !== 'load' && (status === 401 || status === 403) ? t('account.wrongPassword') : errorText(e, context, operation);
};

function useIdentity() {
  const {api} = useSession();
  return useMemo(() => new IdentityClient(api), [api]);
}

/** Loads once and keeps what it loaded: a failed refresh leaves the last good value in place
 * beside the error, instead of replacing the section with a failure. */
function useLoaded<T>(load: (signal: AbortSignal) => Promise<T>, deps: React.DependencyList, context: 'account' | 'devices' | 'profiles') {
  const [value, setValue] = useState<T>();
  const [error, setError] = useState('');
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setError('');
    load(controller.signal).then(v => { if (!controller.signal.aborted) setValue(v); }, e => { if (!controller.signal.aborted) setError(say(e, context, 'load')); });
    return () => controller.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, revision]);
  return {value, setValue, error, reload: useCallback(() => setRevision(r => r + 1), [])};
}

/* ---------- Profile restrictions ---------- */

const permissions = (): {key: 'allowDownloads' | 'allowLiveTv' | 'allowDvr' | 'allowWatchTogether'; label: string; help: string}[] => [
 {key: 'allowDownloads', label: t('limits.feature.downloads'), help: t('web.limits.downloadsHelp')},
 {key: 'allowLiveTv', label: t('limits.feature.liveTv'), help: t('web.limits.liveTvHelp')},
 {key: 'allowDvr', label: t('limits.feature.dvr'), help: t('web.limits.dvrHelp')},
 {key: 'allowWatchTogether', label: t('limits.feature.watchTogether'), help: t('web.limits.togetherHelp')},
];

/** What one profile may see and do. C45: the signed-in account session is the authority (the server
 * refuses a profile session), so saving asks for no password. */
export function ProfileRestrictionsDialog({profile, onClose}: {profile: {id: string; name: string}; onClose: () => void}) {
  const identity = useIdentity();
  const systems = useLoaded<readonly RatingSystem[]>(s => identity.ratingSystems(s), [identity], 'profiles');
  const current = useLoaded<ProfileRestrictions>(s => identity.restrictions(profile.id, s), [identity, profile.id], 'profiles');
  const [draft, setDraft] = useState<ProfileRestrictions>();
  const [labels, setLabels] = useState('');
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState('');
  useEffect(() => { if (current.value) { setDraft(current.value); setLabels(current.value.blockedLabels.join(', ')); } }, [current.value]);
  const system = systems.value?.find(s => s.id === draft?.ratingSystem) ?? systems.value?.[0];
  const blocked = labels.split(',').map(l => l.trim()).filter(Boolean);
  const dirty = !!draft && !!current.value && (JSON.stringify({...draft, blockedLabels: blocked}) !== JSON.stringify({...current.value, blockedLabels: [...current.value.blockedLabels]}));
  const save = async () => {
    if (busy || !draft || !current.value) return;
    setBusy(true); setFailure('');
    try {
      const saved = await identity.saveRestrictions(current.value, {ratingSystem: draft.ratingSystem, maximumAgeRating: draft.maximumAgeRating, allowUnrated: draft.allowUnrated, blockedLabels: blocked, allowDownloads: draft.allowDownloads, allowLiveTv: draft.allowLiveTv, allowDvr: draft.allowDvr, allowWatchTogether: draft.allowWatchTogether});
      current.setValue(saved); onClose();
    } catch (e) { setFailure(say(e, 'profiles', 'save')); } finally { setBusy(false); }
  };
  return (
    <>
      <Dialog open onOpenChange={o => !o && onClose()} title={t('limits.title', {name: profile.name})} description={t('limits.subtitle')} width={560}
        actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} /><Button variant="primary" label={t('limits.save')} disabled={!dirty} loading={busy} onClick={() => void save()} /></>}>
        {failure ? <Notice tone="error" compact>{failure}</Notice> : null}
        {(current.error || systems.error) && !draft ? <Notice tone="error" action={{label: t('action.tryAgain'), onClick: () => { current.reload(); systems.reload(); }}}>{current.error || systems.error}</Notice> : null}
        {!draft && !current.error ? <Loading label={t('limits.loading')} /> : null}
        {draft ? (
          <div style={{display: 'flex', flexDirection: 'column', gap: 16}}>
            <SettingsGroup title={t('limits.content')}>
              <SettingsRow label={t('limits.ratingSystem')} control={<Select hideLabel label={t('limits.ratingSystem')} value={draft.ratingSystem} onChange={e => setDraft({...draft, ratingSystem: e.target.value, maximumAgeRating: ''})} options={(systems.value ?? []).map(s => ({value: s.id, label: t('web.limits.systemOption', {name: s.name, region: s.region})}))} />} />
              <SettingsRow label={t('limits.highestRating')} help={t('limits.highestRatingHelp')} control={<Select hideLabel label={t('limits.highestRating')} value={draft.maximumAgeRating} onChange={e => setDraft({...draft, maximumAgeRating: e.target.value})} options={[{value: '', label: t('limits.noLimit')}, ...(system?.values ?? []).map(v => ({value: v.code, label: t('web.limits.ratingOption', {label: v.label, age: v.minimumAge})}))]} />} />
              <SettingsRow label={t('web.limits.unrated')} help={t('limits.allowUnratedHelp')} control={<Switch checked={draft.allowUnrated} onCheckedChange={v => setDraft({...draft, allowUnrated: v})} label={t('limits.allowUnrated')} />} />
              <SettingsRow label={t('web.limits.hiddenLabels')} help={t('web.limits.hiddenLabelsHelp')} stack control={<Input hideLabel label={t('web.limits.hiddenLabels')} value={labels} onChange={e => setLabels(e.target.value)} style={{width: '100%'}} />} />
            </SettingsGroup>
            <SettingsGroup title={t('limits.features')}>
              {permissions().map(p => <SettingsRow key={p.key} label={p.label} help={p.help} control={<Switch checked={draft[p.key]} onCheckedChange={v => setDraft({...draft, [p.key]: v})} label={p.label} />} />)}
            </SettingsGroup>
          </div>
        ) : null}
      </Dialog>
    </>
  );
}

/* ---------- Devices ---------- */

const platformName = (platform: string): string => ({web: t('device.platform.web'), ios: t('device.platform.ios'), tvos: t('device.platform.tvos'), android: t('device.platform.android'), androidtv: t('device.platform.androidtv')} as Record<string, string>)[platform] ?? platform;
// Mid-sentence: "last seen just now" (the shared helper lowercases the sentence-case forms).
const ago = (iso: string) => relative(Date.now() - Date.parse(iso));

/** Settings › This device: what this browser is called in every device list, with Rename. */
export function DeviceNameRow() {
  const identity = useIdentity();
  const installation = browserInstallation();
  const devices = useLoaded<readonly Device[]>(s => identity.devices(installation, s), [identity, installation], 'devices');
  const current = devices.value?.find(d => d.current);
  const [name, setName] = useState<string>();
  const [failure, setFailure] = useState('');
  if (!current) return null;
  const save = async () => {
    setFailure('');
    try { await identity.editDevice(current.id, {name: (name ?? '').trim()}); setName(undefined); devices.reload(); } catch (e) { setFailure(say(e, 'devices', 'action')); }
  };
  return (
    <>
      <SettingsRow label={t('settings.row.deviceName')} meta={current.name || platformName(current.platform) || t('web.devices.device')} control={<Button size="sm" variant="secondary" label={t('settings.device.rename')} onClick={() => { setName(current.name); setFailure(''); }} />} />
      <Dialog open={name !== undefined} onOpenChange={o => !o && setName(undefined)} title={t('settings.row.deviceName')} width={440}
        actions={<><Button variant="ghost" label={t('action.cancel')} onClick={() => setName(undefined)} /><Button variant="primary" label={t('web.devices.saveName')} disabled={!name?.trim() || name.trim() === current.name} onClick={() => void save()} /></>}>
        <div style={{display: 'flex', flexDirection: 'column', gap: 16}}>
          {failure ? <Notice tone="error" compact>{failure}</Notice> : null}
          <Input label={t('profile.name')} value={name ?? ''} maxLength={80} onChange={e => setName(e.target.value)} />
        </div>
      </Dialog>
    </>
  );
}

export function DevicesSection() {
  const identity = useIdentity();
  const installation = browserInstallation();
  const devices = useLoaded<readonly Device[]>(s => identity.devices(installation, s), [identity, installation], 'devices');
  const [chosen, setChosen] = useState<Device>();
  const [name, setName] = useState('');
  const [failure, setFailure] = useState('');
  const [everywhere, setEverywhere] = useState(false);
  const session = useSession();
  const [removing, setRemoving] = useState<Device>();
  const act = async (work: () => Promise<unknown>) => { setFailure(''); try { await work(); setChosen(undefined); devices.reload(); } catch (e) { setFailure(say(e, 'devices', 'action')); } };
  const list = devices.value;
  return (
    <SettingsGroup title={t('devices.title')} description={t('devices.footer')}>
      {devices.error ? <div style={{padding: 12}}><Notice tone={list ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: devices.reload}}>{devices.error}</Notice></div> : null}
      {!list && !devices.error ? <div style={{padding: 16}}><Loading label={t('status.loadingThing', {thing: t('devices.title').toLowerCase()})} /></div> : null}
      {list?.map(d => (
        <SettingsRow key={d.id} icon={deviceGlyph(d.platform, d.name)} label={<>{d.name || platformName(d.platform) || t('web.devices.device')} {d.current ? <Badge tone="accent">{t('device.thisDevice')}</Badge> : null} {deviceAwaitingApproval(d) ? <Badge tone="warning" dot>{t('web.devices.waitingApproval')}</Badge> : null}</>}
          help={[platformName(d.platform), d.appVersion ? `${d.app} ${d.appVersion}` : '', t('web.devices.lastSeen', {when: ago(d.lastSeen)}), d.ip ?? ''].filter(Boolean).join(' · ')}
          meta={d.sessions ? t('web.devices.sessions', {count: d.sessions}) : t('device.signedOut')} onClick={() => { setChosen(d); setName(d.name); setFailure(''); }} />
      ))}
      {list && !list.length ? <div style={{padding: 16}}><Text variant="caption" tone="tertiary">{t('web.devices.none')}</Text></div> : null}
      <div style={{padding: '8px 16px 16px'}}><Button variant="outline" size="sm" icon="signOut" label={t('account.signOutEverywhere.row')} onClick={() => { setEverywhere(true); setFailure(''); }} /></div>

      <Dialog open={!!chosen} onOpenChange={o => !o && setChosen(undefined)} title={chosen?.name || platformName(chosen?.platform ?? '') || t('web.devices.device')} description={chosen ? t('web.devices.firstSeen', {date: new Date(chosen.firstSeen).toLocaleDateString()}) : undefined} width={480}
        actions={<><Button variant="ghost" label={t('action.close')} onClick={() => setChosen(undefined)} /><Button variant="primary" label={t('web.devices.saveName')} disabled={!chosen || name.trim() === chosen.name || !name.trim()} onClick={() => chosen && void act(() => identity.editDevice(chosen.id, {name: name.trim()}))} /></>}>
        {chosen ? (
          <div style={{display: 'flex', flexDirection: 'column', gap: 16}}>
            {failure ? <Notice tone="error" compact>{failure}</Notice> : null}
            <Input label={t('profile.name')} value={name} maxLength={80} onChange={e => setName(e.target.value)} />
            {deviceAwaitingApproval(chosen) ? (
              <Notice tone="warning" title={t('web.devices.approvalTitle')}>{t('web.devices.approvalBody')}
                <div style={{display: 'flex', gap: 8, marginTop: 12}}><Button size="sm" variant="primary" label={t('web.devices.approve')} onClick={() => void act(() => identity.approveDevice(chosen.id, true))} /><Button size="sm" variant="outline" label={t('device.deny')} onClick={() => void act(() => identity.approveDevice(chosen.id, false))} /></div>
              </Notice>
            ) : null}
            <SettingsGroup>
              <SettingsRow label={t('web.devices.trusted')} help={t('web.devices.trustedHelp')} control={<Switch checked={chosen.trusted} onCheckedChange={v => void act(() => identity.editDevice(chosen.id, {trusted: v}))} label={t('web.devices.trusted')} />} />
            </SettingsGroup>
            <div style={{display: 'flex', gap: 8, flexWrap: 'wrap'}}>
              <Button variant="outline" size="sm" icon="signOut" label={chosen.current ? t('device.signOutThis') : t('device.signOutOther').replace('…', '')} disabled={!chosen.sessions} onClick={() => void act(async () => { await identity.signOutDevice(chosen.id); if (chosen.current) session.signOut(); })} />
              {!chosen.current ? <Button variant="danger" size="sm" icon="trash" label={t('web.devices.removeAction')} onClick={() => setRemoving(chosen)} /> : null}
            </div>
          </div>
        ) : null}
      </Dialog>
      <ConfirmDialog open={!!removing} onOpenChange={o => !o && setRemoving(undefined)} title={t('confirm.removeDevice.title', {device: removing?.name || platformName(removing?.platform ?? '')})} body={t('confirm.removeDevice.body')} confirmLabel={t('web.devices.removeAction')} cancelLabel={t('action.cancel')} onConfirm={() => { const d = removing!; setRemoving(undefined); void act(() => identity.removeDevice(d.id)); }} />
      <Dialog open={everywhere} onOpenChange={setEverywhere} title={t('account.signOutEverywhere.title')} description={t('web.devices.everywhereBody')} width={440}
        actions={<><Button variant="ghost" label={t('action.cancel')} onClick={() => setEverywhere(false)} /><Button variant="danger" label={t('account.signOutEverywhere.action')} onClick={() => void act(async () => { await identity.signOutEverywhere(); setEverywhere(false); session.signOut(); })} /></>}>
        {/* C45: no password; the signed-in session is the authority. */}
        {failure ? <Notice tone="error" compact>{failure}</Notice> : null}
      </Dialog>
    </SettingsGroup>
  );
}

/* ---------- Two-factor ---------- */

export function TwoFactorSection() {
  const identity = useIdentity();
  const state = useLoaded<TwoFactorState>(s => identity.twoFactor(s), [identity], 'account');
  const [step, setStep] = useState<'idle' | 'password' | 'scan' | 'codes' | 'disable'>('idle');
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [enrolment, setEnrolment] = useState<TwoFactorEnrolment>();
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState('');
  const [confirmDisable, setConfirmDisable] = useState(false);
  const reset = () => { setStep('idle'); setPassword(''); setCode(''); setEnrolment(undefined); setFailure(''); };
  const run = async (work: () => Promise<void>) => { setBusy(true); setFailure(''); try { await work(); } catch (e) { setFailure(say(e, 'account', 'save')); } finally { setBusy(false); } };
  const on = state.value?.enabled;
  return (
    <SettingsGroup title={t('web.twoStep.title')} description={t('security.twoStep.offHelp')}>
      {state.error && !state.value ? <div style={{padding: 12}}><Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: state.reload}}>{state.error}</Notice></div> : null}
      {state.value ? (
        <SettingsRow label={on ? t('security.twoStep.on') : t('security.twoStep.off')} help={on ? t('security.twoStep.codesLeft', {count: state.value.recoveryCodesRemaining}) : t('web.twoStep.recommended')}
          control={on ? <Button size="sm" variant="secondary" label={t('security.twoStep.turnOffAction')} onClick={() => { reset(); setStep('disable'); }} /> : <Button size="sm" variant="secondary" label={t('web.twoStep.turnOn')} onClick={() => { reset(); setStep('password'); }} />} />
      ) : !state.error ? <div style={{padding: 16}}><Loading label={t('status.loadingThing', {thing: t('web.twoStep.title').toLowerCase()})} /></div> : null}

      <Dialog open={step === 'password'} onOpenChange={o => !o && reset()} title={t('web.twoStep.turnOnTitle')} description={t('security.twoStep.confirmPassword')} width={440}
        actions={<><Button variant="ghost" label={t('action.cancel')} onClick={reset} /><Button variant="primary" label={t('action.continue')} disabled={!password} loading={busy} onClick={() => void run(async () => { setEnrolment(await identity.enrolTwoFactor(password)); setStep('scan'); })} /></>}>
        <form onSubmit={e => e.preventDefault()} style={{display: 'flex', flexDirection: 'column', gap: 12}}>
          {failure ? <Notice tone="error" compact>{failure}</Notice> : null}
          <PasswordInput label={t('web.password')} autoComplete="current-password" autoFocus value={password} onChange={e => setPassword(e.target.value)} />
        </form>
      </Dialog>
      <Dialog open={step === 'scan'} onOpenChange={o => !o && reset()} title={t('web.twoStep.scanTitle')} description={t('web.twoStep.scanBody')} width={460}
        actions={<><Button variant="ghost" label={t('action.cancel')} onClick={reset} /><Button variant="primary" label={t('security.twoStep.verify')} disabled={code.length < 6} loading={busy} onClick={() => void run(async () => { const next = await identity.verifyTwoFactor(password, code); state.setValue(next); setStep('codes'); })} /></>}>
        {enrolment ? (
          <div style={{display: 'flex', flexDirection: 'column', gap: 16, alignItems: 'center'}}>
            {failure ? <Notice tone="error" compact>{failure}</Notice> : null}
            <div style={{background: 'var(--surface-qr)', padding: 12, borderRadius: 12}}><QR value={enrolment.uri} size={176} /></div>
            <Text variant="caption" tone="tertiary" center>{t('web.twoStep.cantScan')} <Text variant="mono">{enrolment.secret.replace(/(.{4})/g, '$1 ').trim()}</Text></Text>
            <Input label={t('web.twoStep.code')} inputMode="numeric" autoComplete="one-time-code" autoFocus value={code} onChange={e => setCode(e.target.value.replace(/\D/g, '').slice(0, 8))} style={{width: 160, textAlign: 'center'}} />
          </div>
        ) : null}
      </Dialog>
      <Dialog open={step === 'codes'} onOpenChange={o => !o && reset()} dismissable={false} title={t('confirm.recoveryCodes.title')} description={t('confirm.recoveryCodes.body')} width={460}
        actions={<><Button variant="secondary" icon="copy" label={t('web.twoStep.copy')} onClick={() => void navigator.clipboard?.writeText((enrolment?.recoveryCodes ?? []).join('\n'))} /><Button variant="primary" label={t('confirm.recoveryCodes.action')} onClick={reset} /></>}>
        <div style={{display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 8}}>{(enrolment?.recoveryCodes ?? []).map(c => <Text key={c} variant="mono" style={{padding: '8px 12px', background: 'var(--surface-recess, var(--surface-recovery-code))', borderRadius: 8, textAlign: 'center'}}>{c}</Text>)}</div>
      </Dialog>
      <Dialog open={step === 'disable'} onOpenChange={o => !o && reset()} title={t('web.twoStep.turnOffTitle')} description={t('web.twoStep.confirmPasswordShort')} width={440}
        actions={<><Button variant="ghost" label={t('action.cancel')} onClick={reset} /><Button variant="danger" label={t('security.twoStep.turnOffAction')} disabled={!password || code.length < 6} loading={busy} onClick={() => setConfirmDisable(true)} /></>}>
        <form onSubmit={e => e.preventDefault()} style={{display: 'flex', flexDirection: 'column', gap: 12}}>
          {failure ? <Notice tone="error" compact>{failure}</Notice> : null}
          <PasswordInput label={t('web.password')} autoComplete="current-password" autoFocus value={password} onChange={e => setPassword(e.target.value)} />
          {/* C45: turning it off needs the current password and a current code. */}
          <Input label={t('security.authenticatorCode')} inputMode="numeric" autoComplete="one-time-code" value={code} onChange={e => setCode(e.target.value.replace(/\s/g, '').slice(0, 12))} />
        </form>
      </Dialog>
      <ConfirmDialog open={confirmDisable} onOpenChange={setConfirmDisable} title={t('web.twoStep.turnOffConfirmTitle')} body={t('security.twoStep.turnOffBody')} confirmLabel={t('security.twoStep.turnOffAction')} cancelLabel={t('action.cancel')} onConfirm={() => void run(async () => { await identity.disableTwoFactor(password, code); setConfirmDisable(false); reset(); state.reload(); })} />
    </SettingsGroup>
  );
}

/** CON-12: a device row shows what the device is: TV, tablet, phone or browser. */
function deviceGlyph(platform: string | undefined, name?: string): IconName {
  if (platform === 'tvos' || platform === 'androidtv' || platform === 'tizen' || platform === 'webos' || platform === 'roku') return 'tvDevice';
  if (platform === 'web') return 'browser';
  if (platform === 'ipados' || /ipad|tablet/i.test(name ?? '')) return 'tablet';
  if (platform === 'ios' || platform === 'android') return 'phone';
  return 'browser';
}
