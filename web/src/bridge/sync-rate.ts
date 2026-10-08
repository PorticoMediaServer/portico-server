/** A short, local speed nudge that keeps this player in step with a group. It multiplies
 * whatever speed the viewer chose and is never sent to the server or shown as their speed. */
let element: HTMLMediaElement | null = null, nudge = 1, base = 1;
export const syncRate = {
  attach(next: HTMLMediaElement | null) { if (element && nudge !== 1) element.playbackRate = base; element = next; nudge = 1; },
  set(next: number) {
    if (!element || next === nudge) return;
    if (nudge === 1) base = element.playbackRate;
    element.playbackRate = next === 1 ? base : base * next;
    nudge = next;
  },
};
