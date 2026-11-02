import test from 'node:test'
import assert from 'node:assert/strict'
import {readdirSync,readFileSync} from 'node:fs'
import {join} from 'node:path'
function vueFiles(dir){return readdirSync(dir,{withFileTypes:true}).flatMap(entry=>entry.isDirectory()?vueFiles(join(dir,entry.name)):entry.name.endsWith('.vue')?[join(dir,entry.name)]:[])}
test('business forms in all four apps use the shared form or filter contract',()=>{
 const missing=[]
 for(const dir of ['admin-web/src','platform-web/src','agent-web/src','user-web/src','shared/src'])for(const path of vueFiles(dir)){
  const source=readFileSync(path,'utf8')
  for(const match of source.matchAll(/<form\b((?:"[^"]*"|'[^']*'|[^'">])*)>/g)){
   const classes=/\bclass="([^"]*)"/.exec(match[1])?.[1].split(/\s+/)??[]
   if(classes.some(name=>['ui-form','ui-filter-bar','login-entry__form','auth-form'].includes(name)))continue
   missing.push(path+':'+(source.slice(0,match.index).split('\n').length))
  }
 }
 assert.deepEqual(missing,[])
})
