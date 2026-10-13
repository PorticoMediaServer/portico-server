/**
 * Fonts for scripts the default subtitle font (Latin, Greek, Cyrillic) does not have.
 *
 * A styled subtitle usually brings its own fonts. When a line uses letters none of them has,
 * libass looks through every font it was given, so the fonts for the scripts actually present in
 * the subtitle are added to the list. A script that is not in the subtitle costs nothing.
 */
import {fallbackFontUrl} from './vendor.ts';

type Script = Readonly<{name: string; file: string; ranges: readonly (readonly [number, number])[]}>;
export const fallbackScripts: readonly Script[] = Object.freeze([
  {name: 'arabic', file: 'NotoSansArabic-Regular.ttf', ranges: [[0x0600, 0x06ff], [0x0750, 0x077f], [0x08a0, 0x08ff], [0xfb50, 0xfdff], [0xfe70, 0xfeff]]},
  {name: 'hebrew', file: 'NotoSansHebrew-Regular.ttf', ranges: [[0x0590, 0x05ff], [0xfb1d, 0xfb4f]]},
  {name: 'thai', file: 'NotoSansThai-Regular.ttf', ranges: [[0x0e00, 0x0e7f]]},
  {name: 'devanagari', file: 'NotoSansDevanagari-Regular.ttf', ranges: [[0x0900, 0x097f], [0xa8e0, 0xa8ff]]},
  {name: 'bengali', file: 'NotoSansBengali-Regular.ttf', ranges: [[0x0980, 0x09ff]]},
  {name: 'tamil', file: 'NotoSansTamil-Regular.ttf', ranges: [[0x0b80, 0x0bff]]},
  // Chinese and Japanese. Korean syllables (U+AC00 to U+D7A3) are not in this font.
  {name: 'cjk', file: 'DroidSansFallbackFull.ttf', ranges: [[0x2e80, 0x30ff], [0x3100, 0x312f], [0x31a0, 0x4dbf], [0x4e00, 0x9fff], [0xf900, 0xfaff], [0xfe30, 0xfe4f], [0xff00, 0xffef]]},
]);

/** The scripts a subtitle's text uses, among those a fallback font exists for. */
export function scriptsIn(text: string): string[] {
  const found = new Set<string>();
  for (let i = 0; i < text.length && found.size < fallbackScripts.length; i++) {
    const code = text.charCodeAt(i);
    if (code < 0x0590) continue;
    for (const script of fallbackScripts) {
      if (found.has(script.name)) continue;
      for (const [from, to] of script.ranges) if (code >= from && code <= to) { found.add(script.name); break; }
    }
  }
  return fallbackScripts.filter(s => found.has(s.name)).map(s => s.name);
}

/** Addresses of the fallback fonts this subtitle needs. */
export function fallbackFontsFor(text: string): string[] {
  const wanted = scriptsIn(text);
  return fallbackScripts.filter(s => wanted.includes(s.name)).map(s => fallbackFontUrl(s.file));
}
