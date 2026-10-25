export type ReaderEnvironment = 'web' | 'native';
export type ReaderPanelKind = 'display' | 'contents' | 'search';

/** Native tablet-read frames 4/6: touch chrome at every tablet width,
 * a display popover when expanded, and reflowing docked content panels.
 * Width changes presentation only; the engine retains the reading location. */
export function readerBookPresentation(width: number, environment: ReaderEnvironment = 'web') {
  const compact = width < 600, expanded = width >= 1024;
  const nativeTablet = environment === 'native' && !compact;
  const presentation = compact ? 'phone' : nativeTablet || !expanded ? 'tablet' : 'desktop';
  const displayPopover = nativeTablet && expanded;
  return {
    presentation,
    displayPopover,
    showPageArrows: expanded && !nativeTablet,
    fullscreen: presentation === 'desktop',
    panelReflows: (panel: ReaderPanelKind | undefined) => !!panel && !compact && !(displayPopover && panel === 'display'),
  } as const;
}
