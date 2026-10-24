import {readFileSync, readdirSync, existsSync} from 'node:fs';
import {fileURLToPath} from 'node:url';
import {join, relative} from 'node:path';
const at = path => fileURLToPath(new URL(path, import.meta.url));
export const browserFloor = ['chrome111', 'edge111', 'firefox113', 'safari16.4', 'ios16.4'];
export const aliases = {'@core': at('../../packages/client-core/src'), '@design': at('../../packages/design/src/index.ts'), '@i18n': at('../../packages/i18n/src/index.ts'), '@': at('../src')};
export const assetNames = {entryFileNames: 'assets/[name]-[hash:8].js', chunkFileNames: 'assets/[name]-[hash:8].js', assetFileNames: 'assets/[name]-[hash:8][extname]'};
export function browserCheck(messages) {
  const source = () => readFileSync(at('./browser-check.js'), 'utf8')
    .replace('__PORTICO_UNSUPPORTED_TITLE__', JSON.stringify(messages['web.browser.unsupportedTitle']))
    .replace('__PORTICO_UNSUPPORTED_BODY__', JSON.stringify(messages['web.browser.unsupportedBody']));
  return {
    name: 'portico-browser-check',
    transformIndexHtml: html => html.replace('__PORTICO_APP_NAME__', messages['next.platform.appName']),
    configureServer(server) {
      server.middlewares.use((request, response, next) => {
        if ((request.url ?? '').split('?')[0] !== '/browser-check.js') return next();
        response.setHeader('Content-Type', 'text/javascript');
        response.end(source());
      });
    },
    generateBundle() { this.emitFile({type: 'asset', fileName: 'browser-check.js', source: source()}); },
  };
}
export function familyImports(root) {
  const directory = join(root, 'src/dev/families');
  const names = existsSync(directory) ? readdirSync(directory).filter(name => /^[a-z][a-z0-9-]*\.tsx$/.test(name)).sort() : [];
  const families = names.map(name => [name.slice(0, -4), '/src/dev/families/' + name]);
  // Keep unselected families outside Vite's eager dependency crawl. Their genuine
  // module is requested only when the registry calls this public loader.
  return `const families=${JSON.stringify(families)};const load=url=>import(/* @vite-ignore */ url);export default families.map(([familyId,url])=>Object.assign(()=>load(url),{familyId}));`;
}
export function kitFamilies() {
  const id = 'virtual:portico-kit-families';
  let root, development;
  return {
    name: 'portico-kit-families',
    configResolved(config) { root = config.root; development = config.command === 'serve'; },
    resolveId(source) { if (source === id) return '\0' + id; },
    load(source) { if (source === '\0' + id) return development ? familyImports(root) : 'export default [];'; },
    configureServer(server) {
      const directory = join(root, 'src/dev/families');
      server.watcher.add(directory);
      const changed = path => {
        if (!relative(directory, path).startsWith('..') && path.endsWith('.tsx')) {
          const module = server.moduleGraph.getModuleById('\0' + id);
          if (module) server.moduleGraph.invalidateModule(module);
          server.ws.send({type: 'full-reload'});
        }
      };
      server.watcher.on('add', changed).on('unlink', changed);
    },
  };
}
