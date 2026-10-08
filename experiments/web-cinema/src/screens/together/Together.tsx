import {useState} from 'react';
import {useNavigate, useSearch} from '@tanstack/react-router';
import {useDataVersion, useQuery, useServices} from '../../data/hooks';
import type {Item, TogetherGroup} from '../../data/types';
import {together, useTogether} from '../../app/together';
import {player} from '../../app/player';
import {useSession} from '../../app/session';
import {Page, PageHeader, Panel} from '../../ui/Layout';
import {Button, IconButton} from '../../ui/Button';
import {ChoiceCards, Field, Input} from '../../ui/Controls';
import {Badge, Notice, Skeleton, StateView, toast} from '../../ui/Feedback';
import {Icon} from '../../ui/Icon';
import {Art, Avatar} from '../../ui/Media';
import {ConfirmDialog} from '../../ui/Overlay';
import {cx} from '../../ui/cx';
import {subtitleFor} from '../shared/items';

/** Watch Together: start a group or join one with a code; inside, the room with its queue and people. */
export function TogetherScreen() {
  const search = useSearch({from: '/app/together'});
  const {groupId} = useTogether();
  const id = search.group ?? groupId;
  return id ? <Room id={id} /> : <Lobby />;
}

function Lobby() {
  const services = useServices();
  const session = useSession();
  const navigate = useNavigate();
  const groups = useQuery('together.groups', () => services.together.groups());
  const [name, setName] = useState(`${session.profile?.name ?? 'My'}’s movie night`);
  const [control, setControl] = useState<'host' | 'everyone'>('host');
  const [code, setCode] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState<'start' | 'join' | null>(null);
  const enter = (g: TogetherGroup) => { together.join(g.id); void navigate({to: '/together', search: {group: g.id}}); };
  return (
    <Page title="Watch Together" className="together-page">
      <PageHeader eyebrow="Watch Together" title="Watch in step, wherever everyone is" lede="Start a group, share the code, and everyone’s player stays in sync. Pause for one, pause for all." />
      <div className="together-grid">
        <Panel title="Start a group" subtitle="You’ll be the host.">
          <form className="stack" onSubmit={e => { e.preventDefault(); }}>
            <Field label="Group name"><Input value={name} onChange={e => setName(e.target.value)} maxLength={60} /></Field>
            <div className="field"><span className="field-label">Who controls playback</span>
              <ChoiceCards label="Who controls playback" columns={2} value={control} onChange={setControl} options={[{value: 'host', title: 'Only me', body: 'Others can suggest titles.', icon: 'shield'}, {value: 'everyone', title: 'Everyone', body: 'Anyone can play or pause.', icon: 'people'}]} />
            </div>
            <Button variant="primary" icon="people" label="Start group" busy={busy === 'start'} disabled={!name.trim()} onClick={async () => { setBusy('start'); const g = await services.together.create(name.trim(), control); setBusy(null); toast(`Started “${g.name}”. Share code ${g.code}.`, {icon: 'people'}); enter(g); }} />
          </form>
        </Panel>
        <Panel title="Join with a code" subtitle="Ask the host for the 8-character code.">
          <form className="stack" onSubmit={async e => {
            e.preventDefault();
            setError('');
            setBusy('join');
            const g = await services.together.join(code);
            setBusy(null);
            if (!g) return setError('That code didn’t match a group. Check it with the host; codes stop working when the group ends.');
            enter(g);
          }}>
            <Field label="Group code" error={error || undefined}><Input value={code} onChange={e => { setCode(e.target.value.toUpperCase()); setError(''); }} placeholder="KXWP 7M2Q" maxLength={9} className="code-input" autoComplete="off" /></Field>
            <Button variant="secondary" label="Join group" busy={busy === 'join'} disabled={code.replace(/\s/g, '').length < 8} onClick={() => undefined} type="submit" />
          </form>
        </Panel>
      </div>
      <section className="section">
        <h2 className="section-title" style={{marginBottom: 12}}>Groups on this server</h2>
        {!groups.data ? <Skeleton h={80} /> : !groups.data.filter(g => g.state !== 'ended').length ? <StateView compact icon="people" title="No groups right now" body="Groups you start or are invited to appear here." /> : (
          <div className="stack" style={{gap: 8}}>
            {groups.data.filter(g => g.state !== 'ended').map(g => (
              <button key={g.id} type="button" className="group-row" onClick={() => enter(g)}>
                <span className="group-faces">{g.members.slice(0, 4).map(m => <Avatar key={m.id} name={m.name} color={m.color} size={32} />)}</span>
                <span className="row-text"><span className="row-label">{g.name}</span><span className="row-help">{g.members.length} people · hosted by {g.members.find(m => m.host)?.name} · {g.state === 'lobby' ? 'Waiting to start' : 'Watching now'}</span></span>
                <Badge tone={g.state === 'playing' ? 'live' : 'accent'}>{g.state === 'playing' ? 'Live' : 'Open'}</Badge>
                <Icon name="chevronRight" size={18} className="row-chevron" />
              </button>
            ))}
          </div>
        )}
      </section>
    </Page>
  );
}

function Room({id}: {id: string}) {
  const services = useServices();
  const version = useDataVersion();
  const navigate = useNavigate();
  const q = useQuery(`together.${id}.${version}`, () => services.together.group(id));
  const g = q.data;
  const items = useQuery(g ? `together.items.${g.queue.join(',')}` : null, () => services.catalog.items(g!.queue));
  const [leaving, setLeaving] = useState(false);
  const [ending, setEnding] = useState(false);
  if (!g) return q.loading ? <Page><Skeleton h={300} /></Page> : <Page title="Watch Together"><StateView icon="people" title="This group has ended" body="Start a new one, or join another with its code." action={{label: 'Back to Watch Together', onClick: () => { together.leave(); void navigate({to: '/together', search: {}}); }}} /></Page>;
  const me = g.members.find(m => m.you);
  const host = !!me?.host;
  const canControl = host || g.control === 'everyone';
  const byId = new Map((items.data ?? []).map(i => [i.id, i]));
  const leave = () => { together.leave(); void navigate({to: '/together', search: {}}); };
  const watch = (item: Item) => { together.join(g.id); void services.together.update(g.id, {state: 'playing', nowPlaying: item.id}); player.play(item.id, {kind: 'video'}); };
  if (g.state === 'ended') return <Page title={g.name}><StateView icon="people" title="The host ended the group" body="Thanks for watching together." action={{label: 'Back to Watch Together', onClick: leave}} /></Page>;
  return (
    <Page title={g.name} className="together-page">
      <PageHeader eyebrow={<><span className="live-dot" /> Watch Together · {g.control === 'host' ? 'Host controls playback' : 'Everyone can control'}</>} title={g.name} back={{label: 'Watch Together', to: '/together'}} actions={<>
        {host ? <Button variant="danger" label="End for everyone" onClick={() => setEnding(true)} /> : null}
        <Button variant="secondary" icon="signOut" label="Leave" onClick={() => (host ? setLeaving(true) : leave())} />
      </>} />
      {g.state === 'hostReconnecting' ? <Notice tone="warn" title="The host is reconnecting">Playback is paused until they’re back. If they don’t return in a few minutes the group ends.</Notice> : null}
      <div className="room-grid">
        <div className="room-main">
          <Panel title="Invite people" subtitle="Anyone with an account on this server can join with the code or the link.">
            <div className="invite-code">
              <span className="code-big tabular" aria-label={`Code ${g.code.split('').join(' ')}`}>{g.code.slice(0, 4)} {g.code.slice(4)}</span>
              <div className="hstack">
                <Button variant="secondary" icon="copy" label="Copy code" onClick={() => { void navigator.clipboard?.writeText(g.code).catch(() => undefined); toast('Code copied'); }} />
                <Button variant="secondary" icon="link" label="Copy link" onClick={() => { void navigator.clipboard?.writeText(`${location.origin}/together/join#code=${g.code}`).catch(() => undefined); toast('Invite link copied'); }} />
                {host ? <Button variant="ghost" icon="refresh" label="New code" onClick={() => void services.together.update(g.id, {code: Math.random().toString(36).slice(2, 10).toUpperCase()}).then(() => toast('New code made; the old one stops working'))} /> : null}
              </div>
            </div>
          </Panel>
          <Panel title="Queue" subtitle={canControl ? 'Choose what to watch; everyone’s player follows.' : 'The host picks what plays. You can suggest titles from any title’s More menu.'} padded={false}>
            {g.queue.length ? (
              <ol className="room-queue">
                {g.queue.map((qid, i) => {
                  const it = byId.get(qid);
                  return (
                    <li key={qid} className={cx('room-q', g.nowPlaying === qid && 'is-now')}>
                      <Art src={it?.art.backdrop ?? it?.art.poster} tint={it?.tint} ratio="16/9" className="room-q-art" />
                      <span className="row-text"><span className="row-label">{it?.title ?? '…'}</span><span className="row-help">{g.nowPlaying === qid ? 'Playing for everyone' : i === 0 ? 'Up first' : it ? subtitleFor(it) : ''}</span></span>
                      {canControl && it ? <Button size="sm" variant={g.nowPlaying === qid ? 'secondary' : 'primary'} icon="play" label={g.nowPlaying === qid ? 'Rejoin' : 'Watch this'} onClick={() => watch(it)} /> : null}
                      {canControl ? <IconButton size="sm" variant="ghost" icon="close" label={`Remove ${it?.title ?? 'title'}`} onClick={() => void services.together.update(g.id, {queue: g.queue.filter(x => x !== qid)})} /> : null}
                    </li>
                  );
                })}
              </ol>
            ) : <StateView compact icon="queue" title="Nothing queued" body="Use Add to group queue on any title." />}
          </Panel>
        </div>
        <Panel title={`People · ${g.members.length}`} className="room-people">
          <ul className="people-list">
            {g.members.map(m => (
              <li key={m.id}>
                <Avatar name={m.name} color={m.color} size={38} />
                <span className="row-text"><span className="row-label">{m.name}{m.you ? ' (you)' : ''}</span><span className="row-help">{m.host ? 'Host · ' : ''}{m.status === 'inStep' ? 'In step' : m.status === 'buffering' ? 'Loading…' : m.status === 'catchingUp' ? 'Catching up' : 'Away'}</span></span>
                <span className={cx('status-dot', `is-${m.status}`)} aria-hidden />
                {host && !m.you ? <Button size="sm" variant="ghost" label="Make host" onClick={() => void services.together.update(g.id, {members: g.members.map(x => ({...x, host: x.id === m.id, you: x.you}))}).then(() => toast(`${m.name} is the host now`))} /> : null}
              </li>
            ))}
          </ul>
          {canControl ? (
            <div className="hstack" style={{marginTop: 16}}>
              <Button variant="secondary" icon="pause" label="Pause for everyone" onClick={() => { player.pause(); toast('Paused for everyone'); }} />
              <Button variant="secondary" icon="play" label="Play for everyone" onClick={() => { player.resume(); toast('Playing for everyone'); }} />
            </div>
          ) : null}
        </Panel>
      </div>
      <ConfirmDialog open={leaving} onOpenChange={setLeaving} title="Leave as host?" body="The group keeps going and someone else becomes the host." confirmLabel="Leave" onConfirm={leave} />
      <ConfirmDialog open={ending} onOpenChange={setEnding} danger title="End the group for everyone?" body="Everyone’s playback stops and the code stops working." confirmLabel="End for everyone" onConfirm={() => services.together.end(g.id).then(() => { together.leave(); toast('Group ended'); })} />
    </Page>
  );
}
