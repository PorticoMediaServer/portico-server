import React from 'react';
import {cx} from './cx';
import s from './Grid.module.css';

export type GridDensity = 'poster' | 'square' | 'landscape' | 'person' | 'compact' | 'large';
/** A width that follows the viewer's card size (Small · Medium · Large; `--card-scale` on the root). */
export const scaled = (width: string) => `calc(${width} * var(--card-scale, 1))`;
const mins: Record<GridDensity, string> = {poster: scaled('clamp(120px, 11vw, 168px)'), square: scaled('clamp(140px, 13vw, 190px)'), landscape: scaled('clamp(220px, 22vw, 300px)'), person: scaled('clamp(110px, 10vw, 140px)'), compact: scaled('clamp(96px, 8.5vw, 130px)'), large: scaled('clamp(150px, 14vw, 210px)')};

/** Responsive card grid. Column count follows available width; card geometry never changes. */
export function Grid({density = 'poster', children, className, style}: {density?: GridDensity; children: React.ReactNode; className?: string; style?: React.CSSProperties}) {
  return (
    <div className={cx(s.grid, className)} style={{'--card-min': mins[density], ...style} as React.CSSProperties}>
      {children}
    </div>
  );
}

export function List({children, className}: {children: React.ReactNode; className?: string}) {
  return <div className={cx(s.list, className)}>{children}</div>;
}

/** The minimum column width a density uses (for grids drawn elsewhere, e.g. `WindowedGrid`). */
export const gridColumnMin = (density: GridDensity) => mins[density];

const watchCardSize = (changed: () => void) => {
  const observer = new MutationObserver(changed);
  observer.observe(document.documentElement, {attributes: true, attributeFilter: ['data-poster-size']});
  return () => observer.disconnect();
};
/** The viewer's card size as the root carries it, for layouts that measure themselves (a windowed grid's row pitch). */
export const useCardSize = (): string => React.useSyncExternalStore(watchCardSize, () => document.documentElement.dataset.posterSize ?? 'regular', () => 'regular');
