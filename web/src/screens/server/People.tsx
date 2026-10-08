import {useEffect, useState} from 'react';
import {parseAccessMember, parseLimitsDocument, type AccessMember, type AccessTier, type MemberLimits} from '@core/server-administration.ts';
import type {DirectMember} from '@core/server-admin/panel-logic.ts';
import {problem, useAction, useRead} from '../../admin/console';
import {nextCursorOf, useCursorList} from '../../admin/cursor-read';
import {createOperationIds} from './operation-ids';
import {ListPager} from './ListPager';
import {AccountDevices, LimitsFields, accessListPath, tierLabels} from './Access';
import {useInlineForm} from '../settings/ServerForms';
import {useLibrariesContext} from '../../app/libraries';
import {useSession} from '../../app/session';
import {currentI18n} from '../../app/i18n';
import {useNavigate, useSearch} from '@tanstack/react-router';
import {Badge, Button, Checkbox, Dialog, Input, Notice, PasswordInput, Select, SettingsGroup, SettingsRow, Text, PasswordStrength} from '../../ui';
import {passwordStrength} from '@core/password-strength.ts';
import {SectionHeader} from './Server';

/**
 * People › Accounts: one list of everyone who can use this server (Portico Account members and
 * server accounts alike). An account opens to its role, libraries, sign-in, limits and devices;
 * invitations and API keys are the page's other two sub-pages. A person's own profiles are in
 * Settings › Profile & account, not here.
 */
export function AccountsPanel() {
  const {api} = useSession();
  const t = currentI18n().t;
  const navigate = useNavigate();
  const search = useSearch({from: '/app/settings/$section'});
  const libraries = useLibrariesContext();
  const read = useRead<{items: DirectMember[]}>(() => api.request('/v1/direct/members'), [api]);
  // Roles and limits come from the access list, joined by account.
  const access = useCursorList(async cursor => { const raw = await api.request(accessListPath('members', cursor)); const list = (raw as {items?: unknown[]}).items ?? []; return {items: list.map(parseAccessMember), nextCursor: nextCursorOf(raw)}; }, [api]);
  const action = useAction();
  const [adding, setAdding] = useState(false);
  const members = read.data?.items ?? [];
  const libName = (id: string) => libraries.items.find(l => l.id === id)?.name ?? t('server.people.libraryGone');
  const open = (id?: string) => void navigate({to: '.', search: id ? {id} : {}});
  const selected = search.id ? members.find(m => m.id === search.id) : undefined;
  if (search.id && selected) return <AccountDetail member={selected} access={access.data?.find(a => a.accountId === selected.id)} libraries={libraries.items} onBack={() => open()} onChanged={() => { read.reload(); access.reload(); }} />;
  return (
    <>
      <SettingsGroup title={t('web.people.accountsTitle')} description={t('web.people.accountsLede')} action={<Button size="sm" variant="primary" icon="plus" label={t('web.people.addAccount')} onClick={() => setAdding(true)} />}>
        {read.error ? <div style={{padding: 12}}><Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></div> : null}
        {members.map(m => {
          const limits = access.data?.find(a => a.accountId === m.id)?.limits;
          const facts = [m.hostedAccountId ? t('server.people.porticoAccount') : '', m.role === 'owner' ? t('server.people.allLibraries') : m.allowedLibraries.length ? m.allowedLibraries.map(libName).join(', ') : t('server.people.allLibraries'), limits?.maxStreams ? t('server.people.streams', {count: limits.maxStreams}) : '', limits?.maxContentRating ? t('server.people.upTo', {rating: limits.maxContentRating}) : ''];
          return <SettingsRow key={m.id} icon="profile" label={m.username} help={facts.filter(Boolean).join(' · ')} meta={m.disabled ? <Badge tone="danger">{t('web.people.disabledBadge')}</Badge> : <Badge tone={m.role === 'owner' ? 'accent' : 'neutral'}>{tierLabels[m.role]}</Badge>} onClick={() => open(m.id)} />;
        })}
        <ListPager pages={access} />
      </SettingsGroup>
      <LocalMemberDialog open={adding} member={null} libraries={libraries.items} onClose={() => setAdding(false)} busy={action.busy} error={action.error} onSave={input => void action.run(() => api.request('/v1/direct/members', 'POST', input)).then(ok => { if (ok) { setAdding(false); read.reload(); access.reload(); } })} />
    </>
  );
}

/** One account: who it is, what it may do, and the devices signed in with it. Limits save with the page. */
function AccountDetail({member, access, libraries, onBack, onChanged}: {member: DirectMember; access?: AccessMember; libraries: readonly {id: string; name: string}[]; onBack: () => void; onChanged: () => void}) {
  const {api} = useSession();
  const t = currentI18n().t;
  const action = useAction();
  const [opIds] = useState(createOperationIds);
  const [editing, setEditing] = useState(false);
  const owner = member.role === 'owner';
  const doc = useRead(async () => parseLimitsDocument(await api.request(`/v1/admin/access/members/${encodeURIComponent(member.id)}/limits`), member.id), [api, member.id], {auto: !owner});
  const [limits, setLimits] = useState<MemberLimits>();
  useEffect(() => { if (doc.data) setLimits(doc.data.limits); }, [doc.data]);
  const inline = useInlineForm('limits:' + member.id, {
    dirty: !!doc.data && !!limits && JSON.stringify(limits) !== JSON.stringify(doc.data.limits),
    discard: () => setLimits(doc.data?.limits),
    save: async () => {
      if (!doc.data || !limits) return;
      const key = JSON.stringify({id: member.id, revision: doc.data.revision, limits});
      await api.request(`/v1/admin/access/members/${encodeURIComponent(member.id)}/limits`, 'PUT', {expectedRevision: doc.data.revision, operationId: opIds.forPayload(key), limits});
      opIds.release(); doc.reload(); onChanged();
    },
  });
  const setRole = (role: AccessTier) => access && void action.run(async () => { const key = JSON.stringify({id: member.id, revision: access.revision, role}); await api.request(`/v1/admin/access/members/${encodeURIComponent(member.id)}/role`, 'PUT', {expectedRevision: access.revision, operationId: opIds.forPayload(key), role}); opIds.release(); onChanged(); });
  const libName = (id: string) => libraries.find(l => l.id === id)?.name ?? t('server.people.libraryGone');
  return (
    <>
      <SectionHeader title={member.username} lede={member.hostedAccountId ? t('server.people.porticoAccount') : t('server.people.serverAccount')} actions={<Button size="sm" variant="ghost" icon="back" label={t('settings.server.accounts')} onClick={onBack} />} />
      {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
      {inline.error ? <Notice tone="error" compact>{problem(inline.error, 'action')}</Notice> : null}
      <SettingsGroup>
        <SettingsRow label={t('web.access.roleLabel')} control={owner || !access ? <Badge tone="accent">{tierLabels[member.role]}</Badge> : <Select hideLabel label={t('web.access.roleFor', {name: member.username})} value={access.role} disabled={action.busy} onChange={e => setRole(e.target.value as AccessTier)} options={[{value: 'member', label: t('web.access.roleMember')}, {value: 'admin', label: t('web.access.roleAdmin')}]} />} />
        <SettingsRow label={t('web.people.visibleLibraries')} meta={owner || !member.allowedLibraries.length ? t('server.people.allLibraries') : member.allowedLibraries.map(libName).join(', ')} control={!owner ? <Button size="sm" variant="ghost" label={t('web.selection.edit')} onClick={() => { action.clear(); setEditing(true); }} /> : undefined} />
        {!owner ? <SettingsRow label={t('server.people.signIn')} help={member.hostedAccountId ? t('web.people.hostedNote') : t('server.people.signInHelp')} meta={member.disabled ? <Badge tone="danger">{t('web.people.disabledBadge')}</Badge> : undefined} control={<Button size="sm" variant="ghost" label={t('web.selection.edit')} onClick={() => { action.clear(); setEditing(true); }} />} /> : null}
      </SettingsGroup>
      {doc.error ? <Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: doc.reload}}>{doc.error}</Notice> : null}
      {limits && !owner ? <LimitsFields limits={limits} onChange={setLimits} /> : null}
      <AccountDevices accountId={member.id} />
      <LocalMemberDialog open={editing} member={member} libraries={libraries} onClose={() => setEditing(false)} busy={action.busy} error={action.error} onSave={input => void action.run(() => api.request(`/v1/direct/members/${encodeURIComponent(member.id)}`, 'PATCH', {...input, expectedRevision: member.revision})).then(ok => { if (ok) { setEditing(false); onChanged(); } })} />
    </>
  );
}

function LibraryPicker({all, chosen, libraries, onChange}: {all: boolean; chosen: string[]; libraries: readonly {id: string; name: string}[]; onChange: (all: boolean, chosen: string[]) => void}) {
  const t = currentI18n().t;
  return (
    <div style={{display: 'flex', flexDirection: 'column', gap: 8}}>
      <Checkbox checked={all} onCheckedChange={v => onChange(v, v ? [] : chosen)} label={t('web.people.allLibrariesLater')} />
      {!all ? libraries.map(l => <Checkbox key={l.id} checked={chosen.includes(l.id)} onCheckedChange={v => onChange(false, v ? [...chosen, l.id] : chosen.filter(x => x !== l.id))} label={l.name} />) : null}
    </div>
  );
}

function LocalMemberDialog({open, member, libraries, onClose, onSave, busy, error}: {open: boolean; member: DirectMember | null; libraries: readonly {id: string; name: string}[]; onClose: () => void; onSave: (input: {username: string; password: string; name: string; allowedLibraries: string[]; disabled: boolean}) => void; busy: boolean; error: string}) {
  const t = currentI18n().t;
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
  }, [open, member]);
  return (
    <Dialog open={open} onOpenChange={o => !o && onClose()} title={member ? `Edit ${member.username}` : t('web.people.addAccountTitle')} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} /><Button variant="primary" label={member ? t('action.save') : t('web.people.addAccount')} loading={busy} disabled={!username.trim() || (!member && (!passwordStrength(password).acceptable || !name.trim())) || (!!member && !!password && !passwordStrength(password).acceptable) || (!all && !chosen.length)} onClick={() => onSave({username: username.trim(), password, name: name.trim() || username.trim(), allowedLibraries: all ? [] : chosen, disabled})} /></>}>
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
