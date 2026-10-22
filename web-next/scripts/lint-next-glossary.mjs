import {pathToFileURL} from 'node:url';
import {resolve} from 'node:path';
import {AVOIDS} from '../../packages/i18n/scripts/lint-glossary.mjs';
import {nextMessages} from '../../packages/i18n/src/catalog/en-US/next/index.ts';
export function lintNextGlossary(messages) {
  const rules = [...AVOIDS, [/Portico Cloud/, 'Portico Services']];
  return Object.entries(messages).flatMap(([key, value]) => rules.filter(([pattern]) => pattern.test(value)).map(([, expected]) => `${key}: use ${expected}`));
}
if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const failures = lintNextGlossary(nextMessages);
  if (failures.length) { console.error(failures.join('\n')); process.exit(1); }
  console.log('next glossary clean');
}
