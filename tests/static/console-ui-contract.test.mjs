import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync,readdirSync} from 'node:fs'
import {createRequire} from 'node:module'
const require=createRequire(new URL('../../admin-web/package.json',import.meta.url))
const {parse}=require('vue/compiler-sfc')
const shared=['PlatformLottery.vue','PlatformDrawOverview.vue','ConfiguredOrdersPanel.vue','ConfiguredPublishing.vue','ArchiveReport.vue','AutomationSettings.vue','DrawScheduleSettings.vue','FeedMappingEditor.vue','PlatformDrawOverview.vue','RechargeQuotaPanel.vue']
const files=['admin-web/src','platform-web/src'].flatMap(dir=>readdirSync(dir).filter(name=>name.endsWith('.vue')).map(name=>`${dir}/${name}`)).concat(shared.map(name=>`shared/src/${name}`))
function walk(node,visit,parents=[]){if(node.type===1)visit(node,parents);for(const child of node.children??[])walk(child,visit,[...parents,node])}
function check(visit,paths=files){const errors=[];for(const path of paths){const {descriptor}=parse(readFileSync(path,'utf8'));if(descriptor.template)walk(descriptor.template.ast,(node,parents)=>visit(node,path,errors,parents))}assert.deepEqual(errors,[])}
const classes=node=>node.props.find(prop=>prop.name==='class')?.value?.content?.split(/\s+/)??[]
test('both consoles and their business components use shared list table styles',()=>check((node,path,errors)=>{
 if(node.tag==='table'&&!classes(node).some(value=>['console-table','calendar-grid','recharge-detail-table'].includes(value)))errors.push(`${path}:${node.loc.start.line}`)
}))
test('both consoles use the shared modal shell',()=>check((node,path,errors)=>{
 if(node.tag==='dialog'||node.props.some(prop=>prop.name==='role'&&prop.value?.content==='dialog'))errors.push(`${path}:${node.loc.start.line}`)
}))
const paginationFiles=['admin-web','platform-web','user-web','agent-web','shared'].flatMap(dir=>readdirSync(`${dir}/src`,{recursive:true}).filter(name=>name.endsWith('.vue')).map(name=>`${dir}/src/${name}`))
test('pagination buttons use the shared component, including footers and button-only controls',()=>check((node,path,errors,parents)=>{
 if(node.tag!=='button')return
 const label=node.children.map(child=>child.loc.source).join('')
 const pageLabel=/dbe7a5d7|2e9c9e7a|ee97c955|admin\.withdrawal\.(previous|next)|\b(previousPage|nextPage|previousLabel|nextLabel)\b|\.(previous|next)\s*}}|t\(['"](previous|next|previousOrders|nextOrders)['"]\)/.test(label)
 if(pageLabel&&!parents.some(parent=>['TablePagination','PaginationControls'].includes(parent.tag)))errors.push(`${path}:${node.loc.start.line}`)
},paginationFiles))

test('all pagination summaries are generated centrally from page, record count and locale',()=>check((node,path,errors)=>{
 if(!['TablePagination','PaginationControls'].includes(node.tag))return
 const bindings=node.props.filter(prop=>prop.type===7&&prop.name==='bind').map(prop=>prop.arg?.content)
 if(bindings.includes('summary')||!['page','count','total','locale'].every(prop=>bindings.includes(prop)))errors.push(`${path}:${node.loc.start.line}`)
},paginationFiles))

const managementPages=['admin-web/src','platform-web/src','agent-web/src/pages'].flatMap(dir=>readdirSync(dir).filter(name=>name.endsWith('.vue')).map(name=>`${dir}/${name}`))
test('pageable management lists share one surface; embedded business details keep their local scope',()=>check((node,path,errors,parents)=>{
 if(!['TablePagination','PaginationControls'].includes(node.tag))return
 const embedded=parents.some(parent=>parent.type===1&&classes(parent).some(value=>['commission-detail','adjustment-history'].includes(value)))
 if(!embedded&&!parents.some(parent=>['ManagementList','DetailModal'].includes(parent.tag)))errors.push(`${path}:${node.loc.start.line}: list needs ManagementList`)
},managementPages))
test('shared list surfaces do not nest another list surface or put global actions in tablists',()=>check((node,path,errors,parents)=>{
 const scopeStart=parents.findLastIndex(parent=>parent.tag==='DetailModal')
 if(node.tag==='ManagementList'&&parents.slice(scopeStart+1).some(parent=>parent.tag==='ManagementList'))errors.push(`${path}:${node.loc.start.line}: nested list surface`)
 const inTabs=parents.some(parent=>parent.props?.some(prop=>prop.name==='role'&&prop.value?.content==='tablist'))
 const inList=parents.some(parent=>parent.tag==='ManagementList')
 if(node.tag==='button'&&inTabs&&inList&&!node.props.some(prop=>prop.name==='role'&&prop.value?.content==='tab'))errors.push(`${path}:${node.loc.start.line}: action mixed with tabs`)
},managementPages))

const listSources=[...managementPages,...readdirSync('shared/src').filter(name=>name.endsWith('.vue')).map(name=>`shared/src/${name}`)]
test('management filters and pagers use their shared list slots',()=>check((node,path,errors,parents)=>{
 const scope=parents.slice(Math.max(parents.findLastIndex(parent=>parent.tag==='ManagementList'),parents.findLastIndex(parent=>parent.tag==='DetailModal')))
 if(scope[0]?.tag!=='ManagementList')return
 const slot=name=>scope.some(parent=>parent.tag==='template'&&parent.props.some(prop=>prop.name==='slot'&&prop.arg?.content===name))
 if(classes(node).includes('ui-filter-bar')){
  if(node.tag!=='form'||!slot('filters'))errors.push(`${path}:${node.loc.start.line}: filter needs a form in the filters slot`)
 }
 if(['TablePagination','PaginationControls'].includes(node.tag)&&!slot('pagination'))errors.push(`${path}:${node.loc.start.line}: pager needs the pagination slot`)
},listSources))
test('management table headers are retained for empty result sets',()=>check((node,path,errors,parents)=>{
 if(node.tag!=='table'||!parents.some(parent=>parent.tag==='ManagementList'))return
 const scope=parents.slice(parents.findLastIndex(parent=>parent.tag==='ManagementList'))
 if([...scope,node].some(parent=>parent.props?.some(prop=>prop.name==='if'&&/\.length/.test(prop.exp?.content??''))))errors.push(`${path}:${node.loc.start.line}: rows must not control table visibility`)
},listSources))

// Cover lists without pagination as well as tables and detail sublists. A new
// source cannot disappear from review merely by omitting a pagination control.
test('every management collection source is registered in the full audit',()=>{
 const audit=readFileSync('docs/48-list-page-audit.md','utf8')
 const sources=[...managementPages,...readdirSync('shared/src').filter(name=>name.endsWith('.vue')).map(name=>`shared/src/${name}`)]
 const missing=[]
 for(const path of sources){
  const {descriptor}=parse(readFileSync(path,'utf8'));let collection=false
  if(descriptor.template)walk(descriptor.template.ast,node=>{
   if(['ManagementTable','TablePagination','PaginationControls','table'].includes(node.tag))collection=true
   if(['article','li','tr'].includes(node.tag)&&node.props.some(prop=>prop.name==='for'))collection=true
  })
  if(collection&&!audit.includes(`\`${path}\``))missing.push(path)
 }
 assert.deepEqual(missing,[])
})
