import type {View} from 'react-native';

/**
 * The last focused element in the content area (TV). The rail hands focus back
 * to it when the viewer presses right, the way a real rail returns you to
 * where you were.
 */
export const lastContentFocus: {current: View | null} = {current: null};

/**
 * After the viewer opens a screen (TV), focus moves to its top-left control
 * instead of staying where tvOS last had it (often the rail). Controls that
 * mount shortly after the request put themselves forward. A control that asks
 * for preferred focus wins; otherwise the one nearest the top-left does, with
 * height weighing more than width so a page's first row beats a corner button.
 */
type Candidate = {view: () => View | null; take: () => void; preferred?: boolean};
let pendingAt = 0;
let candidates: Candidate[] = [];
let timer: ReturnType<typeof setTimeout> | undefined;

export function requestContentFocus() {
  pendingAt = Date.now();
  candidates = [];
  if (timer) clearTimeout(timer);
  timer = undefined;
}

export function offerContentFocus(c: Candidate) {
  if (!pendingAt || Date.now() - pendingAt > 1500) return;
  candidates.push(c);
  if (timer) return;
  timer = setTimeout(() => {
    const list = candidates;
    pendingAt = 0; candidates = []; timer = undefined;
    const measured: {c: Candidate; x: number; y: number}[] = [];
    let left = list.length;
    const done = () => {
      if (--left > 0) return;
      measured.sort((a, b) => Number(!!b.c.preferred) - Number(!!a.c.preferred) || (a.y + a.x / 2) - (b.y + b.x / 2));
      measured[0]?.c.take();
    };
    for (const c of list) {
      const v = c.view();
      if (!v) { done(); continue; }
      v.measureInWindow((x, y, w, h) => { if (w > 0 && h > 0 && y > -h) measured.push({c, x, y}); done(); });
    }
  }, 300);
}
