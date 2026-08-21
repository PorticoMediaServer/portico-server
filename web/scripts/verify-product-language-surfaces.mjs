import { readFile, readdir } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..', 'src');

async function sourceFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const nested = await Promise.all(entries.map((entry) => {
    const absolute = resolve(directory, entry.name);
    if (entry.isDirectory()) return sourceFiles(absolute);
    return /\.(?:ts|tsx)$/.test(entry.name) ? [absolute] : [];
  }));
  return nested.flat();
}
const semanticSurfaces = [
  'features/search/SearchPage.tsx',
  'features/catalog/CatalogSurface.tsx',
  'features/detail/DetailActionMenu.tsx',
  'features/detail/DetailActions.tsx',
  'features/detail/DetailHierarchy.tsx',
  'features/detail/DetailPage.tsx',
  'features/library/LibraryWorkspacePage.tsx',
  'features/live-tv/DVRWorkspace.tsx',
  'features/live-tv/LiveChannels.tsx',
  'features/live-tv/LiveGuide.tsx',
  'features/live-tv/LiveTVPage.tsx',
  'runtime/RuntimeSurface.tsx',
  'runtime/RouteErrorBoundary.tsx',
];

const forbidden = [
  ['reviewedProductError', /\breviewedProductError\b/],
  ['text-only product error reduction', /\bproductErrorBody\b/],
  ['raw guide status message', /\b(?:operational|status)\.guide\.message\b/],
  ['raw DVR conflict reason', /\bconflict\.reason\b/],
  ['private route failure title', /This page closed unexpectedly/],
  ['private route retry action', /Reopen page/],
];

const failures = [];

function tokenizeJavaScript(source) {
  const tokens = [];
  const modes = [{ type: 'code' }];
  const punctuators = ['?.', '=>', '===', '!==', '==', '!=', '<=', '>=', '&&', '||', '??', '++', '--', '**', '+=', '-=', '*=', '/=', '%=', '...'];
  let index = 0;
  const push = (type, value) => tokens.push({ type, value });
  const previousAllowsRegex = () => {
    const previous = tokens.at(-1);
    if (!previous) return true;
    if (previous.type === 'identifier') return ['return', 'throw', 'case', 'delete', 'void', 'typeof', 'instanceof', 'in', 'of', 'yield', 'await'].includes(previous.value);
    return ['(', '[', '{', ',', ';', ':', '=', '!', '?', '?.', '&&', '||', '??', '=>', '+', '-', '*', '%', '&', '|', '^', '~', '<', '>'].includes(previous.value);
  };

  while (index < source.length) {
    const mode = modes.at(-1);
    if (mode.type === 'template') {
      if (source[index] === '\\') {
        index += 2;
      } else if (source[index] === '`') {
        modes.pop();
        index += 1;
      } else if (source[index] === '$' && source[index + 1] === '{') {
        modes.push({ type: 'code', templateBraceDepth: 1 });
        index += 2;
      } else {
        index += 1;
      }
      continue;
    }
    if (/\s/.test(source[index])) {
      index += 1;
      continue;
    }
    if (source.startsWith('//', index)) {
      const newline = source.indexOf('\n', index + 2);
      index = newline < 0 ? source.length : newline + 1;
      continue;
    }
    if (source.startsWith('/*', index)) {
      const close = source.indexOf('*/', index + 2);
      index = close < 0 ? source.length : close + 2;
      continue;
    }
    if (source[index] === '`') {
      let cursor = index + 1;
      let value = '';
      let hasExpression = false;
      while (cursor < source.length) {
        if (source[cursor] === '\\') {
          value += source[cursor + 1] ?? '';
          cursor += 2;
        } else if (source[cursor] === '`') {
          break;
        } else if (source[cursor] === '$' && source[cursor + 1] === '{') {
          hasExpression = true;
          break;
        } else {
          value += source[cursor];
          cursor += 1;
        }
      }
      if (!hasExpression && source[cursor] === '`') {
        push('string', value);
        index = cursor + 1;
      } else {
        modes.push({ type: 'template' });
        index += 1;
      }
      continue;
    }
    if (source[index] === '}' && mode.templateBraceDepth === 1) {
      modes.pop();
      index += 1;
      continue;
    }
    if (source[index] === '{' && mode.templateBraceDepth) mode.templateBraceDepth += 1;
    if (source[index] === '}' && mode.templateBraceDepth) mode.templateBraceDepth -= 1;
    if (source[index] === '"' || source[index] === "'") {
      const quote = source[index];
      let value = '';
      index += 1;
      while (index < source.length && source[index] !== quote) {
        if (source[index] === '\\') {
          value += source[index + 1] ?? '';
          index += 2;
        } else {
          value += source[index];
          index += 1;
        }
      }
      if (source[index] === quote) index += 1;
      push('string', value);
      continue;
    }
    if (source[index] === '/' && previousAllowsRegex()) {
      let inCharacterClass = false;
      index += 1;
      while (index < source.length) {
        if (source[index] === '\\') index += 2;
        else if (source[index] === '[') { inCharacterClass = true; index += 1; }
        else if (source[index] === ']') { inCharacterClass = false; index += 1; }
        else if (source[index] === '/' && !inCharacterClass) { index += 1; break; }
        else index += 1;
      }
      while (/[A-Za-z]/.test(source[index] ?? '')) index += 1;
      continue;
    }
    const identifier = source.slice(index).match(/^[A-Za-z_$][\w$]*/)?.[0];
    if (identifier) {
      push('identifier', identifier);
      index += identifier.length;
      continue;
    }
    const number = source.slice(index).match(/^(?:\d+(?:\.\d*)?|\.\d+)/)?.[0];
    if (number) {
      push('number', number);
      index += number.length;
      continue;
    }
    const punctuator = punctuators.find((candidate) => source.startsWith(candidate, index)) ?? source[index];
    push('punctuator', punctuator);
    index += punctuator.length;
  }
  return tokens;
}

function matchingOpen(tokens, closeIndex, open, close) {
  let depth = 0;
  for (let index = closeIndex; index >= 0; index -= 1) {
    if (tokens[index].value === close) depth += 1;
    else if (tokens[index].value === open && --depth === 0) return index;
  }
  return -1;
}

function matchingClose(tokens, openIndex, open, close) {
  let depth = 0;
  for (let index = openIndex; index < tokens.length; index += 1) {
    if (tokens[index].value === open) depth += 1;
    else if (tokens[index].value === close && --depth === 0) return index;
  }
  return -1;
}

function staticProperty(tokens) {
  if (tokens.length === 0) return null;
  while (tokens[0]?.value === '(' && matchingClose(tokens, 0, '(', ')') === tokens.length - 1) {
    tokens = tokens.slice(1, -1);
  }
  let value = '';
  for (let index = 0; index < tokens.length; index += 1) {
    if (index % 2 === 0) {
      if (tokens[index].type !== 'string') return null;
      value += tokens[index].value;
    } else if (tokens[index].value !== '+') {
      return null;
    }
  }
  return value;
}

function identifierWords(value) {
  return value
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .split(/[^A-Za-z0-9]+/)
    .filter(Boolean)
    .map((word) => word.toLowerCase());
}

function isJobSourceName(value) {
  const words = identifierWords(value);
  return words.some((word) => word === 'job' || word === 'jobs')
    && !words.some((word) => word === 'error' || word === 'message');
}

function arrowFunctionTokenIndexes(tokens) {
  const ignored = new Set();
  for (let index = 0; index < tokens.length; index += 1) {
    if (tokens[index].value !== '=>') continue;
    if (tokens[index - 1]?.value === ')') {
      const open = matchingOpen(tokens, index - 1, '(', ')');
      if (open >= 0) for (let cursor = open; cursor < index; cursor += 1) ignored.add(cursor);
    } else if (tokens[index - 1]?.type === 'identifier') {
      ignored.add(index - 1);
    }
    ignored.add(index);
    if (tokens[index + 1]?.value === '{') {
      const close = matchingClose(tokens, index + 1, '{', '}');
      if (close >= 0) for (let cursor = index + 1; cursor <= close; cursor += 1) ignored.add(cursor);
      continue;
    }
    const stack = [];
    const pairs = new Map([['(', ')'], ['[', ']'], ['{', '}']]);
    for (let cursor = index + 1; cursor < tokens.length; cursor += 1) {
      const value = tokens[cursor].value;
      if (stack.length === 0 && (value === ',' || value === ')' || value === ']' || value === '}')) break;
      ignored.add(cursor);
      if (pairs.has(value)) stack.push(pairs.get(value));
      else if (value === stack.at(-1)) stack.pop();
    }
  }
  return ignored;
}

function expressionStart(tokens, endIndex) {
  const closing = new Map([[')', '('], [']', '['], ['}', '{']]);
  const opening = new Set(['(', '[', '{']);
  const stack = [];
  for (let index = endIndex; index >= 0; index -= 1) {
    const value = tokens[index].value;
    if (closing.has(value)) stack.push(closing.get(value));
    else if (opening.has(value)) {
      if (stack.at(-1) === value) stack.pop();
      else if (stack.length === 0) return index + 1;
    }
    if (stack.length === 0 && ([';', ',', '=', '=>', ':'].includes(value) || ['return', 'throw', 'case'].includes(value))) return index + 1;
  }
  return 0;
}

function expressionIsJobDerived(tokens, tainted) {
  const ignored = arrowFunctionTokenIndexes(tokens);
  for (let index = 0; index < tokens.length; index += 1) {
    if (ignored.has(index)) continue;
    const token = tokens[index];
    if (token.type === 'identifier' && tainted.has(token.value)) return true;
    if (token.type === 'identifier' && isJobSourceName(token.value)) return true;
    if (token.value !== '[') continue;
    const close = matchingClose(tokens, index, '[', ']');
    if (close < 0) continue;
    const propertyTokens = tokens.slice(index + 1, close);
    const property = staticProperty(propertyTokens);
    if (property && isJobSourceName(property)) return true;
    if (property === null && expressionIsJobDerived(propertyTokens, tainted)) return true;
    index = close;
  }
  return false;
}

function splitTopLevel(tokens, separator) {
  const slices = [];
  const stack = [];
  const pairs = new Map([['(', ')'], ['[', ']'], ['{', '}']]);
  let start = 0;
  for (let index = 0; index < tokens.length; index += 1) {
    const value = tokens[index].value;
    if (pairs.has(value)) stack.push(pairs.get(value));
    else if (value === stack.at(-1)) stack.pop();
    else if (value === separator && stack.length === 0) {
      slices.push(tokens.slice(start, index));
      start = index + 1;
    }
  }
  slices.push(tokens.slice(start));
  return slices;
}

function topLevelIndex(tokens, value) {
  const stack = [];
  const pairs = new Map([['(', ')'], ['[', ']'], ['{', '}']]);
  for (let index = 0; index < tokens.length; index += 1) {
    const tokenValue = tokens[index].value;
    if (pairs.has(tokenValue)) stack.push(pairs.get(tokenValue));
    else if (tokenValue === stack.at(-1)) stack.pop();
    else if (tokenValue === value && stack.length === 0) return index;
  }
  return -1;
}

function declarationAssignments(tokens) {
  const assignments = [];
  for (let index = 0; index < tokens.length; index += 1) {
    if (!['const', 'let', 'var'].includes(tokens[index].value)) continue;
    let end = index + 1;
    const stack = [];
    const pairs = new Map([['(', ')'], ['[', ']'], ['{', '}']]);
    while (end < tokens.length) {
      const value = tokens[end].value;
      if (pairs.has(value)) stack.push(pairs.get(value));
      else if (value === stack.at(-1)) stack.pop();
      if (value === ';' && stack.length === 0) break;
      end += 1;
    }
    for (const declarator of splitTopLevel(tokens.slice(index + 1, end), ',')) {
      const equals = topLevelIndex(declarator, '=');
      if (equals > 0) assignments.push({ lhs: declarator.slice(0, equals), rhs: declarator.slice(equals + 1) });
    }
    index = end;
  }
  return assignments;
}

function directAssignments(tokens) {
  const assignments = [];
  for (let index = 0; index < tokens.length - 2; index += 1) {
    if (tokens[index].type !== 'identifier' || tokens[index + 1].value !== '=') continue;
    if (['const', 'let', 'var'].includes(tokens[index - 1]?.value)) continue;
    let end = index + 2;
    const stack = [];
    const pairs = new Map([['(', ')'], ['[', ']'], ['{', '}']]);
    while (end < tokens.length) {
      const value = tokens[end].value;
      if (pairs.has(value)) stack.push(pairs.get(value));
      else if (value === stack.at(-1)) stack.pop();
      if (value === ';' && stack.length === 0) break;
      end += 1;
    }
    assignments.push({ lhs: [tokens[index]], rhs: tokens.slice(index + 2, end) });
  }
  return assignments;
}

function destructuringAssignments(tokens) {
  const assignments = [];
  for (let index = 0; index < tokens.length; index += 1) {
    const open = tokens[index].value;
    if (open !== '{' && open !== '[') continue;
    const closeValue = open === '{' ? '}' : ']';
    const close = matchingClose(tokens, index, open, closeValue);
    if (close < 0 || tokens[close + 1]?.value !== '=') continue;
    const previous = tokens[index - 1]?.value;
    if (previous && !['(', ';', ',', '='].includes(previous)) continue;
    let end = close + 2;
    const stack = [];
    const pairs = new Map([['(', ')'], ['[', ']'], ['{', '}']]);
    while (end < tokens.length) {
      const value = tokens[end].value;
      if (pairs.has(value)) stack.push(pairs.get(value));
      else if (value === stack.at(-1)) stack.pop();
      if ((value === ';' || value === ')') && stack.length === 0) break;
      end += 1;
    }
    assignments.push({ lhs: tokens.slice(index, close + 1), rhs: tokens.slice(close + 2, end) });
    index = close;
  }
  return assignments;
}

function destructuredBindings(lhs) {
  const bindings = [];
  const messageReads = [];
  if (lhs[0]?.value === '{' && lhs.at(-1)?.value === '}') {
    for (const property of splitTopLevel(lhs.slice(1, -1), ',')) {
      const colon = topLevelIndex(property, ':');
      const keyTokens = colon >= 0 ? property.slice(0, colon) : property;
      const valueTokens = colon >= 0 ? property.slice(colon + 1) : property;
      const computedKey = keyTokens[0]?.value === '[' && matchingClose(keyTokens, 0, '[', ']') === keyTokens.length - 1
        ? staticProperty(keyTokens.slice(1, -1))
        : null;
      const key = keyTokens.length === 1 && ['identifier', 'string'].includes(keyTokens[0].type)
        ? keyTokens[0].value
        : computedKey ?? staticProperty(keyTokens);
      const binding = valueTokens.findLast((token) => token.type === 'identifier')?.value;
      if (binding) bindings.push({ binding, key });
      if (key === 'message') messageReads.push(binding ?? 'message');
    }
  } else if (lhs[0]?.value === '[' && lhs.at(-1)?.value === ']') {
    for (const token of lhs.slice(1, -1)) if (token.type === 'identifier') bindings.push({ binding: token.value, key: null });
  }
  return { bindings, messageReads };
}

function containsRawJobMessage(source) {
  const tokens = tokenizeJavaScript(source);
  const tainted = new Set();
  const assignments = [...declarationAssignments(tokens), ...directAssignments(tokens), ...destructuringAssignments(tokens)];
  let changed = true;
  let rawDestructure = false;
  while (changed) {
    changed = false;
    for (const { lhs, rhs } of assignments) {
      const rhsTainted = expressionIsJobDerived(rhs, tainted);
      if (lhs.length === 1 && lhs[0].type === 'identifier') {
        if (rhsTainted && !tainted.has(lhs[0].value)) {
          tainted.add(lhs[0].value);
          changed = true;
        }
        continue;
      }
      const { bindings, messageReads } = destructuredBindings(lhs);
      if (rhsTainted && messageReads.length > 0) rawDestructure = true;
      for (const { binding, key } of bindings) {
        if ((rhsTainted || (key && isJobSourceName(key))) && !tainted.has(binding)) {
          tainted.add(binding);
          changed = true;
        }
      }
    }
  }
  if (rawDestructure) return true;

  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (token.type === 'identifier' && token.value === 'message' && ['.', '?.'].includes(tokens[index - 1]?.value)) {
      const end = index - 2;
      if (expressionIsJobDerived(tokens.slice(expressionStart(tokens, end), end + 1), tainted)) return true;
    }
    if (token.value === '[') {
      const close = matchingClose(tokens, index, '[', ']');
      if (close > index && staticProperty(tokens.slice(index + 1, close)) === 'message') {
        const end = tokens[index - 1]?.value === '?.' ? index - 2 : index - 1;
        if (expressionIsJobDerived(tokens.slice(expressionStart(tokens, end), end + 1), tainted)) return true;
      }
    }
    if (token.type === 'identifier' && token.value === 'Reflect' && tokens[index + 1]?.value === '.' && tokens[index + 2]?.value === 'get' && tokens[index + 3]?.value === '(') {
      const close = matchingClose(tokens, index + 3, '(', ')');
      if (close > 0) {
        const args = splitTopLevel(tokens.slice(index + 4, close), ',');
        if (args.length >= 2 && staticProperty(args[1]) === 'message' && expressionIsJobDerived(args[0], tainted)) return true;
      }
    }
  }
  return false;
}

for (const sample of [
  'job.message',
  'option.job?.message',
  'createdState.scanJob?.message',
  'jobs[0].message',
  'jobs?.[0]?.message',
  'jobs?.[index].message',
  'jobs?.[0]?.["message"]',
  'state?.jobs?.[0]?.message',
  'option["job"]?.message',
  'jobList.at(0).message',
  'jobList.at?.(0).message',
  'jobList.find((job) => job.status === "done")?.message',
  '(jobs?.[0])?.message',
  'const value = (jobs?.[0])?.message',
  'option[jobKey]?.message',
  'active.mediaJob["message"]',
  "active.mediaJob?.['message']",
  '`${state.jobs?.[0]?.message}`',
  'const { message } = job',
  'const { message: rawMessage } = scanJob',
  'const selected = jobs.at(0); selected.message',
  'let selected; selected = jobs.find((entry) => entry.status === "done"); selected?.message',
  'const list = response.jobs; const selected = list[index]; selected["mes" + "sage"]',
  'const list = jobs; const [first] = list; first.message',
  'const { scanJob: selected } = response; selected.message',
  'let rawMessage; ({ message: rawMessage } = jobs.at(0))',
  'const selected = jobs.at(0); selected[`message`]',
  'const selected = jobs.at(0); selected[("mes" + "sage")]',
  'const { ["mes" + "sage"]: rawMessage } = jobs.at(0)',
  'const selected = jobs.at(0); Reflect.get(selected, "mes" + "sage")',
  'const selected = jobs.at(0); Reflect.get(selected, ("message"))',
  'Reflect.get(jobs.at(0), `message`)',
  'items.map((job) => job.status); job.message',
]) {
  if (!containsRawJobMessage(sample)) failures.push(`verifier regression: raw job sample was not rejected: ${sample}`);
}
for (const sample of [
  'job.status',
  'scanJob?.progress',
  'jobMessage',
  'message',
  'error.message',
  'jobError.message',
  'const failure = jobError; failure.message',
  'const failure = new Error("job failed"); failure.message',
  'const { message } = jobError',
  'Reflect.get(jobError, "message")',
  'option["job"].status',
  'items.find((job) => job.active)?.message',
  'items.find((job) => ({ status: job.status }))?.message',
  '// jobs?.[0]?.message',
  'const pattern = /jobs?.message/;',
  '`jobs?.[0]?.message`',
]) {
  if (containsRawJobMessage(sample)) failures.push(`verifier regression: safe job sample was rejected: ${sample}`);
}
const allSourceFiles = await sourceFiles(root);
const importPattern = /^\s*import\s+(?:type\s+)?(?:[^;]*?\s+from\s+)?['"]([^'"]+)['"]\s*;?/gm;
for (const absolute of allSourceFiles) {
  const source = await readFile(absolute, 'utf8');
  const relative = absolute.slice(root.length + 1);
  if (containsRawJobMessage(source)) failures.push(`${relative}: contains raw job message access`);
  const imports = new Map();
  for (const match of source.matchAll(importPattern)) {
    const module = match[1];
    imports.set(module, (imports.get(module) ?? 0) + 1);
  }
  for (const [module, count] of imports) {
    if (count > 1) failures.push(`${relative}: imports ${module} ${count} times`);
  }
}
for (const relative of semanticSurfaces) {
  const source = await readFile(resolve(root, relative), 'utf8');
  for (const [label, pattern] of forbidden) {
    if (pattern.test(source)) failures.push(`${relative}: contains ${label}`);
  }
}

for (const relative of ['features/settings/StatusDashboard.tsx', 'features/settings/LiveTVOperations.tsx']) {
  const source = await readFile(resolve(root, relative), 'utf8');
  for (const [label, pattern] of forbidden.slice(2, 4)) {
    if (pattern.test(source)) failures.push(`${relative}: contains ${label}`);
  }
}

const liveFormat = await readFile(resolve(root, 'features/live-tv/liveFormat.ts'), 'utf8');
for (const symbol of ['ProductMessageId', 'ProductMessageVariables', 'productProblem']) {
  const count = liveFormat.match(new RegExp(`\\b${symbol}\\b`, 'g'))?.length ?? 0;
  if (count < 2) failures.push(`features/live-tv/liveFormat.ts: ${symbol} is not imported and used`);
}
if (liveFormat.indexOf('export function') < liveFormat.lastIndexOf('\nimport ')) {
  failures.push('features/live-tv/liveFormat.ts: an import appears after executable declarations');
}

const libraryCss = await readFile(resolve(root, 'features/library/library.css'), 'utf8');
if (!/@media\s*\(max-width:\s*760px\)[\s\S]*?\.library-alpha-rail button\s*\{[\s\S]*?min-width:\s*44px;[\s\S]*?min-height:\s*44px;/.test(libraryCss)) {
  failures.push('features/library/library.css: compact alpha targets are not fixed at a 44px minimum');
}

if (failures.length) {
  console.error(`Product-language surface verification failed:\n- ${failures.join('\n- ')}`);
  process.exitCode = 1;
} else {
  console.log(`Verified ${allSourceFiles.length} Web source files, ${semanticSurfaces.length} semantic surfaces, and compact alpha target sizing.`);
}
