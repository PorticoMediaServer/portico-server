import {useEffect, useState} from 'react';
import {deleteMember, parseAccessMember, parseLimitsDocument, previewMemberDeletion, type AccessMember, type AccessTier, type MemberDeletionPreview, type MemberLimits} from '@core/server-administration.ts';
import {memberOffLine, readAccessRecord, type DirectMember} from '@core/server-admin/panel-logic.ts';
import {problem, sendFresh, useAction, useRead} from '../../admin/console';
import {nextCursorOf, useCursorList} from '../../admin/cursor-read';
import {roleLabel} from '@core/server-admin/people-words.ts';
import {isStaleAnswer, rebaseDraft} from '@core/freshness-actions.ts';
import {createOperationIds} from './operation-ids';
import {ListPager} from './ListPager';
import {AccountDevices, LimitsFields, accessListPath} from './Access';
import {useInlineForm} from '../settings/ServerForms';
import {useLibrariesContext} from '../../app/libraries';
import {useSession} from '../../app/session';
import {useNavigate, useSearch} from '@tanstack/react-router';
import {Badge, Button, Checkbox, Dialog, Input, Loading, Notice, PasswordInput, Select, SettingsGroup, SettingsLoading, SettingsNote, SettingsRow, Text, PasswordStrength} from '../../ui';
import {passwordStrength} from '@core/password-strength.ts';
import {SectionHeader} from './Server';
import {useI18n} from '../../app/i18n';
import {dependsOn} from '@core/freshness-hub.ts';

const MEMBER_MOVES = [dependsOn.list('members')];

/**
 * People › Accounts: one list of everyone who can use this server (Portico Account members and
 * server accounts alike). An account opens to its role, libraries, sign-in, limits and devices;
 * invitations and API keys are the page's other two sub-pages. A person's own profiles are in
 * Settings › Profile & account, not here.
 */
export function AccountsPanel() {
  const {owner} = useSession();
  return owner ? <OwnerAccounts /> : <AdministeredAccounts />;
}

/** Turns an access record into the account shape the detail page draws (an administrator reads no other). */
const administered = (a: AccessMember): DirectMember => ({id: a.accountId, username: a.username, primaryProfileId: a.profileId, role: a.role, revision: a.revision, disabled: a.disabled, allowedLibraries: [...a.allowedLibraries], ...(a.disabledAt ? {disabledAt: a.disabledAt} : {}), ...(a.disabledReason ? {disabledReason: a.disabledReason} : {})});

/**
 * People › Accounts for an administrator: everyone, from the access list the administrator tier
 * may read. An account opens to its limits (for the members below them) and its devices; its
 * sign-in, libraries, role and removal stay the owner's, as the server has it.
 */
function AdministeredAccounts() {
  const i18n = useI18n();
  const {t} = i18n;
  const {api} = useSession();
  const navigate = useNavigate();
  const search = useSearch({from: '/app/settings/$section'});
  const libraries = useLibrariesContext();
  const accessPage = async (cursor: string) => { const raw = await api.request(accessListPath('members', cursor)); const list = (raw as {items?: unknown[]}).items ?? []; return {items: list.map(parseAccessMember), nextCursor: nextCursorOf(raw)}; };
  const access = useCursorList(accessPage, [api], {on: MEMBER_MOVES});
  const members = access.data ?? [];
  const libName = (id: string) => libraries.items.find(l => l.id === id)?.name ?? t('server.people.libraryGone');
  const open = (id?: string) => void navigate({to: '.', search: id ? {id} : {}});
  const selected = search.id ? members.find(m => m.accountId === search.id) : undefined;
  if (search.id && selected) return <AccountDetail administered member={administered(selected)} access={selected} libraries={libraries.items} onBack={() => open()} onChanged={access.reload} onDeleted={() => { open(); access.reload(); }}
    readMember={async () => { const now = await readAccessRecord(api, 'members', selected.accountId, parseAccessMember); return now ? administered(now) : undefined; }} readAccess={() => readAccessRecord(api, 'members', selected.accountId, parseAccessMember)} />;
  return (
    <SettingsGroup title={t('web.people.accountsTitle')} description={t('web.people.accountsLede')}>
      {access.error ? <SettingsNote><Notice tone={access.data ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: access.reload}}>{access.error}</Notice></SettingsNote> : null}
      {!access.data && access.loading ? <SettingsLoading thing={t('server.thing.accounts')} /> : null}
      {members.map(m => {
        const facts = [memberOffLine(m, t, iso => i18n.date(iso)), m.role === 'owner' || !m.allowedLibraries.length ? t('server.people.allLibraries') : m.allowedLibraries.map(libName).join(', '), m.limits.maxStreams ? t('server.people.streams', {count: m.limits.maxStreams}) : '', m.limits.maxContentRating ? t('server.people.upTo', {rating: m.limits.maxContentRating}) : ''];
        return <SettingsRow key={m.accountId} icon="profile" label={m.username} help={facts.filter(Boolean).join(' · ')} meta={m.disabled ? <Badge tone="danger">{t('web.people.disabledBadge')}</Badge> : <Badge tone={m.role === 'owner' ? 'accent' : 'neutral'}>{roleLabel(t, m.role)}</Badge>} onClick={() => open(m.accountId)} />;
      })}
      <ListPager pages={access} />
    </SettingsGroup>
  );
}

function OwnerAccounts() {
  const i18n = useI18n();
  const {t} = i18n;
  const {api} = useSession();
  const navigate = useNavigate();
  const search = useSearch({from: '/app/settings/$section'});
  const libraries = useLibrariesContext();
  // Someone joining with an invitation, leaving, or being turned off elsewhere shows here by itself.
  const read = useRead<{items: DirectMember[]}>(() => api.request('/v1/direct/members'), [api], {on: MEMBER_MOVES});
  // Roles and limits come from the access list, joined by account.
  const accessPage = async (cursor: string) => { const raw = await api.request(accessListPath('members', cursor)); const list = (raw as {items?: unknown[]}).items ?? []; return {items: list.map(parseAccessMember), nextCursor: nextCursorOf(raw)}; };
  const access = useCursorList(accessPage, [api], {on: MEMBER_MOVES});
  const action = useAction();
  const [adding, setAdding] = useState(false);
  const members = read.data?.items ?? [];
  const libName = (id: string) => libraries.items.find(l => l.id === id)?.name ?? t('server.people.libraryGone');
  const open = (id?: string) => void navigate({to: '.', search: id ? {id} : {}});
  const selected = search.id ? members.find(m => m.id === search.id) : undefined;
  if (search.id && selected) return <AccountDetail member={selected} access={access.data?.find(a => a.accountId === selected.id)} libraries={libraries.items} onBack={() => open()} onChanged={() => { read.reload(); access.reload(); }} onDeleted={() => { open(); read.reload(); access.reload(); }}
    readMember={async () => (await api.request<{items: DirectMember[]}>('/v1/direct/members')).items.find(m => m.id === selected.id)} readAccess={() => readAccessRecord(api, 'members', selected.id, parseAccessMember)} />;
  return (
    <>
      <SettingsGroup title={t('web.people.accountsTitle')} description={t('web.people.accountsLede')} action={<Button size="sm" variant="secondary" icon="plus" label={t('web.people.addAccount')} onClick={() => { action.clear(); setAdding(true); }} />}>
        {read.error ? <SettingsNote><Notice tone={read.data ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
        {action.notice ? <SettingsNote><Notice tone="success" compact>{action.notice}</Notice></SettingsNote> : null}
        {!read.data && read.loading ? <SettingsLoading thing={t('server.thing.accounts')} /> : null}
        {members.map(m => {
          const limits = access.data?.find(a => a.accountId === m.id)?.limits;
          const facts = [memberOffLine(m, t, iso => i18n.date(iso)), m.hostedAccountId ? t('server.people.porticoAccount') : '', m.role === 'owner' ? t('server.people.allLibraries') : m.allowedLibraries.length ? m.allowedLibraries.map(libName).join(', ') : t('server.people.allLibraries'), limits?.maxStreams ? t('server.people.streams', {count: limits.maxStreams}) : '', limits?.maxContentRating ? t('server.people.upTo', {rating: limits.maxContentRating}) : ''];
          return <SettingsRow key={m.id} icon="profile" label={m.username} help={facts.filter(Boolean).join(' · ')} meta={m.disabled ? <Badge tone="danger">{t('web.people.disabledBadge')}</Badge> : <Badge tone={m.role === 'owner' ? 'accent' : 'neutral'}>{roleLabel(t, m.role)}</Badge>} onClick={() => open(m.id)} />;
        })}
        <ListPager pages={access} />
      </SettingsGroup>
      <LocalMemberDialog open={adding} member={null} libraries={libraries.items} onClose={() => setAdding(false)} busy={action.busy} error={action.error} onSave={input => void action.run(() => api.request('/v1/direct/members', 'POST', input), t('server.people.accountAdded')).then(ok => { if (ok) { setAdding(false); read.reload(); access.reload(); } })} />
    </>
  );
}

/** One account: who it is, what it may do, and the devices signed in with it. Limits save with the page. */
function AccountDetail({member, access, libraries, onBack, onChanged, onDeleted, readMember, readAccess, administered = false}: {member: DirectMember; access?: AccessMember; libraries: readonly {id: string; name: string}[]; onBack: () => void; onChanged: () => void; onDeleted: () => void; /** The account, and its role record, as the server has them now: read again before a change is sent again. */ readMember: () => Promise<DirectMember | undefined>; readAccess: () => Promise<AccessMember | undefined>; /** Opened by an administrator: limits and devices of the members below them; the rest is shown, not changed. */ administered?: boolean}) {
  const i18n = useI18n();
  const {t} = i18n;
  const [deleting, setDeleting] = useState(false);
  const {api} = useSession();
  const action = useAction();
  const [opIds] = useState(createOperationIds);
  const [editing, setEditing] = useState(false);
  const owner = member.role === 'owner';
  // An administrator manages strictly below themselves (members); an owner manages everyone but the owner.
  const manages = administered ? member.role === 'member' : !owner;
  const readLimits = async () => parseLimitsDocument(await api.request(`/v1/admin/access/members/${encodeURIComponent(member.id)}/limits`), member.id);
  const doc = useRead(readLimits, [api, member.id], {auto: manages});
  const [limits, setLimits] = useState<MemberLimits>();
  // What the draft was last compared with: a newer document is taken under the owner's own edits, never over them.
  const [basis, setBasis] = useState<MemberLimits>();
  useEffect(() => {
    if (!doc.data) return;
    const next = doc.data.limits;
    setLimits(current => (current && basis ? rebaseDraft(basis, current, next).merged : next));
    setBasis(next);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [doc.data]);
  const inline = useInlineForm('limits:' + member.id, {
    dirty: !!basis && !!limits && JSON.stringify(limits) !== JSON.stringify(basis),
    discard: () => setLimits(basis),
    save: async () => {
      if (!doc.data || !limits || !basis) return;
      const put = (revision: number, value: MemberLimits) => api.request(`/v1/admin/access/members/${encodeURIComponent(member.id)}/limits`, 'PUT', {expectedRevision: revision, operationId: opIds.forPayload(JSON.stringify({id: member.id, revision, limits: value})), limits: value});
      try { await put(doc.data.revision, limits); }
      catch (e) {
        if (!isStaleAnswer(e)) throw e;
        // The account moved (its role, or a limit set elsewhere): carry these edits onto it and save again.
        const fresh = await readLimits().catch(() => { throw e; });
        const rebased = rebaseDraft(basis, limits, fresh.limits);
        setBasis(fresh.limits); setLimits(rebased.merged);
        if (rebased.conflict) throw e;
        await put(fresh.revision, rebased.merged);
      }
      opIds.release(); doc.reload(); onChanged();
    },
  });
  // A role decides what someone may do on the server, so choosing one asks first instead of taking effect as the menu closes.
  const [askRole, setAskRole] = useState<AccessTier>();
  const setRole = (role: AccessTier) => access && void action.run(async () => {
    await sendFresh(access,
      a => api.request(`/v1/admin/access/members/${encodeURIComponent(member.id)}/role`, 'PUT', {expectedRevision: a.revision, operationId: opIds.forPayload(JSON.stringify({id: member.id, revision: a.revision, role})), role}),
      readAccess, now => (now.role === role ? 'done' : now.role === 'owner' ? false : true));
    opIds.release(); onChanged();
  });
  // Saving the account's sign-in and libraries: when the account moved underneath, it is sent again only if what this dialog showed is still what the server has.
  const saveMember = (input: {username: string; password: string; name: string; allowedLibraries: string[]; disabled: boolean}) => action.run(async () => {
    await sendFresh(member,
      m => api.request(`/v1/direct/members/${encodeURIComponent(member.id)}`, 'PATCH', {...input, expectedRevision: m.revision}),
      readMember, (now, was) => now.disabled === was.disabled && JSON.stringify([...now.allowedLibraries].sort()) === JSON.stringify([...was.allowedLibraries].sort()));
  });
  const libName = (id: string) => libraries.find(l => l.id === id)?.name ?? t('server.people.libraryGone');
  return (
    <>
      <SectionHeader title={member.username} lede={member.hostedAccountId ? t('server.people.porticoAccount') : t('server.people.serverAccount')} actions={<Button size="sm" variant="ghost" icon="back" label={t('settings.server.accounts')} onClick={onBack} />} />
      {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
      {inline.error ? <Notice tone="error" compact>{problem(inline.error, 'action')}</Notice> : null}
      <SettingsGroup>
        <SettingsRow label={t('web.access.roleLabel')} control={owner || administered || !access ? <Badge tone="accent">{roleLabel(t, member.role)}</Badge> : <Select hideLabel label={t('web.access.roleFor', {name: member.username})} value={access.role} disabled={action.busy} onChange={e => { const next = e.target.value as AccessTier; if (next !== access.role) setAskRole(next); }} options={[{value: 'member', label: t('web.access.roleMember')}, {value: 'admin', label: t('web.access.roleAdmin')}]} />} />
        <SettingsRow label={t('web.people.visibleLibraries')} meta={owner || !member.allowedLibraries.length ? t('server.people.allLibraries') : member.allowedLibraries.map(libName).join(', ')} control={!owner && !administered ? <Button size="sm" variant="ghost" label={t('web.selection.edit')} onClick={() => { action.clear(); setEditing(true); }} /> : undefined} />
        {!owner && !administered ? <SettingsRow label={t('server.people.signIn')} help={member.hostedAccountId ? t('web.people.hostedNote') : t('server.people.signInHelp')} state={memberOffLine(member, t, iso => i18n.date(iso)) || undefined} meta={member.disabled ? <Badge tone="danger">{t('web.people.disabledBadge')}</Badge> : undefined} control={<Button size="sm" variant="ghost" label={t('web.selection.edit')} onClick={() => { action.clear(); setEditing(true); }} />} /> : null}
      </SettingsGroup>
      {doc.error ? <Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: doc.reload}}>{doc.error}</Notice> : null}
      {limits && manages ? <LimitsFields limits={limits} onChange={setLimits} /> : null}
      <AccountDevices accountId={member.id} />
      {/* The last thing on the page, and never offered for the owner: the dialog reads what goes before it asks. */}
      {!owner && !administered ? <SettingsGroup>
        <SettingsRow label={t('server.people.delete')} help={t('server.people.deleteHelp')} control={<Button size="sm" variant="secondary" label={t('server.people.delete')} onClick={() => setDeleting(true)} />} />
      </SettingsGroup> : null}
      {deleting ? <DeleteMemberDialog member={member} onClose={() => setDeleting(false)} onDeleted={onDeleted} /> : null}
      <Dialog open={!!askRole} onOpenChange={o => !o && setAskRole(undefined)} title={t('web.access.roleConfirmTitle', {name: member.username})} description={t(askRole === 'admin' ? 'web.access.roleConfirmAdmin' : 'web.access.roleConfirmMember', {name: member.username})} width={480}
        actions={<><Button variant="ghost" label={t('action.cancel')} onClick={() => setAskRole(undefined)} /><Button variant="primary" label={t('web.access.roleConfirm')} onClick={() => { const role = askRole; setAskRole(undefined); if (role) setRole(role); }} /></>} />
      <LocalMemberDialog open={editing} member={member} libraries={libraries} onClose={() => setEditing(false)} busy={action.busy} error={action.error} onSave={input => void saveMember(input).then(ok => { if (ok) { setEditing(false); onChanged(); } })} />
    </>
  );
}

function LibraryPicker({all, chosen, libraries, onChange}: {all: boolean; chosen: string[]; libraries: readonly {id: string; name: string}[]; onChange: (all: boolean, chosen: string[]) => void}) {
  const {t} = useI18n();
  return (
    <div style={{display: 'flex', flexDirection: 'column', gap: 8}}>
      <Checkbox checked={all} onCheckedChange={v => onChange(v, v ? [] : chosen)} label={t('web.people.allLibrariesLater')} />
      {!all ? libraries.map(l => <Checkbox key={l.id} checked={chosen.includes(l.id)} onCheckedChange={v => onChange(false, v ? [...chosen, l.id] : chosen.filter(x => x !== l.id))} label={l.name} />) : null}
    </div>
  );
}

function LocalMemberDialog({open, member, libraries, onClose, onSave, busy, error}: {open: boolean; member: DirectMember | null; libraries: readonly {id: string; name: string}[]; onClose: () => void; onSave: (input: {username: string; password: string; name: string; allowedLibraries: string[]; disabled: boolean}) => void; busy: boolean; error: string}) {
  const {t} = useI18n();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [name, setName] = useState('');
  const [all, setAll] = useState(true);
  const [chosen, setChosen] = useState<string[]>([]);
  const [disabled, setDisabled] = useState(false);
  useEffect(() => {
    if (open) {
      setUsername(member?.username ?? '');
      setPassword('');
      setName('');
      setAll(!member || !member.allowedLibraries.length);
      setChosen(member?.allowedLibraries ?? []);
      setDisabled(member?.disabled ?? false);
    }
    // Filled when the dialog opens for an account; the list reading itself again must not clear what is being typed.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, member?.id]);
  return (
    <Dialog open={open} onOpenChange={o => !o && onClose()} title={member ? t('web.people.editAccountTitle', {name: member.username}) : t('web.people.addAccountTitle')} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} /><Button variant="primary" label={member ? t('action.save') : t('web.people.addAccount')} loading={busy} disabled={!username.trim() || (!member && (!passwordStrength(password).acceptable || !name.trim())) || (!!member && !!password && !passwordStrength(password).acceptable) || (!all && !chosen.length)} onClick={() => onSave({username: username.trim(), password, name: name.trim() || username.trim(), allowedLibraries: all ? [] : chosen, disabled})} /></>}>
      <Input label={t('web.direct.username')} value={username} onChange={e => setUsername(e.target.value)} autoCapitalize="off" spellCheck={false} disabled={!!member} minLength={3} maxLength={64} autoFocus />
      {!member ? <Input label={t('web.people.profileName')} value={name} onChange={e => setName(e.target.value)} maxLength={120} help={t('web.people.profileNameHelp')} /> : null}
      {member?.hostedAccountId ? <Text variant="caption" tone="secondary">{t('web.people.hostedNote')}</Text> : (
        <>
          <PasswordInput label={member ? t('auth.newPassword') : t('auth.password')} optional={!!member} value={password} onChange={e => setPassword(e.target.value)} autoComplete="new-password" aria-describedby="member-password-strength" help={member ? t('web.people.keepPasswordHelp') : undefined} />
          {!member || password ? <PasswordStrength id="member-password-strength" password={password} /> : null}
        </>
      )}
      <Text variant="label" tone="tertiary">{t('web.people.visibleLibraries')}</Text>
      <LibraryPicker all={all} chosen={chosen} libraries={libraries} onChange={(a, c) => { setAll(a); setChosen(c); }} />
      {member ? <Checkbox checked={disabled} onCheckedChange={setDisabled} label={t('web.people.disableAccount')} help={t('web.people.disableAccountHelp')} /> : null}
      {error ? <Notice tone="error" compact>{error}</Notice> : null}
    </Dialog>
  );
}

/**
 * Delete a person: the server's own preview is the confirmation. It says what goes (the account,
 * its profiles and what they saved, its sign-ins) and what stays (the libraries, recordings, and
 * the plays in the owner's history, without the name). If the account changed while the dialog
 * was open the server refuses, and the preview is read again before asking again.
 */
function DeleteMemberDialog({member, onClose, onDeleted}: {member: DirectMember; onClose: () => void; onDeleted: () => void}) {
  const {t} = useI18n();
  const {api} = useSession();
  const [preview, setPreview] = useState<MemberDeletionPreview>();
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const read = () => { setError(''); previewMemberDeletion(api, member.id).then(setPreview, e => setError(problem(e))); };
  useEffect(read, [api, member.id]); // eslint-disable-line react-hooks/exhaustive-deps
  const confirm = () => {
    if (!preview) return;
    setBusy(true); setError('');
    deleteMember(api, preview).then(onDeleted, e => {
      setBusy(false);
      if ((e as {code?: string} | null)?.code === 'revision_mismatch') { setPreview(undefined); setError(t('server.people.deleteChanged')); previewMemberDeletion(api, member.id).then(setPreview, () => {}); }
      else setError(problem(e, 'action'));
    });
  };
  return (
    <Dialog open onOpenChange={o => !o && !busy && onClose()} dismissable={!busy} title={t('server.people.deleteTitle', {name: member.username})} width={500}
      actions={<><Button variant="ghost" label={t('action.cancel')} disabled={busy} onClick={onClose} /><Button variant="danger" label={t('server.people.deleteConfirm')} disabled={!preview} loading={busy} onClick={confirm} /></>}>
      {error ? <Notice tone="error" compact action={preview ? undefined : {label: t('action.tryAgain'), onClick: read}}>{error}</Notice> : null}
      {!preview && !error ? <Loading label={t('server.people.delete')} /> : null}
      {preview ? (
        <div style={{display: 'flex', flexDirection: 'column', gap: 8}}>
          <Text as="p" variant="body" tone="secondary">{t('server.people.deleteBody', {name: preview.username || member.username, profiles: preview.profiles, devices: preview.devices})}</Text>
          {preview.playsKept ? <Text as="p" variant="body" tone="secondary">{t('server.people.deleteHistoryKept', {count: preview.playsKept})}</Text> : null}
          {preview.porticoAccount ? <Text as="p" variant="body" tone="secondary">{t('server.people.deletePorticoAccount')}</Text> : null}
        </div>
      ) : null}
    </Dialog>
  );
}
