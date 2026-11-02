import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,expectNoPageOverflow,expectInsideViewport} from './fixture'
for(const locale of ['en','zh-CN'] as const)test(`four surfaces share filter and form geometry (${locale})`,async({page},info)=>{
 const zh=locale==='zh-CN',preset=page.viewportSize()!
 const measured:Record<string,unknown>={}
 for(const surface of ['user','brand','platform','agent'] as const){
  const directory=surface==='brand'?'admin-web':surface+'-web',origin=origins[surface]
  const source=await(await page.request.get(origin+'/src/'+(surface==='user'?'Page':'App')+'.vue')).text()
  const vuePath=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
  const url=origin+'/__unified-forms'
  await page.route(url,route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/UnifiedFormsHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';${surface==='agent'?'':`import'/@fs/${resolve(directory+'/src/style.css')}';`}${surface==='user'?`import'/@fs/${resolve('user-web/src/workspace.css')}';`:`import'/@fs/${resolve('shared/src/management.css')}';`}import'/@fs/${resolve('shared/src/key-value.css')}';import'/@fs/${resolve('shared/src/controls.css')}';createApp(Harness,${JSON.stringify({locale,surface})}).mount('#app')</script></html>`}))
  await page.goto(url)
  const search=page.locator('.ui-filter-bar input'),query=page.locator('.ui-filter-bar button[type=submit]')
  await search.fill('member001');await query.click();await expect(page.getByTestId('query-count')).toHaveText('1')
  await page.locator('.ui-filter-bar').getByRole('button',{name:zh?'清除':'Clear',exact:true}).click();await expect(search).toHaveValue('')
  const viewportSizes=info.project.name.includes('reflow')?[320,640,900,901,1440]:[preset.width]
  for(const width of viewportSizes){
   if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
   await expectNoPageOverflow(page)
   const geometry=await page.locator('.ui-filter-bar').evaluate(el=>{
    const style=getComputedStyle(el),input=getComputedStyle(el.querySelector('input')!),button=getComputedStyle(el.querySelector('button')!)
    return {gap:style.gap,padding:style.padding,border:style.borderWidth,inputHeight:input.height,inputRadius:input.borderRadius,inputPadding:input.padding,buttonHeight:button.height,buttonRadius:button.borderRadius}
   })
   const shortLandscape=await page.evaluate(()=>matchMedia('(max-height:450px) and (orientation:landscape)').matches)
   expect(geometry).toMatchObject({gap:shortLandscape?'4px 10px':'10px',padding:'0px',border:'0px'})
   const key=`filter-${width}`
   if(surface==='user')measured[key]=geometry;else expect(geometry).toEqual(measured[key])
   await page.getByRole('button',{name:zh?'创建会员':'Create member',exact:true}).click()
   const dialog=page.getByRole('dialog')
   await expectInsideViewport(dialog);await expectNoPageOverflow(page)
   expect((await dialog.boundingBox())!.width).toBeLessThanOrEqual(520)
   await expect(dialog.getByRole('button',{name:zh?'保存':'Save',exact:true})).toBeDisabled()
   await page.mouse.move(0,0)
   await dialog.locator('.detail-modal-close').evaluate(async el=>{getComputedStyle(el).backgroundColor;await Promise.all(el.getAnimations().map(animation=>animation.finished))})
   const divider=await dialog.evaluate(el=>{
    const heading=el.querySelector('.detail-modal-heading')!,content=el.querySelector('.detail-modal-content')!
    return {headingMargin:getComputedStyle(heading).margin,gap:content.getBoundingClientRect().top-heading.getBoundingClientRect().bottom,padding:getComputedStyle(content).paddingTop}
   })
   expect(divider).toEqual({headingMargin:'0px',gap:0,padding:'12px'})
   const form=await dialog.locator('.ui-form').evaluate(el=>{
    const s=getComputedStyle(el),label=getComputedStyle(el.querySelector('label')!),actions=getComputedStyle(el.querySelector('.ui-form-actions')!),input=getComputedStyle(el.querySelector('input')!)
    return {gap:s.gap,labelGap:label.gap,actionsJustify:actions.justifyContent,actionsDirection:actions.flexDirection,inputHeight:input.height,inputRadius:input.borderRadius,labelLineHeight:label.lineHeight,close:(()=>{const st=getComputedStyle(el.closest('dialog')!.querySelector('.detail-modal-close')!);return {width:st.width,height:st.height,border:st.borderWidth,background:st.backgroundColor,shadow:st.boxShadow}})(),fields:[...el.querySelectorAll('input:not([data-select-validation])')].map(field=>{const st=getComputedStyle(field);return {width:st.width,fontFamily:st.fontFamily,fontSize:st.fontSize,fontWeight:st.fontWeight,padding:st.padding}})}
   })
   expect(form).toMatchObject({gap:'12px',labelGap:'6px',actionsJustify:'flex-end',actionsDirection:'row'})
   const formKey=`form-${width}`
   if(surface==='user')measured[formKey]=form;else expect(form).toEqual(measured[formKey])
   const actions=dialog.locator('.ui-form-actions'),submit=actions.getByRole('button')
   await expect(actions.getByRole('button')).toHaveCount(1)
   const box=(await submit.boundingBox())!,parent=(await actions.boundingBox())!
   expect(box.width).toBeLessThan(parent.width-30)
   if(!info.project.name.includes('reflow')||[320,1440].includes(width))await page.screenshot({path:info.outputPath(`unified-form-${surface}-${locale}-${width}.png`)})
   await dialog.getByRole('button',{name:zh?'关闭':'Close',exact:true}).click()
  }
  if(info.project.name.includes('reflow'))await page.setViewportSize(preset)
 }
})
