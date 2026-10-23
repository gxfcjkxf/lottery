import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
const doc = JSON.parse(readFileSync(new URL('../../docs/openapi.json',import.meta.url),'utf8'));
const ajv = new Ajv2020({ strict: false });
addFormats(ajv); ajv.addFormat('int64',{type:'number',validate:Number.isInteger});
ajv.addSchema({$id:'urn:lottery:configured-preview',components:doc.components});
const validate = name => ajv.compile({$ref:`urn:lottery:configured-preview#/components/schemas/${name}`});

for (const name of ['mark_six_v2','canada_28','canada_996','canada_vip','canada_42','canada_web','canada_50','canada_baccarat','canada_niuniu']) test(`the complete ${name} template conforms to the configured preview contract`, () => {
  const catalog = JSON.parse(readFileSync(new URL(`../../backend/internal/rules/config/${name}.json`,import.meta.url),'utf8'));
  const check = validate('ConfiguredCatalog');
  assert.equal(check(catalog),true,JSON.stringify(check.errors));
  const input = validate('ConfiguredPreviewInput');
  const snapshot = {catalog,bindings:{version:'operator-attributes',attributes:{}},prices:{version:'operator-prices',plates:{}}};
  assert.equal(input({snapshot,requests:[{play_id:'new_group',options:['choice'],plate:'A',points:'9007199254740993',multiplier:'1'}]}),true,JSON.stringify(input.errors));
  // The engine, rather than JSON Schema, validates completeness of the binding and price tables.
  assert.equal(input({snapshot,requests:[],sale:true}),false);
  assert.equal(input({snapshot,requests:[{play_id:'new_group',options:['choice'],plate:'A',points:3,multiplier:'1'}]}),false);
});

test('preview APIs retain management scopes, a bounded explicit draft and no sale or idempotency semantics', () => {
  for (const [surface,scope] of [['admin','brand'],['platform','platform']]) {
    const op = doc.paths[`/api/v1/${surface}/platform-games/rule-preview`].post;
    assert.deepEqual(op['x-permissions'],[`platform_game.view.${scope}`]);
    assert.equal(op['x-idempotent-operation'],false);
    assert.equal(op.parameters.some(p => p.name==='Idempotency-Key'),false);
    assert.equal(op.parameters.some(p => p.name==='X-Brand-ID'),surface==='admin');
    assert.match(op.requestBody.description,/4 MiB/);
    assert.match(op.requestBody.description,/No persistence/);
  }
  assert.equal(doc.paths['/api/v1/platform-games/rule-preview'],undefined);
});

test('dedicated sales are absent while historical queries remain authenticated',()=>{
 for(const family of ['mark-six','canada']){
  for(const prefix of ['/api/v1','/api/v1/b/{brandCode}']){
   assert.equal(doc.paths[`${prefix}/${family}/games`],undefined);
   assert.equal(doc.paths[`${prefix}/${family}/games/{id}`],undefined);
   assert.equal(doc.paths[`${prefix}/${family}/preview`],undefined);
   assert.equal(doc.paths[`${prefix}/${family}/orders`].post,undefined);
   assert.ok(doc.paths[`${prefix}/${family}/orders`].get.security.length);
  }
  assert.equal(doc.paths[`/api/v1/platform/platform-games/{id}/${family}/rules`],undefined);
  assert.equal(doc.paths[`/api/v1/admin/platform-games/{id}/${family}/sales`],undefined);
  assert.ok(doc.paths[`/api/v1/admin/platform-games/{id}/${family}/orders`].get.security.length);
 }
 assert.ok(doc.paths['/api/v1/configured/orders'].get.security.length);
});
