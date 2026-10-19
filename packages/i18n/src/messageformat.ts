/**
 * A small ICU MessageFormat implementation (the FormatJS / `intl-messageformat`
 * syntax subset Portico uses): `{arg}`, `{arg, number[, integer|percent|::group-off]}`,
 * `{arg, date|time[, short|medium|long|full]}`, `{arg, plural, [offset:n] =0 {…} one {…} other {…}}`
 * with `#`, `{arg, selectordinal, …}`, `{arg, select, key {…} other {…}}`, and
 * apostrophe quoting (`''` is a quote, `'{…}'` is literal).
 *
 * Why not the `intl-messageformat` package: the Apple app's Metro resolver
 * doesn't look outside `portico-react-native/apps/apple/node_modules`, and the shared
 * packages must load unchanged on web, iOS, tvOS and in node tests. Catalogue
 * files are plain ICU strings, so swapping in FormatJS later is a drop-in.
 */
export type MessageValue = string | number | Date | boolean | null | undefined;
export type MessageValues = Readonly<Record<string, MessageValue>>;

type Node =
  | {kind: 'text'; value: string}
  | {kind: 'pound'}
  | {kind: 'arg'; name: string}
  | {kind: 'number'; name: string; style?: string}
  | {kind: 'date' | 'time'; name: string; style?: string}
  | {kind: 'plural'; name: string; ordinal: boolean; offset: number; options: Record<string, Node[]>}
  | {kind: 'select'; name: string; options: Record<string, Node[]>};

export class MessageSyntaxError extends Error {
  readonly source: string;
  constructor(message: string, source: string) { super(`${message} in message: ${source}`); this.name = 'MessageSyntaxError'; this.source = source; }
}

export function parseMessage(source: string): Node[] {
  let i = 0;
  const fail = (why: string): never => { throw new MessageSyntaxError(why, source); };
  const ws = () => { while (i < source.length && /\s/.test(source[i]!)) i++; };
  const ident = (): string => { ws(); const start = i; while (i < source.length && /[^\s,{}#]/.test(source[i]!)) i++; if (start === i) fail('expected a name'); return source.slice(start, i); };
  function nodes(inPlural: boolean, depth: number): Node[] {
    const out: Node[] = [];
    let text = '';
    const flush = () => { if (text) { out.push({kind: 'text', value: text}); text = ''; } };
    while (i < source.length) {
      const c = source[i]!;
      if (c === "'") {
        if (source[i + 1] === "'") { text += "'"; i += 2; continue; }
        const next = source[i + 1];
        if (next === '{' || next === '}' || (inPlural && next === '#')) {
          const end = source.indexOf("'", i + 1);
          if (end < 0) { text += source.slice(i + 1); i = source.length; continue; }
          text += source.slice(i + 1, end).replace(/''/g, "'"); i = end + 1; continue;
        }
        text += c; i++; continue;
      }
      if (c === '}') { if (depth === 0) fail('unmatched }'); break; }
      if (c === '#' && inPlural) { flush(); out.push({kind: 'pound'}); i++; continue; }
      if (c === '{') { flush(); i++; out.push(argument(depth)); continue; }
      text += c; i++;
    }
    flush();
    return out;
  }
  function options(ordinalOrPlural: boolean, depth: number): {offset: number; options: Record<string, Node[]>} {
    const opts: Record<string, Node[]> = {};
    let offset = 0;
    ws();
    if (ordinalOrPlural && source.startsWith('offset:', i)) { i += 7; ws(); const m = /^\d+/.exec(source.slice(i)); if (!m) fail('bad offset'); offset = Number(m![0]); i += m![0].length; }
    for (;;) {
      ws();
      if (source[i] === '}') break;
      const key = ident();
      ws();
      if (source[i] !== '{') fail(`expected { after ${key}`);
      i++;
      opts[key] = nodes(ordinalOrPlural, depth + 1);
      if (source[i] !== '}') fail('unclosed option');
      i++;
    }
    if (!opts.other) fail('missing other');
    return {offset, options: opts};
  }
  function argument(depth: number): Node {
    const name = ident();
    ws();
    if (source[i] === '}') { i++; return {kind: 'arg', name}; }
    if (source[i] !== ',') fail('expected , or }');
    i++;
    const type = ident();
    ws();
    let node: Node;
    if (type === 'plural' || type === 'selectordinal' || type === 'select') {
      if (source[i] !== ',') fail(`expected , after ${type}`);
      i++;
      const {offset, options: opts} = options(type !== 'select', depth);
      node = type === 'select' ? {kind: 'select', name, options: opts} : {kind: 'plural', name, ordinal: type === 'selectordinal', offset, options: opts};
    } else if (type === 'number' || type === 'date' || type === 'time') {
      let style: string | undefined;
      if (source[i] === ',') { i++; style = ident(); ws(); }
      node = type === 'number' ? {kind: 'number', name, style} : {kind: type, name, style};
    } else {
      return fail(`unknown argument type ${type}`);
    }
    ws();
    if (source[i] !== '}') fail('unclosed argument');
    i++;
    return node;
  }
  const result = nodes(false, 0);
  if (i < source.length) fail('unexpected }');
  return result;
}

/**
 * `locale` formats numbers and dates; `pluralLocale` (default `locale`) picks plural forms. They
 * differ when the words are in another language than the formats: a French catalogue read with
 * Canadian-English dates still needs French plural rules, and an English catalogue on a Japanese
 * device still needs "one" and "other".
 */
export type FormatOptions = {locale: string; pluralLocale?: string; timeZone?: string; hour12?: boolean};

const pluralCache = new Map<string, Intl.PluralRules | null>();
function pluralCategory(locale: string, n: number, ordinal: boolean): string {
  const key = `${locale}|${ordinal}`;
  let rules = pluralCache.get(key);
  if (rules === undefined) {
    try { rules = typeof Intl !== 'undefined' && Intl.PluralRules ? new Intl.PluralRules(locale, {type: ordinal ? 'ordinal' : 'cardinal'}) : null; } catch { rules = null; }
    pluralCache.set(key, rules);
  }
  if (rules) return rules.select(n);
  // English fallback for runtimes without Intl.PluralRules.
  if (!ordinal) return n === 1 ? 'one' : 'other';
  const t = n % 10, h = n % 100;
  return t === 1 && h !== 11 ? 'one' : t === 2 && h !== 12 ? 'two' : t === 3 && h !== 13 ? 'few' : 'other';
}

function numberText(value: number, o: FormatOptions, style?: string): string {
  try {
    // `::group-off` (an ICU skeleton): a number that is a name more than an amount, "1080p" and not "1,080p".
    const opts: Intl.NumberFormatOptions = style === 'integer' ? {maximumFractionDigits: 0} : style === 'percent' ? {style: 'percent', maximumFractionDigits: 0} : style === '::group-off' ? {useGrouping: false} : {};
    return new Intl.NumberFormat(o.locale, opts).format(value);
  } catch { return String(value); }
}

function dateText(value: Date | number, o: FormatOptions, kind: 'date' | 'time', style = 'medium'): string {
  const d = value instanceof Date ? value : new Date(value);
  if (!Number.isFinite(d.getTime())) return '';
  const s = (['short', 'medium', 'long', 'full'].includes(style) ? style : 'medium') as 'short' | 'medium' | 'long' | 'full';
  try {
    return new Intl.DateTimeFormat(o.locale, {...(kind === 'date' ? {dateStyle: s} : {timeStyle: s === 'full' || s === 'long' ? 'short' : s}), timeZone: o.timeZone, ...(kind === 'time' && o.hour12 !== undefined ? {hour12: o.hour12} : {})}).format(d);
  } catch { return kind === 'date' ? d.toDateString() : d.toTimeString().slice(0, 5); }
}

function render(nodes: Node[], values: MessageValues, o: FormatOptions, pound?: number): string {
  let out = '';
  for (const n of nodes) {
    switch (n.kind) {
      case 'text': out += n.value; break;
      case 'pound': out += pound === undefined ? '#' : numberText(pound, o); break;
      case 'arg': { const v = values[n.name]; out += v instanceof Date ? dateText(v, o, 'date') : typeof v === 'number' ? numberText(v, o) : v == null ? '' : String(v); break; }
      case 'number': { const v = Number(values[n.name]); out += Number.isFinite(v) ? numberText(v, o, n.style) : ''; break; }
      case 'date': case 'time': { const v = values[n.name]; out += v instanceof Date || typeof v === 'number' ? dateText(v, o, n.kind, n.style) : ''; break; }
      case 'plural': {
        const raw = Number(values[n.name]);
        const v = Number.isFinite(raw) ? raw : 0;
        const exact = n.options[`=${v}`];
        const branch = exact ?? n.options[pluralCategory(o.pluralLocale ?? o.locale, v - n.offset, n.ordinal)] ?? n.options.other!;
        out += render(branch, values, o, v - n.offset);
        break;
      }
      case 'select': { const key = String(values[n.name] ?? 'other'); out += render(n.options[key] ?? n.options.other!, values, o, pound); break; }
    }
  }
  return out;
}

const parsed = new Map<string, Node[]>();
/** Format an ICU message. Parses once per distinct source string. */
export function formatMessage(source: string, values: MessageValues = {}, options: FormatOptions = {locale: 'en-US'}): string {
  let nodes = parsed.get(source);
  if (!nodes) { nodes = parseMessage(source); parsed.set(source, nodes); }
  return render(nodes, values, options);
}

/** Argument names a message uses (for catalogue drift checks). */
export function messageArguments(source: string): string[] {
  const names = new Set<string>();
  const walk = (nodes: Node[]) => { for (const n of nodes) { if ('name' in n) names.add(n.name); if (n.kind === 'plural' || n.kind === 'select') Object.values(n.options).forEach(walk); } };
  walk(parseMessage(source));
  return [...names].sort();
}

export type ArgumentKind = 'text' | 'number' | 'date' | 'time' | 'plural' | 'selectordinal' | 'select';

/**
 * What a message needs from its caller, for checking a translation against its source: each
 * argument's kind, each select's keys, and each plural's exact (`=n`) keys, offset and categories.
 * An argument used more than once keeps its most specific kind (a plural beats a bare `{n}`).
 */
export type MessageStructure = Readonly<{
  args: Readonly<Record<string, ArgumentKind>>;
  selects: Readonly<Record<string, readonly string[]>>;
  plurals: readonly Readonly<{name: string; ordinal: boolean; offset: number; exact: readonly string[]; categories: readonly string[]}>[];
}>;

const kindRank: Record<ArgumentKind, number> = {text: 0, number: 1, date: 1, time: 1, select: 2, plural: 2, selectordinal: 2};

export function messageStructure(source: string): MessageStructure {
  const args: Record<string, ArgumentKind> = {};
  const selects: Record<string, string[]> = {};
  const plurals: {name: string; ordinal: boolean; offset: number; exact: string[]; categories: string[]}[] = [];
  const note = (name: string, kind: ArgumentKind) => { const had = args[name]; if (!had || kindRank[kind] > kindRank[had]) args[name] = kind; };
  const walk = (nodes: Node[]) => {
    for (const n of nodes) {
      switch (n.kind) {
        case 'text': case 'pound': break;
        case 'arg': note(n.name, 'text'); break;
        case 'number': note(n.name, 'number'); break;
        case 'date': case 'time': note(n.name, n.kind); break;
        case 'select': note(n.name, 'select'); selects[n.name] = [...new Set([...(selects[n.name] ?? []), ...Object.keys(n.options)])].sort(); Object.values(n.options).forEach(walk); break;
        case 'plural': {
          note(n.name, n.ordinal ? 'selectordinal' : 'plural');
          const keys = Object.keys(n.options);
          plurals.push({name: n.name, ordinal: n.ordinal, offset: n.offset, exact: keys.filter(k => k.startsWith('=')).sort(), categories: keys.filter(k => !k.startsWith('=')).sort()});
          Object.values(n.options).forEach(walk);
          break;
        }
      }
    }
  };
  walk(parseMessage(source));
  return {args, selects, plurals};
}

/**
 * Rewrite a message's literal text, keeping its arguments, plurals and selects exactly; the result
 * is valid ICU again (quotes, braces and `#` re-escaped). The pseudo-locale is made with this.
 */
export function mapMessageText(source: string, map: (text: string) => string, wrap?: (body: string) => [before: string, after: string]): string {
  const quote = (text: string, inPlural: boolean) => {
    let out = text.replace(/'/g, "''");
    out = out.replace(inPlural ? /[{}#]+/g : /[{}]+/g, run => `'${run}'`);
    return out;
  };
  const print = (nodes: Node[], inPlural: boolean): string => nodes.map(n => {
    switch (n.kind) {
      case 'text': return quote(map(n.value), inPlural);
      case 'pound': return '#';
      case 'arg': return `{${n.name}}`;
      case 'number': case 'date': case 'time': return `{${n.name}, ${n.kind}${n.style ? `, ${n.style}` : ''}}`;
      case 'select': return `{${n.name}, select, ${Object.entries(n.options).map(([k, v]) => `${k} {${print(v, inPlural)}}`).join(' ')}}`;
      case 'plural': return `{${n.name}, ${n.ordinal ? 'selectordinal' : 'plural'}, ${n.offset ? `offset:${n.offset} ` : ''}${Object.entries(n.options).map(([k, v]) => `${k} {${print(v, true)}}`).join(' ')}}`;
    }
  }).join('');
  const body = print(parseMessage(source), false);
  if (!wrap) return body;
  const [before, after] = wrap(body);
  return quote(before, false) + body + quote(after, false);
}
