import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,installLayoutApi,expectNoPageOverflow} from './fixture'

for(const surface of ['user','brand','platform','agent'] as const){
 test(`shared operation toast is usable across views (${surface})`,async({page},info)=>{
  await page.clock.install()
  const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message))
  await installLayoutApi(page,'zh-CN')
  await page.goto(origins[surface])
  await expect(page.locator('form').first()).toBeVisible()
  await page.waitForLoadState('networkidle')
  await page.evaluate(async path=>{
   const {showOperationToast,dismissOperationToast}=await import(path)
   dismissOperationToast()
   const source=Symbol('result')
   showOperationToast(()=> '旧的投注回执','success',source)
   showOperationToast(()=> '投注成功','success',source)
   showOperationToast(()=> '尚未开盘，暂不可投注。','error')
  },'/@fs'+resolve('shared/src/operation-toast.ts'))
  const stack=page.locator('.operation-toast-stack'),success=stack.locator('.toast-success'),failure=stack.locator('.toast-error')
  await expect(stack.locator('.toast')).toHaveCount(2)
  await expect(success).toContainText('投注成功');await expect(stack).not.toContainText('旧的投注回执')
  await expect(failure).toHaveAttribute('role','alert')
  const preset=page.viewportSize()!
  for(const width of info.project.name.endsWith('reflow')?[320,480,900,1280]:[preset.width]){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
   await expectNoPageOverflow(page)
   const box=await stack.boundingBox();expect(box).not.toBeNull();expect(box!.x).toBeGreaterThanOrEqual(0);expect(box!.y).toBeGreaterThanOrEqual(0)
   expect(page.viewportSize()!.width-box!.x-box!.width).toBeLessThanOrEqual(21)
   expect(page.viewportSize()!.height-box!.y-box!.height).toBeLessThanOrEqual(35)
   await expect(success.locator('button')).toHaveCSS('width','44px')
   await expect(success.locator('.toast-message')).toHaveCSS('font-size','14px')
  }
  await success.getByRole('button').focus();await page.clock.fastForward(4000)
  await expect(success).toBeVisible();await expect(failure).toHaveCount(0)
  await page.screenshot({path:info.outputPath(`${surface}-toast.png`)})
  await success.getByRole('button').click();await expect(stack).toHaveCount(0)
  expect(errors).toEqual([])
 })
}
