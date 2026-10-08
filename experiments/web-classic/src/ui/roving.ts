import React, {useCallback, useEffect} from 'react';

/**
 * Roving focus for rows and grids of cards: the group is one Tab stop and the
 * arrow keys move within it (Up and Down move by a row in grids), Home and End
 * jump to the ends. Items are elements marked `data-rove`.
 */
export function useRoving(ref: React.RefObject<HTMLElement | null>, mode: 'row' | 'grid', deps: readonly unknown[] = []) {
  const items = useCallback(() => (ref.current ? [...ref.current.querySelectorAll<HTMLElement>('[data-rove]')] : []), [ref]);
  // Keep exactly one tab stop, also as cards arrive (paged grids fill in after mount).
  useEffect(() => {
    const root = ref.current;
    if (!root) return;
    const ensure = () => {
      const list = items();
      if (list.length && !list.some(el => el.tabIndex === 0)) list[0]!.tabIndex = 0;
    };
    ensure();
    const mo = new MutationObserver(ensure);
    mo.observe(root, {childList: true, subtree: true});
    return () => mo.disconnect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
  const onFocus = useCallback((e: React.FocusEvent) => {
    const target = (e.target as HTMLElement).closest<HTMLElement>('[data-rove]');
    if (!target) return;
    for (const el of items()) el.tabIndex = el === target ? 0 : -1;
  }, [items]);
  const onKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.altKey || e.ctrlKey || e.metaKey) return;
    const current = (e.target as HTMLElement).closest<HTMLElement>('[data-rove]');
    if (!current || current !== e.target) return;
    const list = items();
    const i = list.indexOf(current);
    if (i < 0) return;
    let next = -1;
    if (e.key === 'ArrowRight') next = i + 1;
    else if (e.key === 'ArrowLeft') next = i - 1;
    else if (e.key === 'Home') next = 0;
    else if (e.key === 'End') next = list.length - 1;
    else if (mode === 'grid' && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
      const top = current.getBoundingClientRect().top;
      const cols = list.filter(el => Math.abs(el.getBoundingClientRect().top - top) < 4).length || 1;
      next = e.key === 'ArrowDown' ? i + cols : i - cols;
      if (next >= list.length && e.key === 'ArrowDown') next = list.length - 1;
    } else return;
    if (next < 0 || next >= list.length) { e.preventDefault(); return; }
    e.preventDefault();
    const el = list[next]!;
    el.focus({preventScroll: false});
    el.scrollIntoView({block: 'nearest', inline: 'nearest', behavior: 'smooth'});
  }, [items, mode]);
  return {onFocus, onKeyDown};
}
