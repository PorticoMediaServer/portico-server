import { readdir, readFile, stat } from 'node:fs/promises';
import { resolve } from 'node:path';

const root = resolve(import.meta.dirname, '..');
const dist = resolve(root, 'dist');

const forbiddenMarkers = [
  'EhlerFlix Test',
  'Portico Review',
  'review@portico.local',
  'fixture-owner',
  'fixture-tv',
  'fixture-movies',
  'fixture-music',
  'Fargo',
  'The Rookie',
  'Blade Runner 2049',
  'Bonobo',
  'image.tmdb.org',
  'images.unsplash.com',
  'FixtureFilesystemSource',
  'FixturePorticoDataSource',
  'Fixture recording mutations are unavailable.',
  'portico_fixture_token_shown_once',
];

async function filesBelow(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const nested = await Promise.all(entries.map(async (entry) => {
    const path = resolve(directory, entry.name);
    return entry.isDirectory() ? filesBelow(path) : [path];
  }));
  return nested.flat();
}

try {
  const details = await stat(dist);
  if (!details.isDirectory()) throw new Error('dist is not a directory');
} catch {
  throw new Error('Production bundle is missing. Run `npm run build` before `npm run verify:bundle`.');
}

const bundleFiles = (await filesBelow(dist)).filter((path) => /\.(?:css|html|js|json|map|svg|txt)$/i.test(path));
const findings = [];

const expectedRuntimeMode = String(process.env.PORTICO_EXPECT_RUNTIME_MODE || '').trim();
if (expectedRuntimeMode) {
  const runtimeConfig = await readFile(resolve(dist, 'portico-config.js'), 'utf8');
	const buildManifest = JSON.parse(await readFile(resolve(dist, 'portico-build.json'), 'utf8'));
  if (!runtimeConfig.includes(`"mode":"${expectedRuntimeMode}"`)) {
    findings.push(`portico-config.js does not declare the expected ${expectedRuntimeMode} runtime`);
  }
	if (!/^sha256-[a-f0-9]{24,64}$/.test(buildManifest.buildId || '') || !runtimeConfig.includes(`"buildId":"${buildManifest.buildId}"`)) {
		findings.push('runtime config and immutable build manifest do not share a valid content-hashed build ID');
	}
	const expectedHostedAPIBaseURL = expectedRuntimeMode === 'hosted' ? 'https://web.getportico.tv' : 'https://api.getportico.tv';
	if (buildManifest.mode !== expectedRuntimeMode || buildManifest.hostedApiBaseUrl !== expectedHostedAPIBaseURL) {
		findings.push('build manifest runtime authority does not match the hosted deployment contract');
	}
	const assetScope = JSON.parse(await readFile(resolve(dist, 'portico-asset-scope.json'), 'utf8'));
	const expectedPrefix = `/assets/${buildManifest.buildId}/`;
	if (assetScope.buildId !== buildManifest.buildId || assetScope.assetPrefix !== expectedPrefix) {
		findings.push('asset scope and runtime build identity do not match');
	}
	for (const path of bundleFiles) {
		const contents = await readFile(path, 'utf8');
		for (const match of contents.matchAll(/(?:\/|["'])assets\/[^\s"')]+/g)) {
			const reference = match[0].replace(/^["']/, '/');
			if (!reference.startsWith(expectedPrefix)) findings.push(`${path.slice(dist.length + 1)} contains unscoped or cross-build asset reference ${JSON.stringify(match[0])}`);
		}
	}
	const scopedDirectory = resolve(dist, 'assets', buildManifest.buildId);
	try {
		if (!(await stat(scopedDirectory)).isDirectory()) findings.push('scoped asset directory is missing');
		const assetRoots = await readdir(resolve(dist, 'assets'));
		if (assetRoots.length !== 1 || assetRoots[0] !== buildManifest.buildId) findings.push('dist/assets contains an asset graph outside the current build scope');
	} catch {
		findings.push('scoped asset directory is missing');
	}
}

for (const path of bundleFiles) {
  const contents = await readFile(path, 'utf8');
  for (const marker of forbiddenMarkers) {
    if (contents.includes(marker)) findings.push(`${path.slice(dist.length + 1)} contains ${JSON.stringify(marker)}`);
  }
}

if (findings.length > 0) {
  console.error('Production web bundle contains fixture-only data:');
  for (const finding of findings) console.error(`- ${finding}`);
  process.exitCode = 1;
} else {
  console.log(`Production bundle is free of ${forbiddenMarkers.length} fixture-only markers.`);
}
