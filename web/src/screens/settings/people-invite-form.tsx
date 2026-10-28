import {defaultI18n} from '@i18n';
import {RichSelect, TextField} from '../../kit/controls';
import {SettingsPanel} from '../../kit/settings';
import styles from './people-invite.module.css';
import type {InviteAttempt, InviteChoices} from './people-invite';
import {expiryChoices} from './people-invite';

const t = defaultI18n.t;

/** The drawn Invite someone form (owner-people frames 2/23b/11): role, libraries,
 * expiry, optional Portico Account username and private notes. It writes nothing
 * until Create. While an uncertain create is being reconciled it shows that
 * attempt's original choices with Retry or Dismiss, so an edited form can never
 * submit an old body. */
export function PeopleInviteForm({open, onOpenChange, choices, onChoices, libraries, signIn, audience, busy, error, retained, onCreate, onRetry}: Readonly<{
  open: boolean; onOpenChange: (open: boolean) => void; choices: InviteChoices; onChoices: (choices: InviteChoices) => void;
  libraries: readonly {id: string; name: string}[]; signIn?: 'local' | 'portico'; audience?: string;
  busy: boolean; error: unknown; retained?: InviteAttempt;
  onCreate: () => void; onRetry: () => void;
}>) {
  const locked = !!retained && !busy;
  const shown: InviteChoices = retained
    ? {role: retained.body.role === 'admin' ? 'admin' : 'member', allLibraries: retained.body.allowedLibraries.length === 0,
       libraryIds: retained.body.allowedLibraries, expiresInHours: retained.body.expiresInHours,
       ...(retained.body.notes === undefined ? {} : {notes: retained.body.notes}),
       ...(retained.body.porticoUsername === undefined ? {} : {porticoUsername: retained.body.porticoUsername})}
    : choices;
  const allChecked = shown.allLibraries;
  const set = (patch: Partial<InviteChoices>) => onChoices({...choices, ...patch});
  return <SettingsPanel open={open} onClose={() => {if (!busy) onOpenChange(false);}} title={t('next.settings.people.inviteTitle')}
    audience={audience ?? t('next.shell.serverVisibility.ownerShared')} dismissOnBack={!busy} showCloseButton={!busy}
    create={{label: t(retained ? 'next.settings.people.retryCreate' : 'next.settings.people.create'),
      cancelLabel: t(retained ? 'next.settings.people.dismiss' : 'next.settings.cancel'), pending: busy,
      disabled: busy || (!retained && !shown.allLibraries && !shown.libraryIds.length), onCreate: retained ? onRetry : onCreate}}>
    <div className={styles.form}>
      {retained && <p role="alert">{t('next.settings.people.unsureCreate')}</p>}
      {!retained && error ? <p role="alert">{t('next.settings.people.createFailed')}</p> : null}
      <fieldset disabled={locked || busy} className={styles.fieldset}>
        <legend>{t('next.settings.people.role')}</legend>
        {(['member', 'admin'] as const).map(role => <label key={role} className={styles.choice}>
          <input type="radio" name="invite-role" checked={shown.role === role} onChange={() => set({role})} />
          <span>{t(role === 'admin' ? 'next.settings.people.adminRole' : 'next.settings.people.memberRole')}</span>
        </label>)}
      </fieldset>
      <fieldset disabled={locked || busy} className={styles.fieldset}>
        <legend>{t('next.settings.people.libraries')}</legend>
        <label className={styles.choice}>
          <input type="checkbox" checked={allChecked} onChange={event => {
            set({allLibraries: event.currentTarget.checked, ...(event.currentTarget.checked ? {libraryIds: []} : {})});
          }} />
          <span>{t('next.settings.people.allLibraries')}</span>
        </label>
        {!allChecked && libraries.map(library => <label key={library.id} className={styles.choice}>
          <input type="checkbox" checked={shown.libraryIds.includes(library.id)} onChange={event => {
            const next = event.currentTarget.checked ? [...shown.libraryIds, library.id] : shown.libraryIds.filter(id => id !== library.id);
            set({allLibraries: false, libraryIds: next});
          }} />
          <span>{library.name}</span>
        </label>)}
      </fieldset>
      <RichSelect label={t('next.settings.people.expiry')} value={String(shown.expiresInHours)} disabled={locked || busy}
        options={expiryChoices.map(hours => ({value: String(hours), label: t('next.settings.people.expiryDays', {count: hours / 24})}))}
        onValueChange={value => set({expiresInHours: Number(value)})} />
      {signIn === 'portico' && <TextField label={t('next.settings.people.porticoUsername')} description={t('next.settings.people.porticoUsernameHelp')}
        disabled={locked || busy} value={shown.porticoUsername ?? ''} onChange={event => set({porticoUsername: event.currentTarget.value})} />}
      <TextField label={t('next.settings.people.notes')} description={t('next.settings.people.notesHelp')} disabled={locked || busy}
        value={shown.notes ?? ''} onChange={event => set({notes: event.currentTarget.value})} />
    </div>
  </SettingsPanel>;
}
