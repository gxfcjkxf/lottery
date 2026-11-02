import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync,readdirSync} from 'node:fs'
import {createRequire} from 'node:module'
const require=createRequire(new URL('../../user-web/package.json',import.meta.url))
const {parse}=require('vue/compiler-sfc')
function vueFiles(dir){return readdirSync(dir,{withFileTypes:true}).flatMap(entry=>entry.isDirectory()?vueFiles(`${dir}/${entry.name}`):entry.name.endsWith('.vue')?[`${dir}/${entry.name}`]:[])}
const files=['user-web/src','agent-web/src'].flatMap(vueFiles)
function walk(node,visit){if(node.type===1)visit(node);for(const child of node.children??[])walk(child,visit)}
function check(visit){const errors=[];for(const path of files){const {descriptor}=parse(readFileSync(path,'utf8'));if(descriptor.template)walk(descriptor.template.ast,node=>visit(node,path,errors))}assert.deepEqual(errors,[])}
test('user and agent action buttons have explicit native types',()=>check((node,path,errors)=>{
 if(node.tag==='button'&&!node.props.some(prop=>prop.name==='type'))errors.push(`${path}:${node.loc.start.line}`)
}))
test('user and agent business dialogs use the shared modal shell',()=>check((node,path,errors)=>{
 const classes=node.props.find(prop=>prop.name==='class')?.value?.content?.split(/\s+/)??[]
 if(node.tag==='dialog'||node.props.some(prop=>prop.name==='role'&&prop.value?.content==='dialog')&&!classes.includes('workspace-menu-layer'))errors.push(`${path}:${node.loc.start.line}`)
 if(node.tag==='details'&&classes.some(value=>['ledger-details','account-complete'].includes(value)))errors.push(`${path}:${node.loc.start.line}`)
}))
test('all user record sources are registered, including sublists without their own pager',()=>{
 const audit=readFileSync('docs/48-user-list-audit.md','utf8'),missing=[]
 for(const path of vueFiles('user-web/src')){
  const {descriptor}=parse(readFileSync(path,'utf8'));let collection=false
  if(descriptor.template)walk(descriptor.template.ast,node=>{
   if(['TablePagination','PaginationControls','table'].includes(node.tag)||['li','article','tr'].includes(node.tag)&&node.props.some(prop=>prop.name==='for'))collection=true
  })
  if(collection&&!audit.includes(path.split('/').at(-1)))missing.push(path)
 }
 assert.deepEqual(missing,[])
})
test('user pagination is an actual template component with a working navigation callback',()=>{
 const missing=[]
 for(const path of vueFiles('user-web/src')){
  const {descriptor}=parse(readFileSync(path,'utf8'))
  if(descriptor.template)walk(descriptor.template.ast,node=>{
   if(!['TablePagination','PaginationControls'].includes(node.tag))return
   const bindings=node.props.filter(prop=>prop.type===7&&prop.name==='bind').map(prop=>prop.arg?.content)
   if(!['page','count','total','locale','page-size','go-to'].every(prop=>bindings.includes(prop)))missing.push(`${path}:${node.loc.start.line}`)
  })
 }
 assert.deepEqual(missing,[])
})
