/**
 * The ambient glow's tint. Hovering or focusing artwork (or opening a title) sets it; the
 * atmosphere transitions to it slowly. Portico blue is the resting colour.
 */
export const PORTICO_BLUE = '#409fd4';
let current = PORTICO_BLUE;
let timer: ReturnType<typeof setTimeout> | undefined;

export function setTint(color: string | undefined | null, delay = 120) {
  const next = color || PORTICO_BLUE;
  if (timer) clearTimeout(timer);
  timer = setTimeout(() => {
    if (next === current) return;
    current = next;
    document.documentElement.style.setProperty('--tint', next);
  }, delay);
}

export const resetTint = () => setTint(PORTICO_BLUE, 0);
