import test from 'node:test';
import assert from 'node:assert/strict';
import {schemas} from '../src/schemas.ts';
import {canonicalQualityLadder,type CreateDownloadPreparationsRequest,type CreateDownloadRequestRequest} from '../src/types.ts';
import {decode} from '../src/runtime.ts';

test('quality defaults r25: actual item and container request schemas permit omission and keep the nine canonical values',()=>{
 const item:CreateDownloadPreparationsRequest={operationId:'op',mediaId:'item'};
 const container:CreateDownloadRequestRequest={operationId:'op',deviceId:'dev',target:{kind:'album',id:'album'},policy:{episodes:'all'}};
 for(const [name,body] of [['CreateDownloadPreparationsRequest',item],['CreateDownloadRequestRequest',container]] as const){
  const schema=schemas[name];
  assert.ok(!(schema.required as readonly string[]).includes('quality'));
  assert.equal(schema.properties.quality.default,'original');
  assert.deepEqual(schema.properties.quality.enum,canonicalQualityLadder.map(q=>q.id));
  assert.doesNotThrow(()=>decode(name,body));
 }
});
