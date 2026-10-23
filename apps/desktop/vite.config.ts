import {mergeConfig} from 'vite';
import {fileURLToPath} from 'node:url';
import web from '../../web-next/vite.config.ts';

const root = fileURLToPath(new URL('../../web-next', import.meta.url));
const entry = fileURLToPath(new URL('./src/main.ts', import.meta.url));
// Reuse the original HTML, React entry, router, CSS, controls and responsive rules. Only the
// entry installs a different MediaPlayer before importing the existing app.
export default mergeConfig(web, {
  root,
  plugins: [{name: 'portico-native-entry', transformIndexHtml: {order: 'pre', handler: (html: string) => html.replace('src="/src/main.tsx"', `src="/@fs/${entry}"`)}}],
  server: {host: '127.0.0.1', port: 14229, strictPort: true, fs: {allow: [fileURLToPath(new URL('../..', import.meta.url))]}},
  build: {outDir: process.env.PORTICO_DESKTOP_WEB_DIST ?? fileURLToPath(new URL('./dist', import.meta.url)), emptyOutDir: true},
});
