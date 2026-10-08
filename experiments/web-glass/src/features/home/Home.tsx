import { useHome } from '../../services/queries';
import { Hero, HeroSkeleton } from '../../components/Hero';
import { Shelf, ShelfSkeleton } from '../../components/media';
import { ErrorState } from '../../components/ui';
import { PageHeader } from '../../components/PageHeader';
import { useCurrentProfile } from '../../app/ProfileMenu';
import { useIsMobile } from '../../lib/hooks';

const greeting = () => {
  const h = new Date().getHours();
  return h < 5 ? 'Up late' : h < 12 ? 'Good morning' : h < 18 ? 'Good afternoon' : 'Good evening';
};

export function Home() {
  const home = useHome();
  const me = useCurrentProfile();
  const mobile = useIsMobile();
  return (
    <div className="page flush home">
      {mobile && <PageHeader title="Home" subtitle={me ? `${greeting()}, ${me.name}` : undefined} />}
      {home.isError ? (
        <div style={{ paddingTop: 'calc(var(--topbar-h) + 40px)' }}>
          <ErrorState error={home.error} retry={() => home.refetch()} title="Couldn’t reach Harbor" />
        </div>
      ) : home.isLoading || !home.data ? (
        <>
          <HeroSkeleton />
          <ShelfSkeleton style="landscape" />
          <ShelfSkeleton />
        </>
      ) : (
        <>
          <Hero items={home.data.hero} />
          <div className="home-shelves">
            {home.data.shelves.map((s, i) => (
              <Shelf key={s.id} shelf={s} eager={i < 2} />
            ))}
          </div>
        </>
      )}
    </div>
  );
}
