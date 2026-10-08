import {useState} from 'react';
import {useParams} from '@tanstack/react-router';
import {useQuery, useServices} from '../../data/hooks';
import type {Item} from '../../data/types';
import {Page, Tabs, useTitle} from '../../ui/Layout';
import {Art} from '../../ui/Media';
import {ErrorState, StateView} from '../../ui/Feedback';
import {Facts, ReadMore, Stage, TitleSkeleton} from '../detail/parts';
import {MediaCard} from '../shared/Cards';
import {Shelf} from '../shared/Shelf';

/** A person: portrait, life, what they're known for, and every credit on this server. */
export function PersonScreen() {
  const {personId} = useParams({from: '/app/person/$personId'});
  const services = useServices();
  const person = useQuery(`person.${personId}`, () => services.catalog.person(personId));
  const credits = useQuery(`credits.${personId}`, () => services.catalog.personCredits(personId));
  const ids = [...new Set([...(credits.data?.cast ?? []), ...(credits.data?.crew ?? [])].map(c => c.itemId))];
  const items = useQuery(credits.data ? `credits.items.${personId}` : null, () => services.catalog.items(ids));
  const [tab, setTab] = useState<'all' | 'cast' | 'crew'>('all');
  useTitle(person.data?.name);
  if (person.error) return <Page><ErrorState error={person.error} onRetry={person.reload} /></Page>;
  const p = person.data;
  if (!p) return person.loading ? <Page flush><TitleSkeleton /></Page> : <Page><StateView icon="profile" title="Portico doesn’t know this person" action={{label: 'Go home', to: '/', icon: 'home'}} /></Page>;
  const byId = new Map((items.data ?? []).map(i => [i.id, i]));
  const cast = (credits.data?.cast ?? []).map(c => ({item: byId.get(c.itemId), role: c.character})).filter((x): x is {item: Item; role: string | undefined} => !!x.item);
  const crew = (credits.data?.crew ?? []).map(c => ({item: byId.get(c.itemId), role: c.job})).filter((x): x is {item: Item; role: string | undefined} => !!x.item);
  const shown = tab === 'cast' ? cast : tab === 'crew' ? crew : [...cast, ...crew].filter((x, i, a) => a.findIndex(y => y.item.id === x.item.id) === i);
  const knownFor = [...shown].sort((a, b) => (b.item.art.backdrop ? 1 : 0) - (a.item.art.backdrop ? 1 : 0)).slice(0, 8);
  const year = (d?: string | null) => (d ? new Date(d).getFullYear() : null);
  const life = p.born ? (p.died ? `${fmt(p.born)} – ${fmt(p.died)} (aged ${year(p.died)! - year(p.born)!})` : `Born ${fmt(p.born)}`) : null;
  const lead = knownFor.find(k => k.item.art.backdrop)?.item;
  return (
    <Page flush className="title-page person-page">
      <Stage art={lead?.art.backdrop} tint={p.tint} className="stage-short stage-person">
        <div className="person-head">
          <Art src={p.art.headshot} tint={p.tint} className="person-photo" ratio="1/1" eager fallbackIcon="profile" />
          <div className="music-info">
            <div className="eyebrow stage-eyebrow">{p.knownFor}</div>
            <h1 className="display stage-title is-music">{p.name}</h1>
            <Facts items={[life, p.birthplace, `${cast.length + crew.length} ${cast.length + crew.length === 1 ? 'credit' : 'credits'} on this server`]} />
            {p.bio ? <ReadMore text={p.bio} lines={3} className="stage-syn" /> : <p className="muted">Portico doesn’t have a biography for {p.name} yet.</p>}
          </div>
        </div>
      </Stage>
      {knownFor.length > 3 ? <Shelf id="known" title="Known for">{knownFor.map(k => <MediaCard key={k.item.id} item={k.item} subtitle={k.role ? `as ${k.role}` : undefined} />)}</Shelf> : null}
      <div className="page-pad" style={{marginTop: 36}}>
        <h2 className="shelf-title" style={{marginBottom: 12}}>Credits</h2>
        <Tabs label="Credits" value={tab} onChange={setTab} tabs={[{value: 'all', label: 'All', count: cast.length + crew.length}, {value: 'cast', label: 'Acting', count: cast.length, hidden: !cast.length}, {value: 'crew', label: 'Crew', count: crew.length, hidden: !crew.length}]} />
        {!items.data ? null : shown.length ? (
          <div className="card-grid">{shown.map(k => <MediaCard key={k.item.id + (k.role ?? '')} item={k.item} fill subtitle={[k.item.year, k.role ? (tab === 'crew' || crew.some(c => c.item.id === k.item.id && c.role === k.role) ? k.role : `as ${k.role}`) : null].filter(Boolean).join(' · ')} />)}</div>
        ) : <StateView compact icon="film" title={`No ${tab === 'crew' ? 'crew' : tab === 'cast' ? 'acting' : ''} credits on this server`} body={`${p.name}’s other work isn’t in your libraries.`} />}
      </div>
    </Page>
  );
}

const fmt = (d: string) => new Date(d).toLocaleDateString('en-US', {month: 'long', day: 'numeric', year: 'numeric'});
