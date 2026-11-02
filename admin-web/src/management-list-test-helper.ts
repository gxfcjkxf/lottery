import {readFileSync} from 'node:fs'
import {compileScript,parse} from 'vue/compiler-sfc'
import ts from 'typescript'
import * as VueRuntime from 'vue'
import {defineComponent,h,type Component} from 'vue'

// Use the real list shell so every named slot participates in business tests.
const source=readFileSync(new URL('../../shared/src/ManagementList.vue',import.meta.url),'utf8')
const content=compileScript(parse(source).descriptor,{id:'management-list-test',inlineTemplate:true}).content
const javascript=ts.transpileModule(content,{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext}}).outputText
const body=javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']vue["'];?\s*$/gm,(_m,bindings:string)=>`const {${bindings.replace(/\s+as\s+/g,': ')}}=runtime;`).replace(/export\s+default\s+/,'return ')
export const ManagementListTestComponent=new Function('runtime',body)(VueRuntime) as Component
// Native popover placement/focus is exercised in browsers, while host-renderer
// tests can still invoke the actual business actions inside the menu.
export const MemberActionsMenuTestComponent=defineComponent({props:['id','label','disabled'],setup(props,{slots}){return()=>h('div',{'data-menu':props.id},[h('button',{disabled:props.disabled,'aria-label':props.label}),slots.default?.()])}})

export function clickHostButton(node:any) {
 if(!node)throw new Error('Button was not found')
 if(node.props.disabled)return
 if(typeof node.props.onClick==='function'){node.props.onClick();return}
 if(node.props.type==='submit'){
  let form=node.parent
  while(form&&form.tag!=='form')form=form.parent
  if(typeof form?.props.onSubmit!=='function')throw new Error('Submit button has no owning form')
  form.props.onSubmit({preventDefault(){},stopPropagation(){},target:node});return
 }
 throw new Error('Button has no click or submit action')
}
export function owningHostForm(node:any){while(node&&node.tag!=='form')node=node.parent;if(!node)throw new Error('Field has no owning form');return node}
