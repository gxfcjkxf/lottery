import { test, expect, type Page } from '@playwright/test'
import {checkWithdrawalObservability} from './withdrawal-observability'

const userOrigin = process.env.TEST_WITHDRAWAL_USER_ORIGIN ?? 'http://localhost:5173'
const adminOrigin = process.env.TEST_WITHDRAWAL_ADMIN_ORIGIN ?? 'http://localhost:5174'
const adminUsername = process.env.TEST_WITHDRAWAL_ADMIN_USERNAME
const adminPassword = process.env.TEST_WITHDRAWAL_ADMIN_PASSWORD
const userPassword = process.env.TEST_WITHDRAWAL_USER_PASSWORD
const brand = '0199a000-0000-7000-8000-000000000001'

async function responseData(response: Awaited<ReturnType<Page['request']['get']>>, status=200) {
  expect(response.status()).toBe(status)
  const envelope=await response.json()
  expect(envelope.success).toBe(true)
  return envelope.data
}
test('closed user admission and exact administrative receipt recovery preserve source funds', async ({page, context}, info) => {
  test.skip(!adminUsername || !adminPassword || !userPassword, 'Provide explicitly owned synthetic withdrawal fixture credentials')
  test.setTimeout(60_000)
  const errors:string[]=[]
  page.on('pageerror', error=>errors.push(error.message))
  const username=info.project.name==='mobile' ? 'withdraw_paid_user' : 'withdraw_user'
  await page.addInitScript(()=>localStorage.setItem('luma-language','en'))
  await page.goto(`${userOrigin}/login`)
  await page.getByLabel('Username or phone', {exact:true}).fill(username)
  await page.getByLabel('Password', {exact:true}).fill(userPassword!)
  await page.locator('.auth-form').getByRole('button',{name:/Continue/}).click()
  await expect(page).toHaveURL(/\/account$/)
  const originalWallet=await responseData(await page.request.get(`${userOrigin}/api/v1/wallet`))
  const available=await responseData(await page.request.get(`${userOrigin}/api/v1/withdrawal-availability`))
  expect(available).toMatchObject({can_apply:true,eligibility_configured:true,policy_enabled:true,reason_code:'AVAILABLE',real_payments:false})
  const qualification=await responseData(await page.request.get(`${userOrigin}/api/v1/withdrawal-qualification`))
  expect(qualification).toMatchObject({meets_turnover:false,valid_points:'0',valid_order_count:'0',credit_numerator:'0',credit_denominator:'1'})
  const list=await responseData(await page.request.get(`${userOrigin}/api/v1/withdrawals?state=reviewing`))
  expect(list.items).toHaveLength(1)
  const order=list.items[0]
  expect(order.points).toBe('80')
  expect(originalWallet.withdrawal_points).toBe('80')
  await page.goto(`${userOrigin}/withdraw`)
  const userPanel=page.locator('.withdrawal-page')
  await expect(userPanel).toContainText('Turnover')
  await expect(userPanel.getByRole('button',{name:'Submit request',exact:true})).toBeDisabled()
  await expect(userPanel).not.toContainText('8,200')
  await expect(userPanel).not.toContainText('68%')
  await userPanel.getByRole('button',{name:'History',exact:true}).click()
  await expect(userPanel.locator('.withdrawal-history')).toContainText('Reviewing')
  const denied=await page.request.post(`${userOrigin}/api/v1/withdrawals`,{headers:{Origin:userOrigin,'Idempotency-Key':crypto.randomUUID(),'X-Withdrawal-Actor-Context':available.actor_context},data:{points:'50',source_allocation:[{source:'recharge',state:'available',points:'50'}]}})
  expect(denied.status()).toBe(409)
  expect((await denied.json()).error.code).toBe('WITHDRAWAL_ACTIVE_ORDER')
  expect(await responseData(await page.request.get(`${userOrigin}/api/v1/wallet`))).toEqual(originalWallet)
  await page.goto(adminOrigin)
  await page.getByTestId('admin-language').selectOption('en')
  await page.getByLabel('Account',{exact:true}).fill(adminUsername!)
  await page.getByLabel('Password',{exact:true}).fill(adminPassword!)
  await page.getByRole('button',{name:'Sign in and load members',exact:true}).click()
  await page.locator('.directory-brand-bar select').selectOption(brand)
  if(info.project.name==='mobile') await page.locator('.mobile-nav button').nth(3).click()
  else await page.locator('.side-nav').getByRole('button',{name:/Funds.*ledger/}).click()
  const adminPanel=page.locator('.withdrawal-management')
  await expect(adminPanel.getByRole('heading',{name:'Withdrawal requests',exact:true})).toBeVisible()
  await adminPanel.getByLabel('Member ID',{exact:true}).fill(order.member_id)
  await adminPanel.getByRole('button',{name:'Filter',exact:true}).click()
  await adminPanel.getByRole('button',{name:'History',exact:true}).click()
  await expect(adminPanel.locator('.withdrawal-history-row')).toHaveCount(1)
  await adminPanel.getByRole('button',{name:'Approve',exact:true}).click()
  await adminPanel.getByLabel('Reason',{exact:true}).fill('中文审核原因保持原值')
  const intents:Array<{key:string,body:string|null}>=[]
  const approvalUrl=`${adminOrigin}/api/v1/admin/withdrawals/${order.id}/approve`
  await page.route(approvalUrl,async route=>{
    intents.push({key:route.request().headers()['idempotency-key']!,body:route.request().postData()})
    const response=await route.fetch()
    expect(response.status()).toBe(200)
    if(intents.length===1) await route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({success:false,error:{code:'INJECTED_RECEIPT_LOSS',message:'Receipt lost after commit'}})})
    else await route.fulfill({response})
  })
  await adminPanel.getByRole('button',{name:'Confirm Approve',exact:true}).click()
  await expect(adminPanel.getByRole('button',{name:'Replay same action',exact:true})).toBeVisible()
  await adminPanel.getByRole('button',{name:'Refresh',exact:true}).click()
  await expect(adminPanel.getByRole('button',{name:'Replay same action',exact:true})).toBeVisible()
  await expect(adminPanel.locator('tbody')).toContainText('Withdrawal in progress')
  await page.getByTestId('admin-language').selectOption('zh-CN')
  await expect(adminPanel).toContainText('中文审核原因保持原值')
  await page.getByTestId('admin-language').selectOption('en')
  await adminPanel.getByRole('button',{name:'Replay same action',exact:true}).click()
  await expect(adminPanel.getByRole('button',{name:'Replay same action',exact:true})).toHaveCount(0)
  expect(intents).toHaveLength(2)
  expect(intents[0]).toEqual(intents[1])
  await page.unroute(approvalUrl)
  await adminPanel.getByRole('button',{name:'Refresh',exact:true}).click()
  await adminPanel.getByRole('button',{name:'History',exact:true}).click()
  const lastAction=info.project.name==='mobile' ? 'Mark paid (internal)' : 'Mark failed'
  await adminPanel.getByRole('button',{name:lastAction,exact:true}).click()
  await adminPanel.getByLabel('Reason',{exact:true}).fill('内部模拟结果，无外部转账')
  if(info.project.name==='mobile') await expect(adminPanel).toContainText('no external transfer')
  await adminPanel.getByRole('button',{name:info.project.name==='mobile' ? 'Confirm' : `Confirm ${lastAction}`,exact:true}).click()
  await expect(adminPanel.getByRole('status')).toContainText(info.project.name==='mobile' ? 'Withdrawn' : 'Failed')
  await adminPanel.getByRole('button',{name:'Refresh',exact:true}).click()
  await expect(adminPanel.locator('tbody')).toContainText(info.project.name==='mobile' ? 'Withdrawn' : 'Failed')
  await adminPanel.screenshot({path:info.outputPath('withdrawal-admin.png')})
  await checkWithdrawalObservability(page,context,info,order,{userOrigin,adminOrigin},errors,info.project.name==='mobile'?'0':'1')
})
