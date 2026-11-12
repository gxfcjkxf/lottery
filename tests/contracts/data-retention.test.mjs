import test from 'node:test';
import assert from 'node:assert/strict';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import {schemas,operations} from '../../scripts/openapi-data-retention.mjs';
const ajv=new Ajv2020({strict:false});addFormats(ajv);
ajv.addSchema({$id:'urn:lottery:retention',components:{schemas:{...schemas,UUID:{type:'string',format:'uuid'},DateTime:{type:'string',format:'date-time'}}}});
const validate=name=>ajv.compile({$ref:`urn:lottery:retention#/components/schemas/${name}`});
test('monthly retention has one schedule and no arbitrary cleanup or export operation',()=>{
 assert.equal(operations.length,4);assert.ok(operations.every(o=>o.path.startsWith('/api/v1/platform/data-retention-')));
 assert.ok(operations.filter(o=>o.method!=='GET').every(o=>o.idempotency&&o.parameters.some(p=>p.name==='X-Data-Retention-Actor-ID')));
 assert.ok(operations.every(o=>!o.path.includes('download')&&!o.path.includes('export')));
});
test('schedule requires all fields, exact wall clock and monthly execution day 1–28',()=>{
 const check=validate('DataRetentionInput');const input={version:1,enabled:true,execution_day:10,execution_time:'02:00'};
 assert.ok(check(input));for(const change of [{execution_day:0},{execution_day:29},{execution_day:1.5},{execution_time:'2:00'},{execution_time:'24:00'},{daily_enabled:true},{cutoff_at:'2026-10-01T00:00:00Z'}])assert.equal(check({...input,...change}),false);
});
test('counts remain exact strings and the platform audit scope is explicit',()=>{
 const counts=validate('DataRetentionCounts');assert.ok(counts({deleted:'9999999999999999999',receipts:'1',deferred:'0'}));assert.equal(counts({deleted:1,receipts:'1',deferred:'0'}),false);
});
