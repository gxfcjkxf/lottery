import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
import {origins} from './fixture'

test('only a dialog cancellation can close the modal and closeDisabled is respected',async({page})=>{
 const component='/@fs/'+resolve('shared/src/DetailModal.vue')
 const source=await(await page.request.get(origins.brand+component)).text()
 const vue=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
 await page.route('**/__modal-cancel',route=>route.fulfill({contentType:'text/html',body:`<div id="app"></div><script type="module">
 import{createApp,h}from'${vue}';import DetailModal from'${component}';
 createApp({data:()=>({open:true,locked:false,draft:'Keep the draft'}),render(){return this.open?h(DetailModal,{title:'Modal regression',closeLabel:'Close',closeDisabled:this.locked,onClose:()=>this.open=false},{default:()=>[
 h('label',['Draft',h('input',{value:this.draft,onInput:event=>this.draft=event.target.value})]),
 h('label',['Lock closing',h('input',{type:'checkbox',checked:this.locked,onChange:event=>this.locked=event.target.checked})]),h('input',{type:'file'})
 ]}):null}}).mount('#app')
 </script>`}))
 await page.goto(origins.brand+'/__modal-cancel')
 const modal=page.getByRole('dialog',{name:'Modal regression'})
 await expect(modal).toBeVisible()
 await modal.locator('input[type=file]').dispatchEvent('cancel',{bubbles:true})
 await expect(modal).toBeVisible()
 await expect(page.getByLabel('Draft')).toHaveValue('Keep the draft')
 await page.getByLabel('Lock closing').check()
 await page.keyboard.press('Escape')
 await expect(modal).toBeVisible()
 await expect(page.getByRole('button',{name:'Close'})).toBeDisabled()
 await page.getByLabel('Lock closing').uncheck()
 await page.keyboard.press('Escape')
 await expect(modal).toHaveCount(0)
})
