import {useEffect, useRef, useState} from 'react';
import {viewer, useSession} from '../../app/session';
import {currentI18n} from '../../app/i18n';
import {Button, Checkbox, Input, Loading, Notice} from '../../ui';
import {ProfileGrid} from './ProfileGrid';
import s from './Auth.module.css';

/** Profiles for a direct (server-local) account. */
export function DirectProfiles({onDone}: {onDone?: () => void}) {
  const session = useSession();
  const t = currentI18n().t;
  const pending = session.direct.pending;
  const [choice, setChoice] = useState<string>();
  const [dismissed, setDismissed] = useState(false);
  const [pin, setPin] = useState('');
  const [trust, setTrust] = useState(false);
  const entered = useRef<readonly [object, string] | undefined>(undefined);
  const profiles = pending?.snapshot?.profiles ?? [];
  const only = pending?.snapshot && profiles.length === 1 && profiles[0].pinRequired === false ? profiles[0] : undefined;
  const solePin = !dismissed && profiles.length === 1 && profiles[0].pinRequired === true ? profiles[0] : undefined;
  const current = profiles.find(p => p.id === choice) ?? solePin;
  const submit = (id: string, withPin?: string) => {
    void session.direct.chooseProfile(id, {pin: withPin, trust}).then(() => {
      if (viewer.getSnapshot().session?.viewer.profileId === id) onDone?.();
    });
    setPin('');
  };
  useEffect(() => {
    if (!pending?.snapshot || !only) return;
    if (entered.current?.[0] === pending && entered.current[1] === only.id) return;
    entered.current = [pending, only.id];
    submit(only.id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pending]);
  return (
    <div className={s.stack}>
      <p className={s.lede} style={{textAlign: 'center'}}>Who’s watching{pending?.snapshot ? <> as <strong>{pending.snapshot.account.username}</strong></> : null}?</p>
      {pending ? (
        current ? (
          <form className={s.pin} onSubmit={e => { e.preventDefault(); submit(current.id, pin); }}>
            <div style={{display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 8, textAlign: 'center'}}>
              <span className={s.profileArt} style={{width: 40, height: 40, fontSize: 16, background: `var(--profile-art-${current.art})`}}>{current.name.slice(0, 1).toUpperCase()}</span>
              <strong>{current.name}</strong>
            </div>
            <div style={{display: 'flex', flexDirection: 'column', gap: 12, width: '100%', maxWidth: 280, margin: '0 auto'}}>
              <Input label={t('web.direct.profilePin')} type="password" inputMode="numeric" autoComplete="off" pattern="[0-9]{4}" maxLength={4} value={pin} onChange={e => setPin(e.target.value.replace(/\D/g, ''))} autoFocus code />
              <Checkbox checked={trust} onCheckedChange={setTrust} label={t('web.direct.rememberProfile')} />
              <div style={{display: 'flex', gap: 8, justifyContent: 'center'}}>
                <Button variant="ghost" label={t('web.direct.otherProfiles')} onClick={() => { setChoice(undefined); setDismissed(true); }} disabled={session.busy} />
                <Button variant="primary" type="submit" label={t('action.continue')} loading={session.busy} />
              </div>
            </div>
          </form>
        ) : !pending.snapshot ? (
          pending.profilesError ? (
            <Notice tone="error" action={{label: t('action.tryAgain'), onClick: session.direct.reloadProfiles}}>{pending.profilesError}</Notice>
          ) : (
            <Loading label={t('auth.loadingProfiles')} />
          )
        ) : only && !session.error ? (
          <Loading label={t('web.direct.connecting')} />
        ) : (
          <ProfileGrid items={profiles.map(p => ({id: p.id, name: p.name, eligible: true, art: p.art, primary: p.primary, pinRequired: p.pinRequired}))} busy={session.busy} onPick={p => (p.pinRequired ? (setChoice(p.id), setPin(''), setDismissed(false)) : submit(p.id))} />
        )
      ) : null}
      {session.error ? <Notice tone="error">{session.error}</Notice> : null}
      {pending?.accountGrant ? <Button variant="ghost" label={t('action.cancel')} onClick={session.direct.cancelPending} /> : null}
    </div>
  );
}
