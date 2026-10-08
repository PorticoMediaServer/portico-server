import React from 'react';

/**
 * One avatar primitive (CON-12): the profile picture, or the initial on the
 * profile's art color. Sizes 24/32/40/64.
 */
export function Avatar({src, name, art, size = 32}: {src?: string; name: string; art?: string; size?: 24 | 32 | 40 | 64}) {
  const radius = size / 2;
  if (src) {
    return <img src={src} alt="" width={size} height={size} style={{width: size, height: size, borderRadius: radius, objectFit: 'cover', display: 'block'}} />;
  }
  return (
    <span aria-hidden style={{display: 'grid', placeItems: 'center', width: size, height: size, borderRadius: radius, background: art ? `var(--profile-art-${art})` : 'var(--surface-raised)', border: '1px solid var(--line-soft)', fontWeight: 600, fontSize: size <= 24 ? 11 : size <= 32 ? 13 : 16}}>
      {name.slice(0, 1).toUpperCase()}
    </span>
  );
}
