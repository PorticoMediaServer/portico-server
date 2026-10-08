import { Link, useParams } from 'react-router';
import { BookHeadphones, Check, Download, Pause, Play } from 'lucide-react';
import { useAudiobook, useAudiobooks, useSetWatched } from '../../services/queries';
import { usePlayer } from '../../state/player';
import { PageHeader } from '../../components/PageHeader';
import { FloatBar, Section } from '../../components/Detail';
import { MediaCard } from '../../components/media';
import { Button, EmptyState, ErrorState, IconButton, Progress, Skel, cls } from '../../components/ui';
import { toast } from '../../components/overlay';
import { fmtClock, fmtDuration } from '../../lib/hooks';

export function AudiobooksPage() {
  const q = useAudiobooks();
  const listening = (q.data ?? []).filter((b) => b.progress);
  return (
    <div className="page">
      <PageHeader title="Audiobooks" subtitle={q.data ? `${q.data.length} books · ${fmtDuration(q.data.reduce((a, b) => a + b.duration, 0))} of listening` : undefined} />
      <div className="page-body pad">
        {q.isError && <ErrorState error={q.error} retry={() => q.refetch()} />}
        {listening.length > 0 && (
          <section className="book-now">
            <h2 className="shelf-title">Continue Listening</h2>
            <div className="book-now-row">
              {listening.map((b) => (
                <Link key={b.id} to={`/audiobook/${b.id}`} className="book-now-card" style={{ ['--tint' as string]: b.palette.base }} viewTransition>
                  <img src={b.art.square} alt="" />
                  <span className="book-now-main">
                    <span className="t-eyebrow">{b.author}</span>
                    <span className="book-now-title">{b.title}</span>
                    <Progress value={b.progress!} accent />
                    <span className="t-sub">{fmtDuration(b.duration * (1 - b.progress!))} left · narrated by {b.narrator}</span>
                  </span>
                </Link>
              ))}
            </div>
          </section>
        )}
        <h2 className="shelf-title" style={{ margin: '34px 0 14px' }}>
          All Audiobooks
        </h2>
        <div className="square-grid">
          {q.data
            ? q.data.map((b) => <MediaCard key={b.id} shape="square" item={{ id: b.id, kind: 'audiobook', title: b.title, subtitle: b.author, image: b.art.square!, href: `/audiobook/${b.id}`, progress: b.progress, palette: b.palette }} />)
            : Array.from({ length: 10 }, (_, i) => <Skel key={i} style={{ aspectRatio: '1' }} />)}
        </div>
        {q.data?.length === 0 && <EmptyState icon={<BookHeadphones />} title="No audiobooks yet" body="Add a folder of M4B or MP3 books to an Audiobooks library." />}
      </div>
    </div>
  );
}

export function AudiobookPage() {
  const { id = '' } = useParams();
  const q = useAudiobook(id);
  const setWatched = useSetWatched();
  const current = usePlayer((s) => s.book?.id === id);
  const playing = usePlayer((s) => s.playing);
  const idx = usePlayer((s) => s.index);
  if (q.isError) return <div className="page"><ErrorState error={q.error} retry={() => q.refetch()} /></div>;
  const b = q.data?.book;
  const pos = q.data?.state.progress?.position ?? 0;
  const p = usePlayer.getState();
  const start = (at = pos) => {
    if (!b) return;
    if (current) p.toggle();
    else p.playBook(b, at);
  };
  return (
    <div className="page album-page" style={{ ['--tint' as string]: b?.palette.base, ['--glow' as string]: b?.palette.accent }}>
      <FloatBar />
      <div className="album-glow" aria-hidden="true" />
      <header className="album-head pad">
        {b ? <img className="album-art book-art" src={b.art.square} alt="" /> : <Skel className="album-art" />}
        <div className="album-info">
          <span className="t-eyebrow">{b ? `Audiobook · ${b.genres.join(' · ')}` : ' '}</span>
          <h1 className="album-title">{b?.title ?? <Skel w={300} h={40} />}</h1>
          {b && <div className="album-artist">{b.author}</div>}
          {b && (
            <div className="album-meta">
              Narrated by {b.narrator} · {fmtDuration(b.duration)} · {b.chapters.length} chapters · First published {b.year < 1000 ? `c. ${b.year} AD` : b.year}
              {b.series ? ` · ${b.series.name} #${b.series.index}` : ''}
            </div>
          )}
          {b && pos > 0 && (
            <div className="book-progress">
              <Progress value={pos / b.duration} accent />
              <span className="t-sub tnum">{Math.round((pos / b.duration) * 100)}% · {fmtDuration(b.duration - pos)} left</span>
            </div>
          )}
          <div className="album-actions">
            <Button variant="primary" icon={current && playing ? <Pause fill="currentColor" /> : <Play fill="currentColor" />} disabled={!b} onClick={() => start()}>
              {current && playing ? 'Pause' : pos > 0 ? 'Resume' : 'Start Listening'}
            </Button>
            <IconButton label="Download" onClick={() => toast(`Downloading ${b?.title} for offline listening`, <Download />)}>
              <Download />
            </IconButton>
            <IconButton label="Mark as finished" onClick={() => b && setWatched.mutate({ id: b.id, watched: true }, { onSuccess: () => toast('Marked as finished', <Check />) })}>
              <Check />
            </IconButton>
          </div>
          {b && <p className="album-review">{b.summary}</p>}
        </div>
      </header>
      {b && (
        <Section title="Chapters" action={<span className="t-sub">{b.chapters.length} chapters</span>}>
          <div className="chapters-list">
            {b.chapters.map((c, i) => {
              const done = pos >= c.start + c.duration;
              const now = current ? idx === i : pos >= c.start && pos < c.start + c.duration;
              return (
                <button key={i} className={cls('chap-row', done && 'done', now && 'now')} onClick={() => (current ? p.jump(i) : p.playBook(b, c.start))}>
                  <span className="chap-num tnum">{done ? <Check /> : i + 1}</span>
                  <span className="chap-title ellipsis">{c.title}</span>
                  <span className="chap-dur tnum">{fmtClock(c.duration)}</span>
                </button>
              );
            })}
          </div>
        </Section>
      )}
      {q.data && q.data.more.length > 0 && (
        <Section title="You Might Also Like">
          <div className="square-grid">
            {q.data.more.map((x) => (
              <MediaCard key={x.id} shape="square" item={{ id: x.id, kind: 'audiobook', title: x.title, subtitle: x.author, image: x.art.square!, href: `/audiobook/${x.id}` }} />
            ))}
          </div>
        </Section>
      )}
      {b && <p className="source-credit pad">{b.source.credit}</p>}
    </div>
  );
}
