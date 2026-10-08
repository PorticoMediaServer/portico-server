import React from 'react';
import {icons, iconIds, type IconId, type IconDefinition} from '@design';

/**
 * Portico's icon set, drawn from the one shared registry
 * (`packages/design/src/icons.ts`; X-17, CON-15/16): 24×24 grid, round caps
 * and joins, each glyph chosen by meaning. Web and Apple render the same data,
 * and `IconName` is the registry's closed union, so an unknown name fails `tsc`.
 */
export type IconName = IconId;

function element(e: IconDefinition['elements'][number], key: number): React.ReactNode {
  const fill = 'fill' in e && e.fill === 'currentColor' ? 'currentColor' : undefined;
  const stroke = 'stroke' in e && e.stroke === 'none' ? 'none' : undefined;
  switch (e.type) {
    case 'path': return <path key={key} d={e.d} fill={fill} stroke={stroke} />;
    case 'circle': return <circle key={key} cx={e.cx} cy={e.cy} r={e.r} fill={fill} stroke={stroke} />;
    case 'line': return <line key={key} x1={e.x1} y1={e.y1} x2={e.x2} y2={e.y2} />;
    case 'rect': return <rect key={key} x={e.x} y={e.y} width={e.width} height={e.height} rx={e.rx} fill={fill} stroke={stroke} />;
    case 'polygon': return <polygon key={key} points={e.points} fill={fill} stroke={stroke} />;
    case 'ellipse': return <ellipse key={key} cx={e.cx} cy={e.cy} rx={e.rx} ry={e.ry} />;
  }
}

/** Memoised: inline SVG on every card and row; a parent re-render must not re-reconcile it. */
export const Icon = React.memo(function Icon({name, size = 20, strokeWidth, className, title}: {name: IconName; size?: number; strokeWidth?: number; className?: string; title?: string}) {
  const def = icons[name];
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth ?? def.strokeWidth} strokeLinecap="round" strokeLinejoin="round" className={className} aria-hidden={title ? undefined : true} role={title ? 'img' : undefined} focusable="false">
      {title ? <title>{title}</title> : null}
      {def.elements.map(element)}
    </svg>
  );
});

export const iconNames: readonly IconName[] = iconIds;
