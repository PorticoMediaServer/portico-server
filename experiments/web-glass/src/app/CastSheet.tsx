import { Cast, Check, Laptop, Speaker, Tv, Users } from 'lucide-react';
import { useCastTargets } from '../services/queries';
import { usePlayer } from '../state/player';
import { useUi } from '../state/ui';
import { Sheet, toast } from '../components/overlay';
import { Skel, cls } from '../components/ui';

export function CastSheet() {
  const open = useUi((s) => s.cast);
  const set = useUi((s) => s.setCast);
  const targets = useCastTargets(open);
  const castTo = usePlayer((s) => s.castTo);
  const setCast = usePlayer((s) => s.setCast);
  const pick = (id: string | null, name: string) => {
    setCast(id);
    set(false);
    toast(id ? `Now playing on ${name}` : 'Playing on this browser', <Cast />);
  };
  return (
    <Sheet open={open} onClose={() => set(false)} title="Play On" size="sm" compact>
      <div className="cast-list">
        <button className={cls('cast-row', !castTo && 'on')} onClick={() => pick(null, 'this browser')}>
          <span className="cast-icon">
            <Laptop />
          </span>
          <span className="cast-main">
            <span className="cast-name">This Browser</span>
            <span className="cast-sub">Safari on MacBook Pro</span>
          </span>
          {!castTo && <Check className="cast-check" />}
        </button>
        <div className="cast-label">
          {targets.isFetching ? (
            <>
              <span className="spinner" style={{ width: 12, height: 12 }} /> Looking for devices on your network…
            </>
          ) : (
            'Devices on your network'
          )}
        </div>
        {targets.isLoading &&
          Array.from({ length: 3 }, (_, i) => (
            <div key={i} className="cast-row">
              <Skel w={36} h={36} r={10} />
              <Skel w="50%" h={12} />
            </div>
          ))}
        {targets.data?.map((t) => (
          <button key={t.id} className={cls('cast-row', castTo === t.id && 'on')} onClick={() => pick(t.id, t.name)}>
            <span className="cast-icon">{t.kind === 'tv' ? <Tv /> : t.kind === 'group' ? <Users /> : <Speaker />}</span>
            <span className="cast-main">
              <span className="cast-name">{t.name}</span>
              <span className="cast-sub">
                {t.protocol} · {t.room}
              </span>
            </span>
            {castTo === t.id && <Check className="cast-check" />}
          </button>
        ))}
      </div>
    </Sheet>
  );
}
