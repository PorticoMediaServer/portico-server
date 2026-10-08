// Edge refraction for floating glass chrome.
//
// Chromium can run an SVG filter as a backdrop-filter, so we can bend the content
// behind a pill-shaped bar the way a thick lens bends light at its rim. We render a
// displacement map for the element's exact size (a signed-distance "bezel" around a
// rounded rect), wrap it in an feDisplacementMap filter, and point the element's
// --refract-filter at it. Other engines keep the plain blur. It is used on at most
// three small surfaces (tab bar, top bar, mini player) so the cost stays tiny.
import { useEffect } from 'react';
import { useSession } from '../state/session';

let supported: boolean | null = null;
export function refractionSupported(): boolean {
  if (supported !== null) return supported;
  const ua = navigator.userAgent;
  const chromium = /Chrome\/\d+/.test(ua);
  const safari = /Safari\//.test(ua) && !/Chrome\//.test(ua);
  const firefox = /Firefox\//.test(ua);
  supported = chromium && !safari && !firefox;
  return supported;
}

function host(): SVGSVGElement {
  let svg = document.getElementById('refraction-defs') as SVGSVGElement | null;
  if (!svg) {
    svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.id = 'refraction-defs';
    svg.setAttribute('width', '0');
    svg.setAttribute('height', '0');
    svg.setAttribute('aria-hidden', 'true');
    svg.style.position = 'absolute';
    svg.style.pointerEvents = 'none';
    svg.appendChild(document.createElementNS('http://www.w3.org/2000/svg', 'defs'));
    document.body.appendChild(svg);
  }
  return svg;
}

function displacementMap(w: number, h: number, radius: number, bezel: number): string {
  const canvas = document.createElement('canvas');
  canvas.width = w;
  canvas.height = h;
  const ctx = canvas.getContext('2d')!;
  const img = ctx.createImageData(w, h);
  const r = Math.min(radius, w / 2, h / 2);
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      // signed distance to a rounded rect (negative inside)
      const px = x + 0.5 - w / 2;
      const py = y + 0.5 - h / 2;
      const qx = Math.abs(px) - (w / 2 - r);
      const qy = Math.abs(py) - (h / 2 - r);
      const ox = Math.max(qx, 0);
      const oy = Math.max(qy, 0);
      const outside = Math.hypot(ox, oy);
      const inside = Math.min(Math.max(qx, qy), 0);
      const sd = outside + inside - r; // < 0 inside
      const d = -sd; // distance from rim, inward
      let dx = 0;
      let dy = 0;
      if (d > 0 && d < bezel) {
        // gradient of the SDF approximates the outward normal
        let nx: number;
        let ny: number;
        if (qx > 0 && qy > 0) {
          const len = Math.hypot(ox, oy) || 1;
          nx = (ox / len) * Math.sign(px);
          ny = (oy / len) * Math.sign(py);
        } else if (qx > qy) {
          nx = Math.sign(px);
          ny = 0;
        } else {
          nx = 0;
          ny = Math.sign(py);
        }
        const t = 1 - d / bezel;
        const mag = t * t * (3 - 2 * t); // smoothstep: strongest at the rim
        dx = -nx * mag; // sample inward: the rim magnifies what is just inside it
        dy = -ny * mag;
      }
      const i = (y * w + x) * 4;
      img.data[i] = Math.round(128 + dx * 127);
      img.data[i + 1] = Math.round(128 + dy * 127);
      img.data[i + 2] = 128;
      img.data[i + 3] = 255;
    }
  }
  ctx.putImageData(img, 0, 0);
  return canvas.toDataURL('image/png');
}

let counter = 0;

export function useRefraction(ref: React.RefObject<HTMLElement | null>, opts: { radius?: number; bezel?: number; scale?: number } = {}) {
  const transparency = useSession((s) => s.transparency);
  useEffect(() => {
    const el = ref.current;
    if (!el || !refractionSupported() || transparency === 'reduced') return;
    if (window.matchMedia('(prefers-reduced-transparency: reduce)').matches && transparency !== 'full') return;
    document.documentElement.classList.add('refraction');
    const id = `refract-${++counter}`;
    const defs = host().querySelector('defs')!;
    const filter = document.createElementNS('http://www.w3.org/2000/svg', 'filter');
    filter.id = id;
    filter.setAttribute('color-interpolation-filters', 'sRGB');
    filter.setAttribute('x', '0');
    filter.setAttribute('y', '0');
    filter.setAttribute('width', '100%');
    filter.setAttribute('height', '100%');
    const image = document.createElementNS('http://www.w3.org/2000/svg', 'feImage');
    image.setAttribute('result', 'map');
    image.setAttribute('preserveAspectRatio', 'none');
    const disp = document.createElementNS('http://www.w3.org/2000/svg', 'feDisplacementMap');
    disp.setAttribute('in', 'SourceGraphic');
    disp.setAttribute('in2', 'map');
    disp.setAttribute('xChannelSelector', 'R');
    disp.setAttribute('yChannelSelector', 'G');
    disp.setAttribute('scale', String(opts.scale ?? 60));
    filter.append(image, disp);
    defs.appendChild(filter);

    let last = '';
    const build = () => {
      const w = Math.round(el.offsetWidth);
      const h = Math.round(el.offsetHeight);
      if (!w || !h) return;
      const key = `${w}x${h}`;
      if (key === last) return;
      last = key;
      const radius = opts.radius ?? h / 2;
      image.setAttribute('href', displacementMap(w, h, radius, opts.bezel ?? Math.min(26, h * 0.46)));
      image.setAttribute('width', String(w));
      image.setAttribute('height', String(h));
      el.style.setProperty('--refract-filter', `url(#${id})`);
    };
    build();
    let t: ReturnType<typeof setTimeout>;
    const ro = new ResizeObserver(() => {
      clearTimeout(t);
      t = setTimeout(build, 120);
    });
    ro.observe(el);
    return () => {
      ro.disconnect();
      clearTimeout(t);
      filter.remove();
      el.style.removeProperty('--refract-filter');
    };
  }, [ref, transparency, opts.radius, opts.bezel, opts.scale]);
}

/** Pointer-following sheen for interactive glass (cheap: two CSS variables, rAF-throttled). */
export function useSheen(ref: React.RefObject<HTMLElement | null>) {
  useEffect(() => {
    const el = ref.current;
    if (!el || window.matchMedia('(hover: none)').matches) return;
    let raf = 0;
    const onMove = (e: PointerEvent) => {
      cancelAnimationFrame(raf);
      raf = requestAnimationFrame(() => {
        const r = el.getBoundingClientRect();
        el.style.setProperty('--mx', `${e.clientX - r.left}px`);
        el.style.setProperty('--my', `${e.clientY - r.top}px`);
      });
    };
    const onLeave = () => {
      el.style.removeProperty('--mx');
      el.style.removeProperty('--my');
    };
    el.addEventListener('pointermove', onMove);
    el.addEventListener('pointerleave', onLeave);
    return () => {
      cancelAnimationFrame(raf);
      el.removeEventListener('pointermove', onMove);
      el.removeEventListener('pointerleave', onLeave);
    };
  }, [ref]);
}
