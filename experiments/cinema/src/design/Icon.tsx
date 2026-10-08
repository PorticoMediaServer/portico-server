import React from 'react';
import Svg, {Circle, Ellipse, Line, Path, Polygon, Polyline, Rect} from 'react-native-svg';
import {iconData, type IconName} from './iconData';
import {colors, u} from './tokens';

export type {IconName};

const components = {path: Path, circle: Circle, rect: Rect, line: Line, polyline: Polyline, polygon: Polygon, ellipse: Ellipse} as const;

function camel(attrs: Record<string, string>) {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(attrs)) out[k.replace(/-([a-z])/g, (_, c) => c.toUpperCase())] = v;
  return out;
}

/** A stroke icon. `size` is in phone points and scales up on TV. `fill` fills closed shapes (play, star, heart). */
export const Icon = React.memo(function Icon({name, size = 20, color = colors.text, fill, strokeWidth = 2, raw}: {name: IconName; size?: number; color?: string; fill?: boolean | string; strokeWidth?: number; raw?: boolean}) {
  const px = raw ? size : u(size);
  const els = iconData[name] as readonly (readonly [keyof typeof components, Record<string, string>])[];
  const fillColor = fill === true ? color : typeof fill === 'string' ? fill : 'none';
  return (
    <Svg width={px} height={px} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
      {els.map(([tag, attrs], i) => {
        const C = components[tag] as React.ComponentType<Record<string, unknown>>;
        return <C key={i} {...camel(attrs)} fill={fillColor} />;
      })}
    </Svg>
  );
});
