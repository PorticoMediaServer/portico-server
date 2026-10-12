/**
 * The key a player shortcut answers to, or undefined when the keystroke is not the player's:
 * with Ctrl, Cmd or Alt held it is the browser's or the system's (address bar, find, switch
 * tab), and during text composition it is part of a character. Letters are matched in lower
 * case, so Caps Lock does not turn the shortcuts off.
 */
export function shortcutKey(e: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'isComposing'> & {getModifierState?: (key: string) => boolean}): string | undefined {
  if (e.isComposing) return undefined;
  // AltGr types ordinary characters on many layouts, and Windows reports it as Ctrl+Alt.
  const typing = e.getModifierState?.('AltGraph') === true;
  if (e.metaKey || (!typing && (e.ctrlKey || e.altKey))) return undefined;
  return e.key.length === 1 ? e.key.toLowerCase() : e.key;
}
