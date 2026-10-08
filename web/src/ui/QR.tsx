import {useEffect, useMemo, useState} from 'react';
import {uiI18n} from './i18n';

type Encoder = typeof import('qrcode');
let encoder: Promise<Encoder> | undefined;
// The encoder is only needed on two pairing screens, so it stays out of the
// entry chunk and loads on first use.
const loadEncoder = () => (encoder ??= import('qrcode'));

/** QR image with the human-readable value always presented alongside by the caller. */
export function QR({value, size = 168}: {value: string; size?: number}) {
  const [QRCode, setQRCode] = useState<Encoder>();
  useEffect(() => { let active = true; void loadEncoder().then(m => { if (active) setQRCode(m); }); return () => { active = false; }; }, []);
  const cells = useMemo(() => {
    if (!QRCode) return null;
    try {
      const code = QRCode.create(value, {errorCorrectionLevel: 'M'});
      const n = code.modules.size;
      const rects: string[] = [];
      for (let y = 0; y < n; y++) for (let x = 0; x < n; x++) if (code.modules.get(y, x)) rects.push(`M${x} ${y}h1v1h-1z`);
      return {n, d: rects.join('')};
    } catch {
      return null;
    }
  }, [value, QRCode]);
  if (!cells) return null;
  return (
    <svg viewBox={`-2 -2 ${cells.n + 4} ${cells.n + 4}`} width={size} height={size} role="img" aria-label={uiI18n().t('qr.label')} style={{background: 'var(--color-text)', borderRadius: 'var(--radius-control)', padding: 0}} shapeRendering="crispEdges">
      <path d={cells.d} fill="var(--color-ink)" />
    </svg>
  );
}
