/** Isolated proof input. Settings remains the writer of the actual shared main/Vite selection. */
import {fileURLToPath} from 'node:url';
import path from 'node:path';
import type {Plugin} from 'vite';
import base from '../vite.config';

const scratch = process.env.TMPDIR;
if (!scratch || !path.isAbsolute(scratch)) throw new Error('TMPDIR must name the owned build scratch.');
const selectEntry: Plugin = {
  name: 'services-entry-proof-only',
  transformIndexHtml: {order: 'pre', handler: html => html.replace('/src/main.tsx', '/src/services/main.tsx')},
};
export default {
  ...base,
  root: fileURLToPath(new URL('../', import.meta.url)),
  cacheDir: path.join(scratch, 'services-vite-cache'),
  plugins: [...(base.plugins ?? []), selectEntry],
  build: {...base.build, outDir: path.join(scratch, 'services-dist'), emptyOutDir: true, manifest: true},
};
