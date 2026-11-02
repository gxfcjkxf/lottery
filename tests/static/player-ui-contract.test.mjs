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
test('user list pagination reuses the shared component',()=>{
 for(const file of ['WalletSummary','BettingHistory','NotificationsPanel','RechargePanel','JoinCodesPanel','DrawArchive']){
  const source=readFileSync(`user-web/src/${file}.vue`,'utf8')
  assert.match(source,/<TablePagination\b/,file)
  assert.doesNotMatch(source,/<(?:nav|footer|div)\b[^>]*class="(?:pager|history-pagination|notifications-pagination|recharge-pagination|join-code-pagination|page-actions)"/,file)
 }
})
