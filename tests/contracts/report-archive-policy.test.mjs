import test from 'node:test';
import assert from 'node:assert/strict';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import {schemas,operations} from '../../scripts/openapi-report-archive-policy.mjs';
const schedule={version:1,daily_enabled:true,monthly_enabled:true,daily_execution_time:'07:30',daily_period_offset:2,monthly_execution_day:5,monthly_execution_time:'06:00',monthly_period_offset:2};
const ajv=new Ajv2020({strict:false});addFormats(ajv);ajv.addSchema({$id:'urn:lottery:global-archives',components:{schemas:{...schemas,DateTime:{type:'string',format:'date-time'},UUID:{type:'string',format:'uuid'}}}});
const validate=name=>ajv.compile({$ref:`urn:lottery:global-archives#/components/schemas/${name}`});
test('archive configuration is a global platform read/write endpoint without brand scope',()=>{
 assert.deepEqual(operations.map(o=>`${o.method} ${o.path}`),['GET /api/v1/platform/report-archive-policy','PUT /api/v1/platform/report-archive-policy']);
 assert.deepEqual(operations[0].permissions,['platform_settings.view.platform']);assert.deepEqual(operations[1].permissions,['platform_settings.write.platform']);assert.equal(operations[1].idempotency,true);
 assert.ok(operations.every(o=>!o.brandHeader));assert.match(operations[1].description,/every brand atomically/);
});
test('global archive input remains closed and enforces schedule bounds',()=>{
 const check=validate('ReportArchivePolicyUpdateInput');assert.ok(check(schedule),JSON.stringify(check.errors));assert.ok(check({...schedule,reason:''}));
 for(const value of [{...schedule,brand_id:'11111111-1111-4111-8111-111111111111'},{...schedule,monthly_execution_day:32},{...schedule,monthly_period_offset:0},{...schedule,daily_execution_time:'24:00'},{...schedule,reason:' padded '},{...schedule,version:0}])assert.equal(check(value),false);
});
test('global response contains one schedule and next executions without a brand identity',()=>{
 const check=validate('PlatformReportArchivePolicy');const result={...schedule,timezone:'Asia/Singapore',next_runs:{daily:null,monthly:{execute_at:'2026-11-04T22:00:00Z',period_key:'2026-09'}},updated_at:'2026-10-27T00:00:00Z'};
 assert.ok(check(result),JSON.stringify(check.errors));assert.equal(check({...result,brand_id:'11111111-1111-4111-8111-111111111111'}),false);
});
