import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
import {readFileSync} from 'node:fs'
import {origins,gameId,expectNoPageOverflow} from './fixture'
const catalog=JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json','utf8'))
export function registerManagementButtonTests(){
 for(const surface of ['admin','platform'] as const)test(`desktop ${surface} rule actions keep content width and stable hover`,async({page},info)=>{
  const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
  await page.route('**/api/**',route=>{expect(route.request().method()).toBe('GET');expect(new URL(route.request().url()).pathname).toBe(`/api/v1/${surface}/platform-games/rule-preview/template`);return route.fulfill({json:{success:true,data:catalog}})})
  await page.route('**/__button-check',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><body><div id="app"><div id="button-harness" class="management-console"></div></div><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import Component from'/@fs${resolve('shared/src/ConfiguredRuleWorkbench.vue')}';import'/@fs${resolve('shared/src/styles.css')}';import'/src/style.css';import'/@fs${resolve('shared/src/controls.css')}';import'/@fs${resolve('shared/src/management.css')}';createApp(Component,{surface:'${surface}',brandId:'${gameId}',locale:'zh-CN'}).mount('#button-harness')</script></body></html>`}))
  await page.goto((surface==='admin'?origins.brand:origins.platform)+'/__button-check')
  await page.locator('.configured-workbench > summary').click()
  await page.getByRole('button',{name:'载入通用示例配置',exact:true}).click()
  const button=page.getByRole('button',{name:'验证配置',exact:true});await button.scrollIntoViewIfNeeded()
  const before=await button.boundingBox();expect(before).toBeTruthy();expect(before!.width).toBeLessThan(240)
  const styles=await button.evaluate(el=>({image:getComputedStyle(el).backgroundImage,transition:getComputedStyle(el).transitionProperty}))
  expect(styles.image).toBe('none');expect(styles.transition).not.toContain('all')
  for(const y of [1,before!.height/2,before!.height-1,1,before!.height/2]){
   await page.mouse.move(before!.x+before!.width/2,before!.y+y)
   await button.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))))
   const after=await button.boundingBox();expect(after!.x).toBeCloseTo(before!.x,1);expect(after!.y).toBeCloseTo(before!.y,1);expect(after!.width).toBeCloseTo(before!.width,1)
   expect(await button.evaluate(el=>getComputedStyle(el).backgroundImage)).toBe('none')
  }
  await expectNoPageOverflow(page);await page.screenshot({path:info.outputPath(`rule-action-hover-${surface}.png`)})
  expect(errors).toEqual([])
 })
}
