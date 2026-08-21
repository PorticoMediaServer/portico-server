import { createHash } from 'node:crypto';
import { mkdir, readdir, readFile, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';

const mode = process.argv[2];
if (mode !== 'bundled' && mode !== 'hosted') {
  throw new Error('Runtime config mode must be "bundled" or "hosted".');
}

const dist = resolve(import.meta.dirname, '..', 'dist');
await mkdir(dist, { recursive: true });
async function hashTree(directory, relative = '') {
  const hash = createHash('sha256');
  const entries = await readdir(resolve(directory, relative), { withFileTypes: true });
  // Use bytewise code-point ordering so deployment verification is independent
  // of the build machine's ICU locale.
  for (const entry of entries.sort((a, b) => a.name < b.name ? -1 : a.name > b.name ? 1 : 0)) {
    const child = relative ? `${relative}/${entry.name}` : entry.name;
    if (child === 'portico-config.js' || child === 'portico-build.json') continue;
    hash.update(child);
    hash.update(entry.isDirectory() ? await hashTree(directory, child) : await readFile(resolve(directory, child)));
  }
  return hash.digest('hex');
}
const buildId = process.env.PORTICO_WEB_BUILD_ID || `sha256-${(await hashTree(dist)).slice(0, 24)}`;
const config = {
  mode,
  hostedApiBaseUrl: 'https://api.getportico.tv',
  routeProbeTimeoutMs: 3500,
  buildId,
};
await writeFile(
  resolve(dist, 'portico-config.js'),
  `window.__PORTICO_CONFIG__ = ${JSON.stringify(config)};\n`,
  'utf8',
);
await writeFile(resolve(dist, 'portico-build.json'), `${JSON.stringify({ buildId, mode, hostedApiBaseUrl: config.hostedApiBaseUrl })}\n`, 'utf8');

console.log(`Wrote explicit ${mode} runtime config for ${buildId}.`);
