#!/usr/bin/env node

import { readFile, readdir, stat } from 'node:fs/promises';
import { basename, join } from 'node:path';
import process from 'node:process';
import { gzipSync } from 'node:zlib';

const distDirectory = join(process.cwd(), 'dist');
const indexHtml = await readFile(join(distDirectory, 'index.html'), 'utf8');
const buildManifest = JSON.parse(await readFile(join(distDirectory, 'portico-build.json'), 'utf8'));
const assetsDirectory = join(distDirectory, 'assets', buildManifest.buildId);
const assetNames = await readdir(assetsDirectory);

const scriptSources = [...indexHtml.matchAll(/<script[^>]+>/g)]
  .map((match) => match[0])
  .filter((tag) => /\btype="module"/.test(tag))
  .map((tag) => tag.match(/\bsrc="([^"]+)"/)?.[1])
  .filter(Boolean);
const stylesheetSources = [...indexHtml.matchAll(/<link[^>]+rel="stylesheet"[^>]+href="([^"]+)"/g)].map((match) => match[1]);

if (scriptSources.length !== 1) {
  throw new Error(`Expected one initial JavaScript entry point, found ${scriptSources.length}.`);
}

const initialScriptName = basename(scriptSources[0]);
const initialScriptBytes = await sizeOf(initialScriptName);
const initialScriptGzipBytes = gzipSync(await readFile(join(assetsDirectory, initialScriptName))).byteLength;
const initialStylesheetNames = stylesheetSources.map((source) => basename(source));
const initialStylesheetBytes = await totalSize(initialStylesheetNames);
const hlsChunks = assetNames.filter((name) => /^hls-.*\.js$/.test(name));

if (hlsChunks.length !== 1) {
  throw new Error(`Expected one lazy HLS playback chunk, found ${hlsChunks.length}.`);
}
if (scriptSources.some((source) => basename(source) === hlsChunks[0])) {
  throw new Error('The HLS runtime must remain lazy and cannot be part of the initial document scripts.');
}

const hlsBytes = await sizeOf(hlsChunks[0]);
const budgets = {
  // Viewer-generation fencing, resilient route recovery, and the shared
  // product-language catalogue are intentionally available before the first
  // authenticated surface mounts. Bound both source bytes and transfer bytes:
  // the latter is the user-facing cost, while the former still guards parse work.
  initialJavaScript: 480 * 1024,
  initialJavaScriptGzip: 125 * 1024,
  // The production shell includes the always-available player dock and shared
  // overlay/control system. Keep the uncompressed ceiling tight while allowing
  // the complete V1 interaction grammar; the current bundle is about 29 KiB gzip.
  initialStylesheets: 180 * 1024,
  lazyHls: 600 * 1024,
};

reportAdvisory('Initial JavaScript', initialScriptBytes, budgets.initialJavaScript);
reportAdvisory('Initial JavaScript (gzip)', initialScriptGzipBytes, budgets.initialJavaScriptGzip);
assertWithinBudget('Initial stylesheets', initialStylesheetBytes, budgets.initialStylesheets);
assertWithinBudget('Lazy HLS runtime', hlsBytes, budgets.lazyHls);

console.log(JSON.stringify({
  initialJavaScript: {
    file: initialScriptName,
    bytes: initialScriptBytes,
    budget: budgets.initialJavaScript,
    gzipBytes: initialScriptGzipBytes,
    gzipBudget: budgets.initialJavaScriptGzip,
  },
  initialStylesheets: { files: initialStylesheetNames, bytes: initialStylesheetBytes, budget: budgets.initialStylesheets },
  lazyHls: { file: hlsChunks[0], bytes: hlsBytes, budget: budgets.lazyHls },
}, null, 2));

async function sizeOf(name) {
  return (await stat(join(assetsDirectory, name))).size;
}

async function totalSize(names) {
  const sizes = await Promise.all(names.map(sizeOf));
  return sizes.reduce((total, size) => total + size, 0);
}

function assertWithinBudget(label, actual, budget) {
  if (actual > budget) {
    throw new Error(`${label} is ${actual} bytes, over the ${budget}-byte release budget.`);
  }
}

function reportAdvisory(label, actual, budget) {
  if (actual > budget) {
    console.warn(`${label} is ${actual} bytes, over the ${budget}-byte advisory target.`);
  }
}
