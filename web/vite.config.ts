import {defineConfig, type Plugin} from 'vite';
import react from '@vitejs/plugin-react';
import {createReadStream, readdirSync, readFileSync, statSync} from 'node:fs';
import {fileURLToPath} from 'node:url';
import {enUS} from '../packages/i18n/src/catalog/en-US.ts';
const at = (p: string) => fileURLToPath(new URL(p, import.meta.url));

/*
 * Third-party files the subtitle renderer fetches by name at run time: the libass WebAssembly
 * worker (which looks for its `.wasm` beside itself) and the subtitle fonts. They are served
 * exactly as vendored in `third_party/`, under fixed names, never bundled or renamed. The folder
 * names carry a version so a new build is a new address. `web/src/player/subtitles/vendor.ts`
 * names the same paths.
 */
const vendored: Record<string, string> = {
  'vendor/libass-wasm-4.2.4/': at('../third_party/libass-wasm/files/'),
  'vendor/subtitle-fonts-1/': at('../third_party/subtitle-fonts/fonts/'),
};
/* The licences travel with the files: a build carries each folder's notices beside what they cover. */
const vendoredNotices: Record<string, string[]> = {
  'vendor/libass-wasm-4.2.4/': ['LICENSE', 'COPYRIGHT', 'OFL-1.1.txt'].map(name => at('../third_party/libass-wasm/' + name)),
  'vendor/subtitle-fonts-1/': ['OFL-1.1.txt', 'Apache-2.0.txt'].map(name => at('../third_party/subtitle-fonts/' + name)),
};
const vendoredTypes: Record<string, string> = {js: 'text/javascript', wasm: 'application/wasm', woff2: 'font/woff2', ttf: 'font/ttf'};
function vendoredFiles(): Plugin {
  return {
    name: 'portico-vendored-files',
    configureServer(server) {
      server.middlewares.use((request, response, next) => {
        const path = (request.url ?? '').split('?')[0]!;
        for (const [prefix, directory] of Object.entries(vendored)) {
          if (!path.startsWith('/' + prefix)) continue;
          const name = path.slice(prefix.length + 1);
          if (!/^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(name)) break;
          try {
            const size = statSync(directory + name).size;
            response.setHeader('Content-Type', vendoredTypes[name.split('.').pop() ?? ''] ?? 'application/octet-stream');
            response.setHeader('Content-Length', String(size));
            createReadStream(directory + name).pipe(response);
            return;
          } catch { break; }
        }
        next();
      });
    },
    generateBundle() {
      for (const [prefix, directory] of Object.entries(vendored))
        for (const name of readdirSync(directory)) this.emitFile({type: 'asset', fileName: prefix + name, source: readFileSync(directory + name)});
      for (const [prefix, files] of Object.entries(vendoredNotices))
        for (const file of files) this.emitFile({type: 'asset', fileName: prefix + file.split(/[\\/]/).pop(), source: readFileSync(file)});
    },
  };
}

/*
 * The browser floor, stated in README.md. The build writes its syntax for these engines, and
 * `browser-check.js` (served as `/browser-check.js`, before the app) tells an older one plainly
 * that it cannot run Portico rather than leaving the page empty.
 */
const browserFloor = ['chrome111', 'edge111', 'firefox113', 'safari16.4', 'ios16.4'];
function browserCheck(): Plugin {
  const source = () => readFileSync(at('./browser-check.js'), 'utf8')
    .replace('__PORTICO_UNSUPPORTED_TITLE__', JSON.stringify(enUS['web.browser.unsupportedTitle']))
    .replace('__PORTICO_UNSUPPORTED_BODY__', JSON.stringify(enUS['web.browser.unsupportedBody']));
  return {
    name: 'portico-browser-check',
    configureServer(server) {
      server.middlewares.use((request, response, next) => {
        if ((request.url ?? '').split('?')[0] !== '/browser-check.js') return next();
        response.setHeader('Content-Type', 'text/javascript');
        response.end(source());
      });
    },
    generateBundle() {
      this.emitFile({type: 'asset', fileName: 'browser-check.js', source: source()});
    },
  };
}

export default defineConfig({
  plugins: [react(), vendoredFiles(), browserCheck()],
  resolve: {
    alias: {'@core': at('../packages/client-core/src'), '@design': at('../packages/design/src/index.ts'), '@i18n': at('../packages/i18n/src/index.ts'), '@': at('./src')},
    dedupe: ['react', 'react-dom'],
  },
  server: {
    strictPort: true,
    fs: {allow: ['../..']},
    proxy: {
      '/v1': {target: process.env.PORTICO_DEV_SERVER_URL ?? 'http://127.0.0.1:32500', changeOrigin: false},
      '/v2': {target: process.env.PORTICO_DEV_SERVER_URL ?? 'http://127.0.0.1:32500', changeOrigin: false},
    },
  },
  build: {sourcemap: true, target: browserFloor, cssTarget: browserFloor},
});
