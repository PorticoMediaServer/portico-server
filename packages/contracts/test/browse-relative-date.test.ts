import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {schemas} from '../src/schemas.ts';

/** The browse node is published inline wherever a query is accepted; the saved
 * resource definition is the stable one to inspect. */
const node = () => {
  const definition = (schemas.PostSavedResourcesRequest as {properties: {definition: {properties: {query: {properties: Record<string, {description?: string}>}}}}}}).properties.definition;
  return definition.properties.query;
};

test('the published browse-node contract declares the relative date operator and its value', () => {
  const browse = node();
  assert.match(browse.properties.operator?.description ?? '', /within-last-days/);
  assert.match(browse.properties.value?.description ?? '', /1 to 36500/);
});

test('the OpenAPI document carries the same declaration', () => {
  const openapi = JSON.parse(readFileSync(new URL('../../../server/api/portico.openapi.json', import.meta.url), 'utf8'));
  const definition = openapi.components.schemas.PostSavedResourcesRequest.properties.definition;
  const browse = definition.properties.query.anyOf[0];
  assert.match(browse.properties.operator.description, /within-last-days/);
  assert.match(browse.properties.value.description, /1 to 36500/);
});
