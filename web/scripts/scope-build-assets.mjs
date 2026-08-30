import { createHash } from 'node:crypto';
import { mkdir, readdir, readFile, rename, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';

const dist = resolve(import.meta.dirname, '..', 'dist');
const assets = resolve(dist, 'assets');

async function hashTree(directory, relative = '') {
  const hash = createHash('sha256');
  const entries = await readdir(resolve(directory, relative), { withFileTypes: true });
  for (const entry of entries.sort((a, b) => a.name < b.name ? -1 : a.name > b.name ? 1 : 0)) {
    const child = relative ? `${relative}/${entry.name}` : entry.name;
    if (child === 'portico-config.js' || child === 'portico-build.json' || child === 'portico-asset-scope.json') continue;
    hash.update(child);
    hash.update(entry.isDirectory() ? await hashTree(directory, child) : await readFile(resolve(directory, child)));
  }
  return hash.digest('hex');
}

async function filesBelow(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  return (await Promise.all(entries.map(async (entry) => {
    const path = resolve(directory, entry.name);
    return entry.isDirectory() ? filesBelow(path) : [path];
  }))).flat();
}

const buildId = `sha256-${(await hashTree(dist)).slice(0, 24)}`;
const scopedAssets = resolve(assets, buildId);
const temporaryAssets = resolve(dist, `.assets-${buildId}`);
await rename(assets, temporaryAssets);
await mkdir(assets, { recursive: true });
await rename(temporaryAssets, scopedAssets);

const rewriteFiles = [resolve(dist, 'index.html'), resolve(dist, 'portico-service-worker.js'),
  ...(await filesBelow(scopedAssets)).filter((path) => /\.(?:css|html|js|json|map|svg)$/i.test(path))];
for (const path of rewriteFiles) {
  let contents = await readFile(path, 'utf8');
  contents = contents.replaceAll('/assets/', `/assets/${buildId}/`);
  contents = contents.replaceAll('"assets/', `"assets/${buildId}/`);
  contents = contents.replaceAll("'assets/", `'assets/${buildId}/`);
  await writeFile(path, contents, 'utf8');
}
await writeFile(resolve(dist, 'portico-asset-scope.json'), `${JSON.stringify({ buildId, assetPrefix: `/assets/${buildId}/` })}\n`, 'utf8');
console.log(`Scoped the complete emitted asset graph under /assets/${buildId}/.`);
