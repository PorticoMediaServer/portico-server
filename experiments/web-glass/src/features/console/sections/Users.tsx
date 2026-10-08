import { useState } from 'react';
import { Mail, Plus, ShieldCheck, UserX } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import type { UserAdmin } from '../../../services/api';
import { useLibraries, useUsers } from '../../../services/queries';
import { api } from '../../../services/client';
import { Avatar, Button, ErrorState, Segmented, Skel, Switch, cls } from '../../../components/ui';
import { Sheet, toast } from '../../../components/overlay';
import { ago } from '../../../lib/hooks';
import { Panel, SettingRow, Table } from '../ui';

export function UsersSection() {
  const q = useUsers();
  const libs = useLibraries();
  const qc = useQueryClient();
  const [filter, setFilter] = useState<'all' | 'household' | 'friends'>('all');
  const [open, setOpen] = useState<UserAdmin | null>(null);
  const [invite, setInvite] = useState(false);
  const [email, setEmail] = useState('');
  const [picked, setPicked] = useState<string[]>(['Movies', 'TV Shows']);
  if (q.isError) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  const rows = (q.data ?? []).filter((u) => filter === 'all' || (filter === 'household' ? u.household : !u.household));
  const save = async (patch: Partial<UserAdmin>) => {
    if (!open) return;
    const u = await api.updateUser(open.id, patch);
    setOpen({ ...u });
    qc.invalidateQueries({ queryKey: ['admin', 'users'] });
    toast(`Updated ${u.name.split(' ')[0]}`);
  };
  const libNames = (libs.data ?? []).map((l) => l.name);
  return (
    <div className="cstack">
      <div className="ctoolbar">
        <Segmented size="sm" value={filter} onChange={setFilter} options={[{ value: 'all', label: `All ${q.data?.length ?? ''}` }, { value: 'household', label: 'Household' }, { value: 'friends', label: 'Friends' }]} />
        <span style={{ flex: 1 }} />
        <Button size="sm" variant="accent" icon={<Plus />} onClick={() => setInvite(true)}>
          Invite Someone
        </Button>
      </div>
      <Panel pad={false}>
        {!q.data ? (
          <Skel h={320} />
        ) : (
          <Table
            rows={rows}
            getKey={(u) => u.id}
            onOpen={setOpen}
            cols={[
              { key: 'name', label: 'Name', sort: (u) => u.name, width: 'minmax(200px,1.4fr)', render: (u) => (
                <span className="user-cell">
                  <Avatar name={u.name} color={u.avatarColor} size={30} />
                  <span>
                    <b>{u.name}</b>
                    <span className="t-sub ellipsis">{u.email}</span>
                  </span>
                </span>
              ) },
              { key: 'role', label: 'Role', sort: (u) => u.role, width: '90px', render: (u) => <span className={cls('ctag', u.role === 'Owner' && 'info')}>{u.role}</span> },
              { key: 'status', label: 'Status', sort: (u) => u.status, width: '100px', render: (u) => <span className={cls('ctag', u.status === 'active' ? 'ok' : u.status === 'invited' ? 'info' : 'err')}>{u.status}</span> },
              { key: 'libs', label: 'Access', width: 'minmax(140px,1fr)', render: (u) => <span className="ellipsis t-sub">{u.libraries === 'all' ? 'All libraries' : u.libraries.join(', ')}</span> },
              { key: 'limit', label: 'Remote limit', align: 'right', sort: (u) => u.remoteLimitMbps ?? 999, width: '110px', render: (u) => <span className="tnum">{u.remoteLimitMbps ? `${u.remoteLimitMbps} Mbps` : '—'}</span> },
              { key: 'mfa', label: '2FA', width: '80px', render: (u) => (u.mfa === 'none' ? <span className="t-sub">Off</span> : <span className="c-ok"><ShieldCheck className="inline-ic" />{u.mfa === 'passkey' ? 'Passkey' : 'TOTP'}</span>) },
              { key: 'plays', label: 'Plays 30d', align: 'right', sort: (u) => u.plays30d, width: '90px', render: (u) => <span className="tnum">{u.plays30d}</span> },
              { key: 'seen', label: 'Last seen', sort: (u) => (u.lastSeen === '—' ? '' : u.lastSeen), width: '110px', render: (u) => <span className="tnum">{u.lastSeen === '—' ? '—' : ago(u.lastSeen)}</span> },
            ]}
          />
        )}
      </Panel>
      {open && (
        <Sheet
          open
          onClose={() => setOpen(null)}
          title={open.name}
          size="lg"
          footer={
            open.role !== 'Owner' ? (
              <>
                <Button size="sm" variant="danger" icon={<UserX />} onClick={() => save({ status: open.status === 'suspended' ? 'active' : 'suspended' })}>
                  {open.status === 'suspended' ? 'Restore Access' : 'Suspend Access'}
                </Button>
                <span style={{ flex: 1 }} />
                {open.status === 'invited' && (
                  <Button size="sm" variant="plain" icon={<Mail />} onClick={() => toast(`Invitation re-sent to ${open.email}`, <Mail />)}>
                    Resend Invite
                  </Button>
                )}
              </>
            ) : undefined
          }
        >
          <div className="cstack">
            <div className="user-head">
              <Avatar name={open.name} color={open.avatarColor} size={56} />
              <div>
                <div className="t-headline">{open.email}</div>
                <div className="t-sub tnum">
                  {open.role} · joined {ago(open.joinedAt)} · {open.devices} devices · {open.hours30d} h watched in 30 days · last IP {open.lastIp}
                </div>
              </div>
            </div>
            <Panel title="Library access">
              {libNames.map((n) => {
                const on = open.libraries === 'all' || open.libraries.includes(n);
                return (
                  <SettingRow key={n} title={n}>
                    <Switch size="sm" accent checked={on} disabled={open.role === 'Owner'} onChange={(v) => save({ libraries: v ? [...(open.libraries === 'all' ? libNames : open.libraries), n] : (open.libraries === 'all' ? libNames : open.libraries).filter((x) => x !== n) })} label={n} />
                  </SettingRow>
                );
              })}
            </Panel>
            <Panel title="Permissions & limits">
              <SettingRow title="Downloads" sub="Save titles for offline use">
                <Switch size="sm" accent checked={open.downloads} onChange={(v) => save({ downloads: v })} label="Downloads" />
              </SettingRow>
              <SettingRow title="Live TV">
                <Switch size="sm" accent checked={open.liveTv} onChange={(v) => save({ liveTv: v })} label="Live TV" />
              </SettingRow>
              <SettingRow title="Schedule recordings">
                <Switch size="sm" accent checked={open.dvr} onChange={(v) => save({ dvr: v })} label="DVR" />
              </SettingRow>
              <SettingRow title="Remote bandwidth" sub="Streams are converted to fit when they’d exceed this">
                <select className="input sm" value={open.remoteLimitMbps ?? 0} onChange={(e) => save({ remoteLimitMbps: Number(e.target.value) || undefined })}>
                  <option value={0}>No limit</option>
                  {[2, 4, 8, 12, 20, 40].map((n) => (
                    <option key={n} value={n}>
                      {n} Mbps
                    </option>
                  ))}
                </select>
              </SettingRow>
              <SettingRow title="Content rating limit">
                <span className="t-sub">{open.maxRating}</span>
              </SettingRow>
            </Panel>
          </div>
        </Sheet>
      )}
      <Sheet
        open={invite}
        onClose={() => setInvite(false)}
        title="Invite to Harbor"
        size="sm"
        footer={
          <Button
            variant="accent"
            icon={<Mail />}
            disabled={!email.includes('@')}
            onClick={async () => {
              await api.inviteUser(email, picked);
              qc.invalidateQueries({ queryKey: ['admin', 'users'] });
              toast(`Invitation sent to ${email}`, <Mail />);
              setInvite(false);
              setEmail('');
            }}
          >
            Send Invitation
          </Button>
        }
      >
        <div className="form-stack">
          <div className="field">
            <label htmlFor="inv-email">Email</label>
            <input id="inv-email" className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="friend@example.com" autoFocus />
          </div>
          <div className="field">
            <span className="field-label">Libraries to share</span>
            <div className="chip-wrap">
              {libNames.map((n) => (
                <button key={n} className={cls('chip', picked.includes(n) && 'on')} onClick={() => setPicked((p) => (p.includes(n) ? p.filter((x) => x !== n) : [...p, n]))}>
                  {n}
                </button>
              ))}
            </div>
          </div>
          <p className="t-caption">They’ll get a link to create a Portico login for Harbor. No cloud account needed. You can change access at any time.</p>
        </div>
      </Sheet>
    </div>
  );
}
