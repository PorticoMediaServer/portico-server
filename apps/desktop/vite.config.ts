import {defineConfig,mergeConfig} from 'vite';
import {fileURLToPath} from 'node:url';
import web from '../../web-next/vite.config.ts';

const root = fileURLToPath(new URL('../../web-next', import.meta.url));
const entry = fileURLToPath(new URL('./src/main.ts', import.meta.url));
// Reuse the original HTML, React entry, router, CSS, controls and responsive rules. Only the
// entry installs a different MediaPlayer before importing the existing app.
export default defineConfig(async environment=>mergeConfig(await (typeof web==='function'?web(environment):web), {
  root,
  plugins: [{name: 'portico-native-entry', transformIndexHtml: {order: 'pre', handler: (html: string) => {
    const marker='src="/src/main.tsx"';
    if(html.split(marker).length!==2)throw new Error('desktop_entry_missing');
    return html.replace(marker,`src="/@fs/${entry}"`);
  }}}],
  server: {host: '127.0.0.1', port: 14229, strictPort: true, fs: {allow: [fileURLToPath(new URL('../..', import.meta.url))]}},
  build: {outDir: process.env.PORTICO_DESKTOP_WEB_DIST ?? fileURLToPath(new URL('./dist', import.meta.url)), emptyOutDir: true},
}));
