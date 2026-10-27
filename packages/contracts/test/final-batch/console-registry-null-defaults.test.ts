import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {join} from 'node:path';
import {decodeGetConsoleRegistryResponse} from '../../src/decoders.ts';
import {schemas} from '../../src/schemas.ts';

// Final batch supplies the fresh directory populated by the real Go owner
// producer before this explicit decoder check runs.
const directory = process.env.PORTICO_CONSOLE_REGISTRY_FIXTURE_DIR;
assert.ok(directory, 'record the real console registry response before running this final-batch test');
const fixture = JSON.parse(readFileSync(join(directory, 'get-console-registry.json'), 'utf8'));

test('console registry decoder keeps required defaults and accepts declared nulls', () => {
 const decoded = decodeGetConsoleRegistryResponse(fixture.response) as any;
 const fields = decoded.data.fields as any[];
 const byID = new Map(fields.map(field => [field.id, field]));
 for (const id of ['perAccountCap', 'serverCap']) {
  assert.equal(byID.get(id)?.default, null, `${id} default`);
 }
 assert.equal((byID.get('hdrToneMapping') as any).default, true);
 assert.equal((byID.get('analysisStorageGiB') as any).default, 32);
 assert.equal((byID.get('name') as any).default, 'Portico');
 assert.ok(Array.isArray((byID.get('lanNetworks') as any).default) && (byID.get('lanNetworks') as any).default.length > 0);
 assert.deepEqual((byID.get('accessUrls') as any).default, []);

 // Only default became nullable; every other member stays a closed required
 // field with its original default shape.
 const fieldSchema = ((schemas.GetConsoleRegistryResponse as any).properties.data.properties.fields.items) as any;
 assert.equal(fieldSchema.properties.default.type, 'any');
 assert.equal(fieldSchema.properties.default.nullable, true);
 assert.ok(fieldSchema.required.includes('default'));
 assert.equal(fieldSchema.properties.id.nullable, undefined);
 assert.equal(fieldSchema.additionalProperties, false);

 const missing = structuredClone(fixture.response);
 delete missing.data.fields[0].id;
 assert.throws(() => decodeGetConsoleRegistryResponse(missing));

 const malformed = structuredClone(fixture.response);
 malformed.data = null;
 assert.throws(() => decodeGetConsoleRegistryResponse(malformed));
});
