import {fileURLToPath, pathToFileURL} from 'node:url';
import path from 'node:path';
import {existsSync} from 'node:fs';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const aliases = {'@core/': path.join(root, '..', 'packages', 'client-core', 'src') + '/', '@design/': path.join(root, '..', 'packages', 'design') + '/', '@/': path.join(root, 'src') + '/'};
// Vite resolves extensionless relative imports; Node's ESM resolver does not.
// Trying the same candidates a bundler would keeps app source unchanged.
const candidates = ['.ts', '.tsx', '/index.ts', '/index.tsx', '.mjs', '.js'];

export async function resolve(specifier, context, next) {
  if (specifier === '@design') return next(pathToFileURL(path.join(root, '..', 'packages', 'design', 'src', 'index.ts')).href, context);
  if (specifier === '@i18n') return next(pathToFileURL(path.join(root, '..', 'packages', 'i18n', 'src', 'index.ts')).href, context);
  for (const [prefix, target] of Object.entries(aliases)) {
    if (specifier.startsWith(prefix)) {
      const raw = target + specifier.slice(prefix.length);
      const found = [raw, ...candidates.map(suffix => raw + suffix)].find(existsSync);
      return next(pathToFileURL(found ?? raw).href, context);
    }
  }
  try {
    return await next(specifier, context);
  } catch (error) {
    if (error?.code !== 'ERR_MODULE_NOT_FOUND' || !specifier.startsWith('.')) throw error;
    for (const suffix of candidates) {
      try {
        return await next(specifier + suffix, context);
      } catch (retry) {
        if (retry?.code !== 'ERR_MODULE_NOT_FOUND') throw retry;
      }
    }
    throw error;
  }
}
