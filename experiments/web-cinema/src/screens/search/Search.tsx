import React, {useEffect, useRef, useState} from 'react';
import {Link, useSearch} from '@tanstack/react-router';
import {useQuery, useServices} from '../../data/hooks';
import type {Channel, Item, Person, SearchGroup} from '../../data/types';
import {player} from '../../app/player';
import {setTint} from '../../app/tint';
import {Page} from '../../ui/Layout';
import {Chip, SearchInput} from '../../ui/Controls';
import {Art} from '../../ui/Media';
import {ErrorState, Skeleton, StateView} from '../../ui/Feedback';
import {Shelf} from '../shared/Shelf';
import {CardSkeleton, MediaCard} from '../shared/Cards';
import {plural} from '../shared/items';

const KINDS = [
  {id: 'all', label: 'All'}, {id: 'movies', label: 'Movies'}, {id: 'shows', label: 'Shows'}, {id: 'episodes', label: 'Episodes'}, {id: 'people', label: 'People'},
  {id: 'artists', label: 'Artists'}, {id: 'albums', label: 'Albums'}, {id: 'songs', label: 'Songs'}, {id: 'books', label: 'Books'}, {id: 'channels', label: 'Channels'},
];

/**
 * Search everything on the server. Results group by kind with a See all per group; recent searches
 * wait when the field is empty. Press / anywhere to come here. (The text stays out of the URL on
 * purpose; `?q=` only seeds it for links.)
 */
export function SearchScreen() {
  const services = useServices();
  const params = useSearch({from: '/app/search'});
  const [text, setText] = useState(params.q ?? '');
  const [kind, setKind] = useState(params.kind ?? 'all');
  const [q, setQ] = useState(text.trim());
  useEffect(() => { const t = setTimeout(() => setQ(text.trim()), 220); return () => clearTimeout(t); }, [text]);
  const words = q.split(/\s+/).filter(Boolean);
  const valid = q.length >= 2 && words.length <= 8;
  const results = useQuery(valid ? `search.${kind}.${q}` : null, () => services.catalog.search(q, kind));
  const recent = useQuery('search.recent', () => services.catalog.recentSearches());
  const remember = () => { if (valid) void services.catalog.addRecentSearch(q); };
  const inputWrap = useRef<HTMLDivElement>(null);
  return (
    <Page title="Search" className="search-page">
      <div className="search-hero" ref={inputWrap}>
        <h1 className="sr-only">Search</h1>
        <SearchInput size="lg" label="Search your library" placeholder="Films, shows, people, music, books, channels" value={text} onChange={setText} autoFocus onKeyDown={e => e.key === 'Enter' && remember()} />
        <div className="chip-row search-kinds" role="group" aria-label="Search in">
          {KINDS.map(k => <Chip key={k.id} on={kind === k.id} onClick={() => setKind(k.id)}>{k.label}</Chip>)}
        </div>
      </div>
      {!q ? (
        <div className="search-idle">
          {recent.data?.length ? (
            <section className="section">
              <div className="section-head"><h2 className="section-title">Recent searches</h2><button type="button" className="link-btn" onClick={() => void services.catalog.clearRecentSearches()}>Clear</button></div>
              <div className="chip-row">{recent.data.map(r => <Chip key={r} icon="history" onClick={() => setText(r)}>{r}</Chip>)}</div>
            </section>
          ) : null}
          <Suggestions onPick={setText} />
          <p className="faint small search-tip"><span className="kbd">/</span> opens Search from anywhere.</p>
        </div>
      ) : q.length < 2 ? (
        <StateView compact icon="search" title="Keep typing" body="Type at least 2 letters to search." />
      ) : words.length > 8 ? (
        <StateView compact icon="search" title="That’s a lot of words" body="Try 8 words or fewer; a title or a name works best." />
      ) : results.error ? (
        <ErrorState error={results.error} onRetry={results.reload} what="search results" />
      ) : !results.data ? (
        <SearchSkeleton />
      ) : !results.data.length ? (
        <StateView icon="search" title={`Nothing matches “${q}”`} body={kind === 'all' ? 'Check the spelling, or try a shorter part of the name.' : `Nothing in ${KINDS.find(k => k.id === kind)?.label}. Try All.`} action={kind !== 'all' ? {label: 'Search everything', onClick: () => setKind('all')} : undefined} />
      ) : (
        <div className="search-results" onClick={remember}>
          <p className="faint small" aria-live="polite">{plural(results.data.reduce((a, g) => a + g.total, 0), 'result')} for “{q}”</p>
          {results.data.map(g => <ResultGroup key={g.kind} group={g} full={kind !== 'all'} onSeeAll={() => setKind(g.kind)} />)}
        </div>
      )}
    </Page>
  );
}

function ResultGroup({group, full, onSeeAll}: {group: SearchGroup; full: boolean; onSeeAll: () => void}) {
  const services = useServices();
  const people = group.kind === 'people';
  const channels = group.kind === 'channels';
  const data = useQuery(`search.group.${group.kind}.${group.ids.join(',')}`, async () => {
    if (people) return {people: (await Promise.all(group.ids.map(id => services.catalog.person(id)))).filter(Boolean) as Person[]};
    if (channels) return {channels: (await services.live.channels()).filter(c => group.ids.includes(c.id))};
    return {items: await services.catalog.items(group.ids)};
  });
  const shows = useQuery(group.kind === 'episodes' && data.data?.items ? `search.shows.${group.ids.join()}` : null, () => services.catalog.items([...new Set((data.data!.items ?? []).map(i => (i.kind === 'episode' ? i.showId : '')).filter(Boolean))]));
  const showTitle = (i: Item) => (i.kind === 'episode' ? shows.data?.find(s => s.id === i.showId)?.title : undefined);
  const cards = people
    ? (data.data?.people ?? []).map(p => <PersonCard key={p.id} person={p} />)
    : channels
      ? (data.data?.channels ?? []).map(c => <ChannelCard key={c.id} channel={c} />)
      : (data.data?.items ?? []).map(i => <MediaCard key={i.id} item={i} fill={full} showTitle={showTitle(i)} />);
  if (full) {
    return (
      <section className="section">
        <h2 className="section-title" style={{marginBottom: 16}}>{group.label} <span className="faint">{group.total}</span></h2>
        <div className={`card-grid${people ? ' is-circle' : channels ? ' is-landscape' : group.kind === 'albums' || group.kind === 'songs' ? ' is-square' : ''}`}>{cards}</div>
      </section>
    );
  }
  return (
    <div className="search-shelf">
      <Shelf id={`r-${group.kind}`} title={group.label} seeAll={group.total > 6 ? {onClick: onSeeAll, label: `See all ${group.total}`} : undefined}>
        {cards}
      </Shelf>
    </div>
  );
}

function PersonCard({person}: {person: Person}) {
  return (
    <div className="card-cell cell-person">
      <Link to="/person/$personId" params={{personId: person.id}} className="card card-person" onMouseEnter={() => setTint(person.tint)} onFocus={() => setTint(person.tint)}>
        <span className="card-frame"><Art src={person.art.headshot} tint={person.tint} fallbackIcon="profile" /></span>
        <span className="card-caption"><span className="card-title">{person.name}</span><span className="card-sub">{person.knownFor}</span></span>
      </Link>
    </div>
  );
}

function ChannelCard({channel}: {channel: Channel}) {
  return (
    <div className="card-cell cell-landscape">
      <button type="button" className="card card-landscape" onClick={() => player.playLive(channel.id)} onMouseEnter={() => setTint(channel.tint)}>
        <span className="card-frame channel-frame"><Art src={channel.logo} tint={channel.tint} /></span>
        <span className="card-caption"><span className="card-title">{channel.number} · {channel.name}</span><span className="card-sub">Watch live</span></span>
      </button>
    </div>
  );
}

function Suggestions({onPick}: {onPick: (q: string) => void}) {
  const services = useServices();
  const cats = useQuery('categories.lib-movies', () => services.catalog.categories('lib-movies'));
  const genres = (cats.data ?? []).filter(c => c.kind === 'genre').slice(0, 8);
  return (
    <section className="section">
      <h2 className="section-title" style={{marginBottom: 14}}>Browse by genre</h2>
      <div className="tile-grid" style={{paddingTop: 0}}>
        {genres.length ? genres.map(g => (
          <Link key={g.id} to="/library/$libraryId/$view/$entityId" params={{libraryId: 'lib-movies', view: 'genre', entityId: g.id}} className="cat-tile" style={{'--c': g.tint} as React.CSSProperties}>
            <Art src={g.art} tint={g.tint} /><span className="cat-scrim" /><span className="cat-text"><strong>{g.title}</strong><span>{g.count} films</span></span>
          </Link>
        )) : Array.from({length: 8}, (_, i) => <Skeleton key={i} h={120} r={16} />)}
      </div>
      <div className="chip-row" style={{marginTop: 18}}>
        {['Cary Grant', 'Sintel', 'noir', 'jazz', 'Austen'].map(s => <Chip key={s} icon="search" onClick={() => onPick(s)}>{s}</Chip>)}
      </div>
    </section>
  );
}

function SearchSkeleton() {
  return (
    <div className="search-results" aria-busy="true">
      {[0, 1].map(i => <div key={i} className="shelf"><div className="shelf-head"><Skeleton w={160} h={20} /></div><div className="shelf-scroller" style={{overflow: 'hidden'}}>{Array.from({length: 8}, (_, j) => <CardSkeleton key={j} />)}</div></div>)}
    </div>
  );
}
