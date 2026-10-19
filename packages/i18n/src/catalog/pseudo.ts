import {mapMessageText} from '../messageformat.ts';
import {enUS, type MessageId} from './en-US.ts';

/**
 * en-XA, the pseudo-locale: every en-US message with its letters accented and padded to about a
 * third longer, inside brackets. A screen read in it shows at a glance the words that bypass the
 * catalogue (plain English), the labels that clip (a missing bracket) and the glyphs a font lacks.
 * It is for development only and is never offered as a language; set `region.locale` to `en-XA`,
 * or the browser's or device's language, to read it.
 */
const accents: Record<string, string> = {
  a: 'á', b: 'ƀ', c: 'ç', d: 'ð', e: 'é', f: 'ƒ', g: 'ĝ', h: 'ĥ', i: 'î', j: 'ĵ', k: 'ķ', l: 'ļ', m: 'ɱ',
  n: 'ñ', o: 'ö', p: 'þ', q: 'ǫ', r: 'ŕ', s: 'š', t: 'ţ', u: 'û', v: 'ṽ', w: 'ŵ', x: 'ẋ', y: 'ý', z: 'ž',
  A: 'Å', B: 'Ɓ', C: 'Ç', D: 'Ð', E: 'É', F: 'Ƒ', G: 'Ĝ', H: 'Ĥ', I: 'Î', J: 'Ĵ', K: 'Ķ', L: 'Ļ', M: 'Ṁ',
  N: 'Ñ', O: 'Ö', P: 'Þ', Q: 'Ǫ', R: 'Ŕ', S: 'Š', T: 'Ţ', U: 'Û', V: 'Ṽ', W: 'Ŵ', X: 'Ẋ', Y: 'Ý', Z: 'Ž',
};
const accent = (text: string) => text.replace(/[A-Za-z]/g, c => accents[c]!);

/** The padding words, accented too, so the longer text wraps the way real words do. */
const filler = 'one two three four five six seven eight nine ten'.split(' ').map(accent);
function padding(length: number): string {
  const wanted = Math.ceil(length * 0.35);
  let out = '';
  for (let i = 0; out.length < wanted; i++) out += ' ' + filler[i % filler.length];
  return out;
}

/** The visible length of a message body, without its ICU syntax, for sizing the padding. */
const visible = (source: string) => source.replace(/\{[^{}]*\}/g, 'xxxx').replace(/[{}]/g, '').length;

export function pseudoMessage(source: string): string {
  if (!source.trim()) return source;
  return mapMessageText(source, accent, () => ['[', padding(visible(source)) + ']']);
}

let made: Partial<Record<MessageId, string>> | undefined;
/** Derived on first use, like the regional catalogues. */
export function pseudoEnglish(): Partial<Record<MessageId, string>> {
  if (made) return made;
  const out: Partial<Record<MessageId, string>> = {};
  for (const [id, source] of Object.entries(enUS) as [MessageId, string][]) out[id] = pseudoMessage(source);
  return (made = out);
}
