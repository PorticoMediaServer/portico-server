import {readdirSync} from 'node:fs';
import {join} from 'node:path';
import {spawnSync} from 'node:child_process';

function tests(directory) {
  return readdirSync(directory, {withFileTypes: true}).sort((a, b) => a.name.localeCompare(b.name)).flatMap(entry => {
    const path = join(directory, entry.name);
    return entry.isDirectory() ? tests(path) : /\.test\.(?:tsx?|[cm]?js)$/.test(entry.name) ? [path] : [];
  });
}

const result = spawnSync(process.execPath, ['--experimental-strip-types', '--test', ...tests('test')], {stdio: 'inherit'});
if (result.error) throw result.error;
process.exit(result.status ?? 1);
