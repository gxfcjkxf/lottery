import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync,readdirSync} from 'node:fs'
import {join} from 'node:path'
function pages(root){return readdirSync(root,{withFileTypes:true}).flatMap(entry=>entry.isDirectory()?pages(join(root,entry.name)):entry.name.endsWith('.vue')?[join(root,entry.name)]:[])}
test('phase one pages offer no business data export or file downloads',()=>{
 for(const root of ['admin-web/src','platform-web/src','agent-web/src','user-web/src','shared/src'])for(const file of pages(root)){
  // Registration QR saving is an image share action.
  const source=readFileSync(file,'utf8')
  assert.doesNotMatch(source,/\.download\s*=/,file)
  assert.doesNotMatch(source,/@click="[^"\n]*(?:exportAll|exportCsv|exportReport|exportDraft|reviewExport|confirmExport|withGlobalLoading\(download\))/,file)
 }
})
