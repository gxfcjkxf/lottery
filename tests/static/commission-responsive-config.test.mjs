import {test} from 'node:test';import assert from 'node:assert/strict';import {readFileSync} from 'node:fs';
test('shared configured order UI is checked in both languages and real device presets',()=>{
 const config=readFileSync('playwright.layout.config.ts','utf8'),spec=readFileSync('tests/layout/simplified-betting-points.spec.ts','utf8');for(const name of ['Pixel 7','iPhone 13','iPhone 13 landscape'])assert.ok(config.includes(name));assert.match(spec,/surface of \['admin','platform'\]/);assert.match(spec,/locale of \['en','zh-CN'\]/);assert.match(spec,/expectNoPageOverflow/);assert.match(spec,/\[320,480,768,1280\]/);
});
