import { useParams } from 'react-router';
import { usePerson } from '../../services/queries';
import { FloatBar, Section } from '../../components/Detail';
import { MediaCard } from '../../components/media';
import { ErrorState, Skel } from '../../components/ui';

const hue = (s: string) => [...s].reduce((a, c) => (a * 31 + c.charCodeAt(0)) % 360, 7);

export function PersonPage() {
  const { id = '' } = useParams();
  const q = usePerson(id);
  if (q.isError) return <div className="page"><ErrorState error={q.error} retry={() => q.refetch()} /></div>;
  const p = q.data?.person;
  const acting = q.data?.credits.filter((c) => !['Director', 'Writer', 'Creator'].includes(c.role)) ?? [];
  const crew = q.data?.credits.filter((c) => ['Director', 'Writer', 'Creator'].includes(c.role)) ?? [];
  return (
    <div className="page person-page">
      <FloatBar />
      <header className="person-head pad">
        {p ? (
          <span className="person-face big" style={{ ['--h' as string]: hue(p.name) }}>
            {p.name.split(' ').map((w) => w[0]).slice(0, 2).join('')}
          </span>
        ) : (
          <Skel w={160} h={160} r={80} />
        )}
        <div className="person-info">
          <div className="t-eyebrow">{acting.length && crew.length ? 'Actor & Filmmaker' : acting.length ? 'Actor' : 'Filmmaker'}</div>
          <h1 className="page-title">{p?.name ?? <Skel w={280} h={40} />}</h1>
          {p && (
            <div className="person-facts">
              {p.born && <span>Born {p.born}</span>}
              {p.birthplace && <span>{p.birthplace}</span>}
              <span>{q.data!.credits.length} titles on Harbor</span>
            </div>
          )}
          <p className="person-bio">{p?.bio}</p>
        </div>
      </header>
      {acting.length > 0 && (
        <Section title="Acting">
          <div className="credit-grid">
            {acting.map((c) => (
              <MediaCard key={c.id + c.role} shape="poster" item={{ ...c, subtitle: `as ${c.role}` }} />
            ))}
          </div>
        </Section>
      )}
      {crew.length > 0 && (
        <Section title="Directing & Writing">
          <div className="credit-grid">
            {crew.map((c) => (
              <MediaCard key={c.id + c.role} shape="poster" item={{ ...c, subtitle: c.role }} />
            ))}
          </div>
        </Section>
      )}
    </div>
  );
}
