import {readFileSync, readdirSync} from 'node:fs';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
import {transformSync} from 'rolldown/utils';

// The bundler's eager glob is a build-time import list, not a runtime helper.
// Expanding it into real imports keeps the modules (and their route selection)
// the ones the app builds, instead of substituting an empty registry.
const GLOB = 'import.meta.glob';
// Vite replaces import.meta.env at transform time; Node has no such object.
const ENVIRONMENT = 'const __porticoEnv = Object.freeze({DEV: true, PROD: false, MODE: "development", BASE_URL: "/", SSR: false});\n';
const usesEnvironment = source => source.includes('import.meta.env');

export function transformTsx(url) {
  const filename = fileURLToPath(url);
  const original = readFileSync(filename, 'utf8');
  const environment = usesEnvironment(original);
  const source = environment ? original.replaceAll('import.meta.env', '__porticoEnv') : original;
  const {prelude, source: body} = expandGlobs(source, filename);
  return compile(filename, (environment ? ENVIRONMENT : '') + prelude + body, 'tsx');
}

/** Type-only modules the app runs through a bundler reach Vite's environment too. */
export function transformTs(url) {
  const filename = fileURLToPath(url);
  return compile(filename, ENVIRONMENT + readFileSync(filename, 'utf8').replaceAll('import.meta.env', '__porticoEnv'), 'ts');
}

export function usesBundlerEnvironment(url) {
  return usesEnvironment(readFileSync(fileURLToPath(url), 'utf8'));
}

function compile(filename, source, lang) {
  const result = transformSync(filename, source, {lang, jsx: {runtime: 'automatic'}, target: 'es2022'});
  if (result.errors.length) throw new AggregateError(result.errors, `Cannot transform ${filename}`);
  return result.code;
}

function expandGlobs(source, filename) {
  let head = '', out = '', cursor = 0, count = 0;
  for (;;) {
    const start = source.indexOf(GLOB, cursor);
    if (start === -1) return {prelude: head, source: out + source.slice(cursor)};
    out += source.slice(cursor, start);
    const call = readGlobCall(source, start + GLOB.length);
    const glob = globExpression(call, filename, count++);
    head += glob.head;
    out += glob.expression;
    cursor = call.end;
  }
}

/** Read `[<type>](pattern, options)` after the `import.meta.glob` token. */
function readGlobCall(source, index) {
  let at = skipTrivia(source, index);
  if (source[at] === '<') at = skipBalanced(source, at, '<', '>');
  at = skipTrivia(source, at);
  if (source[at] !== '(') throw new SyntaxError(`import.meta.glob without a call at ${at}`);
  const close = skipBalanced(source, at, '(', ')');
  const args = splitArguments(source.slice(at + 1, close - 1));
  if (args.length < 1 || args.length > 2) throw new SyntaxError('import.meta.glob takes a pattern and options');
  return {pattern: stringLiteral(args[0]), options: args[1] === undefined ? {} : globOptions(args[1]), end: close};
}

function globOptions(text) {
  const options = {};
  const body = text.trim().replace(/^\{|\}$/g, '');
  for (const entry of splitArguments(body)) {
    if (!entry.trim()) continue;
    const match = /^([\w$]+)\s*:\s*([\s\S]+)$/.exec(entry.trim());
    if (!match) throw new SyntaxError(`unsupported import.meta.glob option: ${entry}`);
    const [, key, value] = match;
    if (!['eager', 'query', 'import'].includes(key)) throw new SyntaxError(`unsupported import.meta.glob option: ${key}`);
    options[key] = key === 'eager' ? value.trim() === 'true' : stringLiteral(value);
  }
  return options;
}

/** Static imports carry the glob's modules, as the bundler's eager glob does. */
function globExpression(call, filename, glob) {
  const directory = path.dirname(filename);
  const query = call.options.query ?? '';
  if (query && query !== '?url' && query !== '?raw') throw new SyntaxError(`unsupported import.meta.glob query: ${query}`);
  const pick = call.options.import;
  let head = '';
  let count = 0;
  const entries = globFiles(call.pattern, directory).map(file => {
    const specifier = JSON.stringify(relativeSpecifier(directory, file) + query);
    const local = `__porticoGlob${glob}_${count++}`;
    if (!call.options.eager) return `${specifier}: () => import(${specifier})${pick ? `.then(module => module.${pick})` : ''}`;
    head += pick === 'default' ? `import ${local} from ${specifier};\n`
      : pick ? `import {${pick} as ${local}} from ${specifier};\n`
      : `import * as ${local} from ${specifier};\n`;
    return `${specifier}: ${local}`;
  });
  return {head, expression: `({${entries.join(', ')}})`};
}

function globFiles(pattern, directory) {
  const regex = globRegExp(pattern);
  const root = globRoot(pattern, directory);
  const files = [];
  const visit = current => {
    for (const entry of readdirSync(current, {withFileTypes: true})) {
      if (entry.name.startsWith('.')) continue;
      const full = path.join(current, entry.name);
      if (entry.isDirectory()) visit(full);
      else if (entry.isFile() && regex.test(relativeSpecifier(directory, full))) files.push(full);
    }
  };
  try {
    visit(root);
  } catch (error) {
    if (error?.code !== 'ENOENT') throw error;
  }
  return files.sort();
}

/** The deepest directory the pattern names literally, so an absent tree is empty like Vite. */
function globRoot(pattern, directory) {
  const magic = pattern.search(/[*?{[]/);
  const staticPart = magic === -1 ? pattern : pattern.slice(0, magic);
  const base = path.resolve(directory, path.dirname(staticPart) === '.' ? staticPart : path.dirname(staticPart));
  return magic !== -1 && staticPart.endsWith('/') ? path.resolve(directory, staticPart) : base;
}

function relativeSpecifier(directory, file) {
  const relative = path.relative(directory, file).split(path.sep).join('/');
  return relative.startsWith('.') ? relative : `./${relative}`;
}

function globRegExp(pattern) {
  let out = '';
  for (let index = 0; index < pattern.length; index++) {
    const character = pattern[index];
    if (character === '*') {
      if (pattern[index + 1] === '*') {
        if (pattern[index + 2] === '/') { out += '(?:[^/]+/)*'; index += 2; } else { out += '.*'; index += 1; }
      } else {
        out += '[^/]*';
      }
    } else if (character === '?') {
      out += '[^/]';
    } else if (character === '{') {
      const close = pattern.indexOf('}', index);
      if (close === -1) throw new SyntaxError(`unterminated brace in glob: ${pattern}`);
      out += `(?:${pattern.slice(index + 1, close).split(',').map(escapeRegExp).join('|')})`;
      index = close;
    } else {
      out += escapeRegExp(character);
    }
  }
  return new RegExp(`^${out}$`);
}

function escapeRegExp(text) {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function skipTrivia(source, index) {
  for (;;) {
    while (/\s/.test(source[index] ?? '')) index++;
    if (source.startsWith('//', index)) index = source.indexOf('\n', index) + 1 || source.length;
    else if (source.startsWith('/*', index)) index = source.indexOf('*/', index) + 2;
    else return index;
  }
}

/** Index just past the delimiter closing the one at `start`. */
function skipBalanced(source, start, open, close) {
  let depth = 0;
  for (let index = start; index < source.length; index++) {
    const character = source[index];
    if (character === "'" || character === '"' || character === '`') index = skipString(source, index) - 1;
    else if (character === '/' && source[index + 1] === '/') index = source.indexOf('\n', index);
    else if (character === '/' && source[index + 1] === '*') index = source.indexOf('*/', index) + 1;
    else if (character === open) depth++;
    else if (character === close && --depth === 0) return index + 1;
  }
  throw new SyntaxError(`unbalanced ${open}${close}`);
}

function skipString(source, start) {
  const quote = source[start];
  for (let index = start + 1; index < source.length; index++) {
    if (source[index] === '\\') index++;
    else if (source[index] === quote) return index + 1;
  }
  throw new SyntaxError('unterminated string');
}

/** Top-level comma-separated arguments, ignoring commas inside values. */
function splitArguments(text) {
  const parts = [];
  let start = 0;
  for (let index = 0; index < text.length; index++) {
    const character = text[index];
    if (character === "'" || character === '"' || character === '`') index = skipString(text, index) - 1;
    else if ('([{'.includes(character)) index = skipBalanced(text, index, character, {'(': ')', '[': ']', '{': '}'}[character]) - 1;
    else if (character === ',') { parts.push(text.slice(start, index)); start = index + 1; }
  }
  parts.push(text.slice(start));
  return parts;
}

function stringLiteral(text) {
  const value = text.trim();
  const quote = value[0];
  if (!["'", '"', '`'].includes(quote) || !value.endsWith(quote) || value.length < 2) {
    throw new SyntaxError(`expected a string literal, found ${value}`);
  }
  if (quote === '`' && value.includes('${')) throw new SyntaxError(`unsupported template substitution in ${value}`);
  return value.slice(1, -1).replace(/\\(.)/g, '$1');
}
