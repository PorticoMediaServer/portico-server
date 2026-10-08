import {useMemo, useState} from 'react';
import {Link} from '@tanstack/react-router';
import {useQuery} from '@/data/DataProvider';
import {hrefFor} from '@/app/links';
import {Artwork, ErrorState} from '@/ui/Bits';
import {Segmented, Select} from '@/ui/Form';
import {Section} from '@/ui/Page';
import {Shelf} from '@/ui/Shelf';
import {useTitle} from '@/ui/hooks';
import {DetailLoading, useRouteParams} from '../common';
import {DetailFrame, MetaLine, Overview} from './parts';
import s from './Detail.module.css';

/** 1895-10-04 → October 4, 1895 (dates without a time zone shift). */
const longDate = (iso: string) => { const [y, m, d] = iso.split('-').map(Number); return y && m && d ? new Date(y, m - 1, d).toLocaleDateString('en-US', {month: 'long', day: 'numeric', year: 'numeric'}) : iso; };

/** A person: photo, life dates, biography, known for, and every credit on this server. */
export default function PersonScreen() {
  const {personId} = useRouteParams<{personId: string}>();
  const q = useQuery(c => c.catalog.person(personId), [personId]);
  const [role, setRole] = useState<'all' | 'cast' | 'crew'>('all');
  const [sort, setSort] = useState<'new' | 'old' | 'title'>('new');
  useTitle(q.data?.name);
  const credits = useMemo(() => {
    const list = (q.data?.credits ?? []).filter(c => role === 'all' || c.role === role);
    return [...list].sort((a, b) => sort === 'title' ? a.entry.title.localeCompare(b.entry.title) : sort === 'old' ? (a.entry.year ?? 0) - (b.entry.year ?? 0) : (b.entry.year ?? 0) - (a.entry.year ?? 0));
  }, [q.data, role, sort]);
  if (q.error && !q.data) return <ErrorState onRetry={q.refresh} />;
  if (!q.data) return <DetailLoading />;
  const d = q.data;
  const roles = new Set(d.credits.map(c => (c.role === 'cast' ? 'Actor' : c.job ?? 'Crew')));
  return (
    <DetailFrame entry={{id: d.id, kind: 'person', title: d.name, image: d.image, shape: 'circle'}} shape="circle" backdrop={d.knownFor[0]?.backdrop} eyebrow={[...roles].slice(0, 3).join(' · ')}
      sections={<>
        {d.knownFor.length ? <Shelf title="Known for" entries={d.knownFor} /> : null}
        <Section title={`Credits on this server · ${d.credits.length}`} action={<div style={{display: 'flex', gap: 8, flexWrap: 'wrap'}}>
          <Segmented label="Credits" value={role} onChange={setRole} options={[{value: 'all', label: 'All'}, {value: 'cast', label: 'Acting'}, {value: 'crew', label: 'Crew'}]} />
          <Select compact ariaLabel="Sort credits" value={sort} onChange={setSort} options={[{value: 'new', label: 'Newest first'}, {value: 'old', label: 'Oldest first'}, {value: 'title', label: 'Title'}]} />
        </div>}>
          <div role="list">
            {credits.length ? credits.map((c, i) => (
              <div key={`${c.entry.id}-${i}`} role="listitem" className={s.creditRow}>
                <span className={s.creditYear}>{c.entry.year ?? '—'}</span>
                <Artwork src={c.entry.image} title={c.entry.title} shape="poster" tint={c.entry.tint} />
                <span className={s.creditTitle} style={{minWidth: 0}}><Link to={(hrefFor(c.entry) ?? '/') as never}>{c.entry.title}</Link><span className="t-caption c-3" style={{display: 'block'}}>{c.role === 'cast' ? c.character ? `as ${c.character}` : 'Actor' : c.job}</span></span>
                <span className="t-caption c-3">{c.entry.kind === 'show' ? 'Series' : 'Film'}</span>
              </div>
            )) : <p className="c-3" style={{padding: 12}}>No {role === 'cast' ? 'acting' : 'crew'} credits on this server.</p>}
          </div>
        </Section>
      </>}>
      <MetaLine items={[d.born ? `Born ${longDate(d.born)}` : null, d.died ? `Died ${longDate(d.died)}` : null, `${d.credits.length} titles on this server`]} />
      <Overview text={d.bio ?? `${d.name} appears in ${d.credits.length} titles in your libraries.`} />
    </DetailFrame>
  );
}
