import test from 'node:test'
import assert from 'node:assert/strict'
import {readdirSync,readFileSync} from 'node:fs'
import {join} from 'node:path'

test('account association filters use server search rather than enumerable selects in all apps',()=>{
 const visit=dir=>readdirSync(dir,{withFileTypes:true}).flatMap(entry=>entry.isDirectory()?visit(join(dir,entry.name)):entry.name.endsWith('.vue')?[join(dir,entry.name)]:[])
 for(const app of ['admin-web','platform-web','agent-web','user-web'])for(const file of visit(`${app}/src`)){
  for(const tag of readFileSync(file,'utf8').matchAll(/<FilterAssociationSelect\b[^>]*>/gs))assert.doesNotMatch(tag[0],/kind=["'](?:member|agent|actor)["']/,file)
 }
})
