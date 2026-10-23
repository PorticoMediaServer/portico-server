import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {readFileSync, readdirSync, statSync} from 'node:fs';
import path from 'node:path';

const root = path.join(process.env.TMPDIR ?? '', 'services-dist');
assert(process.env.TMPDIR && path.isAbsolute(root), 'Use the owned remote job scratch.');
const index = readFileSync(path.join(root, 'index.html'), 'utf8');
assert(index.includes('/browser-check.js'), 'The classic browser guard must precede the module.');
assert(index.indexOf('/browser-check.js') < index.indexOf('type="module"'), 'The browser guard must run before module loading.');
assert(!index.includes('/src/main.tsx') && !index.includes('/src/services/main.tsx'), 'The output must contain the built entry.');
assert(index.includes('/assets/'), 'The root index must reference emitted assets.');
const files = [];
function walk(directory) {
  for (const name of readdirSync(directory).sort()) {
    const filename = path.join(directory, name), stat = statSync(filename);
    if (stat.isDirectory()) walk(filename);
    else files.push({path: path.relative(root, filename), size: stat.size, sha256: createHash('sha256').update(readFileSync(filename)).digest('hex')});
  }
}
walk(root);
assert(files.some(file => file.path === 'browser-check.js'));
assert(files.some(file => /^assets\/.*\.js$/.test(file.path)));
console.log(JSON.stringify({artifact: 'Isolated explicit Services entry build; metadata only, no actual private cmd/web, API/session or whole UI acceptance.', files}));
