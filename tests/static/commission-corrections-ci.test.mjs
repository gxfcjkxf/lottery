import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
test('retired settlement correction fixture is absent from CI',()=>{
 const workflow=readFileSync(new URL('../../.github/workflows/ci.yaml',import.meta.url),'utf8');
 assert.doesNotMatch(workflow,/commission-corrections-browser:|commission-fixture|qualification-fixture/);
 assert.match(workflow,/TestCommission/);
});
