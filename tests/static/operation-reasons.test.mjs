import test from 'node:test';
import assert from 'node:assert/strict';
import { readdir, readFile } from 'node:fs/promises';

test('manual operation reason inputs stay removed from all four frontend source trees', async () => {
  for (const root of ['admin-web/src','platform-web/src','shared/src','user-web/src']) {
    for (const entry of await readdir(root, { withFileTypes:true })) {
      if (!entry.isFile() || !entry.name.endsWith('.vue')) continue;
      const source = await readFile(`${root}/${entry.name}`,'utf8');
      assert.doesNotMatch(source, /v-model(?:\.\w+)*\s*=\s*["'][^"']*reason[^"']*["']/i, `${root}/${entry.name} reintroduced an operation reason control`);
    }
  }
});

test('write contracts make audit notes optional without relaxing business fields', async () => {
  const doc = JSON.parse(await readFile('docs/openapi.json','utf8'));
  function resolve(schema) { return schema?.$ref ? doc.components.schemas[schema.$ref.split('/').at(-1)] : schema; }
  function inspect(schema, seen = new Set()) {
    if (!schema || typeof schema !== 'object') return;
    if (schema.$ref) {
      if (seen.has(schema.$ref)) return;
      seen.add(schema.$ref); return inspect(resolve(schema), seen);
    }
    if (Array.isArray(schema.required)) assert.ok(!schema.required.includes('reason'), 'a write contract requires a manual reason');
    for (const value of Object.values(schema)) {
      if (Array.isArray(value)) value.forEach(item => inspect(item, seen)); else if (value && typeof value === 'object') inspect(value, seen);
    }
  }
  for (const item of Object.values(doc.paths)) for (const operation of Object.values(item)) if (operation?.requestBody) inspect(operation.requestBody);
  for (const [path, method, required] of [
    ['/api/v1/admin/point-policy','put',['version','max_balance_points','max_recharge_points','max_adjustment_points']],
    ['/api/v1/platform/brand-operation','patch',['version','status']],
    ['/api/v1/admin/platform-games/{id}/configured/sales','post',['version','rule_id','enabled','prices','min_points','max_points','max_period_points','max_user_period_points']],
  ]) {
    const schema = resolve(doc.paths[path][method].requestBody.content['application/json'].schema);
    for (const field of required) assert.ok(schema.required.includes(field), `${path} lost required ${field}`);
  }
  assert.equal(doc.components.schemas.Reason.minLength, undefined);
});
