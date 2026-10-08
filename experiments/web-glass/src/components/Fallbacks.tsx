import { ShelfSkeleton } from './media';
import { Skel } from './ui';

export function PageFallback({ bare }: { bare?: boolean }) {
  if (bare)
    return (
      <div style={{ minHeight: '100dvh', display: 'grid', placeItems: 'center' }}>
        <span className="spinner" />
      </div>
    );
  return (
    <div className="page" aria-busy="true">
      <div className="pad" style={{ display: 'grid', gap: 12, paddingTop: 12 }}>
        <Skel w={220} h={34} r={10} />
        <Skel w={140} h={14} />
      </div>
      <ShelfSkeleton />
      <ShelfSkeleton style="landscape" />
    </div>
  );
}
