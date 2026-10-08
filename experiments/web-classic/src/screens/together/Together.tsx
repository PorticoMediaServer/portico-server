import {useEffect, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useClient, useQuery} from '@/data/DataProvider';
import type {WatchGroup} from '@/data/types';
import {useSession} from '@/app/session';
import {Button, IconButton} from '@/ui/Button';
import {Artwork, Avatar, Badge, EmptyState, Skeleton, Surface} from '@/ui/Bits';
import {ConfirmDialog, Menu, useToast} from '@/ui/Overlay';
import {RadioCards, TextField} from '@/ui/Form';
import {Group, PageHeader, Row, Section} from '@/ui/Page';
import {CardGrid} from '@/ui/Shelf';
import {useTitle} from '@/ui/hooks';
import {useRouteSearch} from '../common';
import s from './Together.module.css';

/** Watch Together: start a group or join with a code, then watch in step with the host. */
export default function TogetherScreen() {
  const session = useSession();
  const search = useRouteSearch<{item?: string; code?: string}>();
  const groups = useQuery(c => c.together.groups(), [], ['together']);
  const current = groups.data?.find(g => g.members.some(m => m.you));
  useTitle('Watch Together');
  if (session.profile?.limits?.together === false) return <EmptyState icon="lock" title="Watch Together isn’t available on this profile" body="The main profile can turn it on in Settings › Account › Profiles." />;
  return (
    <div style={{padding: '28px 0 72px'}}>
      <PageHeader title="Watch Together" subtitle="Watch in step with people on this server, wherever they are. The host plays, pauses and seeks for everyone." />
      {!groups.data ? <div style={{padding: '0 var(--gutter)'}}><Skeleton height={220} /></div> : current ? <Room g={current} /> : <Start item={search.item} code={search.code} others={groups.data} onJoined={() => groups.refresh()} />}
    </div>
  );
}

function Start({item, code: initialCode, others, onJoined}: {item?: string; code?: string; others: WatchGroup[]; onJoined: () => void}) {
  const client = useClient();
  const session = useSession();
  const toast = useToast();
  const [name, setName] = useState(`${session.profile?.name ?? 'My'}’s group`);
  const [control, setControl] = useState<'host-only' | 'anyone'>('host-only');
  const [code, setCode] = useState(initialCode ?? '');
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState<string>();
  const entry = item ? client.catalog.entry(item) : undefined;
  return (
    <div className={s.start}>
      <Surface>
        <h2 className="t-sub">Start a group</h2>
        <p className="t-caption c-2" style={{margin: '4px 0 16px'}}>{entry ? <>You’ll start with <b>{entry.title}</b>. </> : null}Share the code or link with the people you want to watch with.</p>
        <div style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 14}}>
          <TextField label="Group name" value={name} onChange={setName} />
          <RadioCards label="Who controls playback" value={control} onChange={setControl} options={[{value: 'host-only', label: 'Only the host', help: 'Everyone else watches along.'}, {value: 'anyone', label: 'Everyone', help: 'Anyone can play, pause or skip.'}]} />
          <div><Button variant="primary" icon="together" label="Start group" busy={busy === 'start'} onClick={async () => { setBusy('start'); await client.together.create(name.trim() || 'Watch party', control); if (item) await client.together.watch(item); setBusy(undefined); onJoined(); toast.show('Group started. Share the code to invite people.', {icon: 'together'}); }} /></div>
        </div>
      </Surface>
      <Surface>
        <h2 className="t-sub">Join with a code</h2>
        <p className="t-caption c-2" style={{margin: '4px 0 16px'}}>Enter the eight-character code the host shared.</p>
        <form style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 12}} onSubmit={async e => { e.preventDefault(); setBusy('join'); setError(undefined); try { await client.together.join(code); onJoined(); } catch (err) { setError(err instanceof Error ? `${err.message} Check it with the host, or ask for a new one.` : 'That code didn’t work.'); } finally { setBusy(undefined); } }}>
          <TextField label="Code" placeholder="8 letters and numbers" value={code} onChange={v => { setCode(v.toUpperCase()); setError(undefined); }} error={error} autoComplete="off" spellCheck={false} style={code ? {fontSize: 20, letterSpacing: '0.18em', fontWeight: 700} : undefined} />
          <div><Button type="submit" variant="secondary" label="Join" busy={busy === 'join'} disabled={code.replace(/\s/g, '').length < 8} /></div>
        </form>
        {others.length ? (
          <>
            <div className="t-label" style={{margin: '22px 0 8px'}}>Groups on this server</div>
            <Group>
              {others.map(g => <Row key={g.id} iconNode={<span style={{display: 'flex'}}>{g.members.slice(0, 3).map((m, i) => <span key={m.id} style={{marginLeft: i ? -8 : 0}}><Avatar name={m.name} art={m.art} size={28} /></span>)}</span>} label={g.name} help={`Hosted by ${g.members.find(m => m.host)?.name} · ${g.members.length} watching${g.nowPlaying ? ` · ${g.nowPlaying.title}` : ''}`} trailing={<Button size="sm" variant="secondary" label="Join" onClick={async () => { await client.together.join(g.code); onJoined(); }} />} />)}
            </Group>
          </>
        ) : null}
      </Surface>
    </div>
  );
}

function Room({g}: {g: WatchGroup}) {
  const client = useClient();
  const toast = useToast();
  const navigate = useNavigate();
  const me = g.members.find(m => m.you)!;
  const [confirm, setConfirm] = useState<'end' | 'leave'>();
  const [left, setLeft] = useState(Math.max(0, Math.round((g.codeExpires - Date.now()) / 60000)));
  const suggestions = useQuery(c => c.home.row('picks'), []);
  useEffect(() => { const t = setInterval(() => setLeft(Math.max(0, Math.round((g.codeExpires - Date.now()) / 60000))), 30000); return () => clearInterval(t); }, [g.codeExpires]);
  const link = `${location.origin}/together/join#code=${g.code.replace(/\s/g, '')}`;
  return (
    <div style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 28}}>
      <div className={s.room}>
        <Surface className={s.now}>
          {g.nowPlaying ? (
            <>
              <Artwork src={g.nowPlaying.backdrop ?? g.nowPlaying.image} title={g.nowPlaying.title} shape="wide" />
              <div style={{display: 'flex', alignItems: 'center', gap: 12, marginTop: 14, flexWrap: 'wrap'}}>
                <div style={{flex: 1, minWidth: 200}}><div className="t-label">{g.state === 'playing' ? 'Playing for everyone' : 'Ready to start'}</div><div className="t-sub">{g.nowPlaying.title}</div></div>
                {me.host || g.control === 'anyone' ? <Button variant="primary" icon={g.state === 'playing' ? 'pause' : 'play'} label={g.state === 'playing' ? 'Pause for everyone' : 'Play for everyone'} onClick={() => void client.together.setPlaying(g.state !== 'playing')} /> : <Badge>The host controls playback</Badge>}
              </div>
            </>
          ) : <EmptyState compact icon="play" title="Pick something to watch" body={me.host ? 'Choose Watch with a group on any title, or pick one below.' : 'The host is choosing what to watch.'} />}
        </Surface>
        <Surface>
          <div style={{display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12}}><h2 className="t-sub" style={{flex: 1}}>{g.name}</h2><Badge tone="accent">{g.members.length} here</Badge></div>
          <div className={s.code} aria-label={`Invite code ${g.code}`}>{g.code}</div>
          <p className="t-caption c-3" style={{textAlign: 'center', margin: '6px 0 12px'}}>{left ? `Code works for ${left} more minutes` : 'This code expired'}</p>
          <div style={{display: 'flex', gap: 8, justifyContent: 'center', flexWrap: 'wrap'}}>
            <Button size="sm" variant="secondary" icon="link" label="Copy invite link" onClick={() => { void navigator.clipboard?.writeText(link).catch(() => {}); toast.show('Invite link copied', {icon: 'link'}); }} />
            {me.host ? <Button size="sm" variant="ghost" icon="refresh" label="New code" onClick={async () => { await client.together.newCode(); toast.show('New code ready. The old one no longer works.', {icon: 'refresh'}); }} /> : null}
          </div>
          <div className="t-label" style={{margin: '18px 0 6px'}}>People</div>
          {g.members.map(m => (
            <div key={m.id} className={s.member}>
              <Avatar name={m.name} art={m.art} size={32} />
              <span style={{flex: 1, minWidth: 0}}><span className="t-strong">{m.name}{m.you ? ' (you)' : ''}</span><span className="t-caption c-3" style={{display: 'block'}}>{m.host ? 'Host · ' : ''}{({inStep: 'In step', buffering: 'Loading…', catchingUp: 'Catching up', away: 'Away'} as const)[m.status]}</span></span>
              {me.host && !m.you ? <Menu label={`${m.name} options`} items={[{id: 'host', label: 'Make host', icon: 'crown', onSelect: async () => { await client.together.makeHost(m.id); toast.show(`${m.name} is the host now`, {icon: 'crown'}); }}]} trigger={<IconButton size="sm" variant="ghost" icon="more" label={`${m.name} options`} />} /> : null}
            </div>
          ))}
          <div style={{display: 'flex', gap: 8, marginTop: 16, flexWrap: 'wrap'}}>
            <Button variant="ghost" icon="signOut" label="Leave group" onClick={() => setConfirm('leave')} />
            {me.host ? <Button variant="danger" label="End for everyone" onClick={() => setConfirm('end')} /> : null}
          </div>
        </Surface>
      </div>
      {g.queue.length ? (
        <Section title="Up next for the group">
          <Group>{g.queue.map(e => <Row key={e.id} iconNode={<Artwork src={e.image} title={e.title} style={{width: 36}} />} label={e.title} help={e.subtitle} trailing={me.host ? <IconButton size="sm" variant="ghost" icon="close" label={`Remove ${e.title}`} onClick={() => void client.together.queueRemove(e.id)} /> : undefined} />)}</Group>
        </Section>
      ) : null}
      {me.host ? (
        <section>
          <h2 className="t-sub" style={{padding: '0 var(--gutter)', marginBottom: 12}}>Suggestions for the group</h2>
          <CardGrid entries={suggestions.data?.entries.slice(0, 12)} loading={suggestions.loading} onOpen={async e => { if (g.nowPlaying) { await client.together.queueAdd(e.id); toast.show(`Added “${e.title}” to the group’s queue`, {icon: 'queue'}); } else { await client.together.watch(e.id); toast.show(`Playing “${e.title}” for the group`, {icon: 'together'}); } }} />
        </section>
      ) : null}
      <ConfirmDialog open={confirm === 'end'} onOpenChange={o => !o && setConfirm(undefined)} title="End the group for everyone?" body="Everyone stops watching together. Each person keeps their place in the title." action="End for everyone" tone="danger" onConfirm={async () => { await client.together.end(); setConfirm(undefined); toast.show('Group ended', {icon: 'check'}); }} />
      <ConfirmDialog open={confirm === 'leave'} onOpenChange={o => !o && setConfirm(undefined)} title={me.host ? 'Leave as the host?' : 'Leave the group?'} body={me.host ? `${g.members.find(m => !m.you)?.name ?? 'Someone else'} becomes the host, and the group carries on without you.` : 'The group carries on without you. You can join again with the code.'} action="Leave" onConfirm={async () => { await client.together.leave(); setConfirm(undefined); void navigate({to: '/together' as never}); }} />
    </div>
  );
}
