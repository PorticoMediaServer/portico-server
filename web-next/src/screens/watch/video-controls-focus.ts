/** Reveal before the browser tries to Tab into inert player controls. Focus is
 * requested after React commits visibility; an ordinary visible Tab stays native. */
export function revealVideoControlsOnTab(event: Pick<KeyboardEvent, 'key' | 'shiftKey' | 'defaultPrevented' | 'preventDefault'> & Partial<Pick<KeyboardEvent, 'ctrlKey' | 'metaKey' | 'altKey'>>,
  visible: boolean, revealAndFocus: (backwards: boolean) => void): boolean {
  if (event.defaultPrevented || event.ctrlKey || event.metaKey || event.altKey || event.key !== 'Tab' || visible) return false;
  event.preventDefault();
  revealAndFocus(event.shiftKey);
  return true;
}

/** Both idle expiry and picture taps retain any focused chrome control. */
export function videoControlsMayHide(chrome: Pick<HTMLElement, 'contains'> | null, active: Node | null): boolean {
  return !chrome?.contains(active);
}
