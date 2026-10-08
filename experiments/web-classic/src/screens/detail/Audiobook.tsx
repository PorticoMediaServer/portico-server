import {Link} from '@tanstack/react-router';
import {useQuery} from '@/data/DataProvider';
import {usePlayer} from '@/player/PlayerProvider';
import {formatDuration, formatTime} from '@/app/format';
import {IconButton} from '@/ui/Button';
import {ErrorState} from '@/ui/Bits';
import {Section} from '@/ui/Page';
import {Shelf} from '@/ui/Shelf';
import {Icon} from '@/ui/Icon';
import {useTitle} from '@/ui/hooks';
import {cx} from '@/ui/cx';
import {DetailLoading} from '../common';
import {ActionRow, DetailFrame, Facts, MetaLine, Overview} from './parts';
import s from './Detail.module.css';

/** Audiobook: resume at the current chapter or start over, and the chapter list. Speed and sleep timer live in the player. */
export function AudiobookDetail({id}: {id: string}) {
  const q = useQuery(c => c.catalog.audiobook(id), [id], ['personal']);
  const player = usePlayer();
  useTitle(q.data?.entry.title);
  if (q.error && !q.data) return <ErrorState onRetry={q.refresh} />;
  if (!q.data) return <DetailLoading />;
  const d = q.data;
  const total = d.entry.duration ?? 0;
  const pos = d.personal.progress;
  const current = d.chapters.findIndex((c, i) => pos >= c.start && (i === d.chapters.length - 1 || pos < d.chapters[i + 1]!.start));
  const playAt = (seconds: number) => player.play([d.entry], 0, {mode: 'book', position: seconds});
  return (
    <DetailFrame entry={d.entry}
      eyebrow={<><Link to="/library/lib-audiobooks">Audiobooks</Link>{d.series ? <><span aria-hidden>·</span><Link to={`/library/lib-audiobooks/series/${encodeURIComponent(d.series.name)}` as never}>{d.series.name}, book {d.series.index}</Link></> : null}</>}
      sections={<>
        <Section title="Chapters">
          <div role="list">
            {d.chapters.map((c, i) => (
              <div key={c.number} role="listitem" className={cx(s.chapter, i < current && s.chapterDone, i === current && s.chapterCurrent)}>
                <IconButton size="sm" variant="ghost" icon={i === current ? 'play' : 'playOutline'} label={`Play chapter ${c.number}, ${c.title}`} onClick={() => playAt(c.start)} />
                <span style={{minWidth: 0}}><span className="t-strong clamp-1" style={{display: 'block'}}>{c.title}</span><span className="t-caption c-3">{[c.title === `Chapter ${c.number}` ? null : `Chapter ${c.number}`, i === current ? 'You’re here' : null].filter(Boolean).join(' · ')}</span></span>
                <span className="t-caption c-3 tnum">Starts {formatTime(c.start)}</span>
                <span className="t-caption c-2 tnum" style={{textAlign: 'right'}}>{formatTime(c.duration)}</span>
              </div>
            ))}
          </div>
        </Section>
        {d.more.length ? <Shelf title={`More by ${d.author}`} entries={d.more} /> : null}
      </>}>
      <MetaLine items={[d.author, d.entry.year, formatDuration(total), `${d.chapters.length} chapters`]} />
      <ActionRow entry={d.entry} personal={d.personal} watchlist={false} watchedToggle={false} playLabel={pos > 0 ? `Resume chapter ${current + 1}` : 'Listen'} onPlay={fromStart => playAt(fromStart ? 0 : pos)} />
      <Overview text={d.entry.overview} />
      <Facts rows={[['Written by', <Link to={`/library/lib-audiobooks/author/${encodeURIComponent(d.author)}` as never}>{d.author}</Link>], ['Narrated by', d.narrator], ['Genre', d.genre], ['Publisher', d.publisher]]} />
      <p className="t-caption c-3" style={{display: 'flex', gap: 6, alignItems: 'center'}}><Icon name="speed" size={15} />Playback speed and the sleep timer are in the player.</p>
    </DetailFrame>
  );
}
