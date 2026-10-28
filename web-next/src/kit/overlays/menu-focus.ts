export function menuInitialFocus(root: HTMLElement): HTMLElement {
  return root.querySelector<HTMLElement>('[data-menu-item]:not(:disabled)') ?? root;
}

/** A child picker can replace the currently focused row. Recover only that lost
 * focus; an outside click or another overlay keeps its own focus. */
export function recoverMenuChildFocus(root: HTMLElement, previous: HTMLElement | null) {
  if (!previous || root.contains(previous) || root.ownerDocument.activeElement !== root.ownerDocument.body) return;
  const target = root.querySelector<HTMLElement>('h1,h2,h3,[data-menu-heading]') ?? menuInitialFocus(root);
  if (!target.hasAttribute('tabindex')) target.tabIndex = -1;
  target.focus({preventScroll: true});
}
