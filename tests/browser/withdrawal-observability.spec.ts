import {test,expect} from '@playwright/test'
import {checkWithdrawalObservability,responseData} from './withdrawal-observability'

const userOrigin=process.env.TEST_WITHDRAWAL_USER_ORIGIN??'http://localhost:5173'
const adminOrigin=process.env.TEST_WITHDRAWAL_ADMIN_ORIGIN??'http://localhost:5174'
const adminUsername=process.env.TEST_WITHDRAWAL_ADMIN_USERNAME
const adminPassword=process.env.TEST_WITHDRAWAL_ADMIN_PASSWORD
const userPassword=process.env.TEST_WITHDRAWAL_USER_PASSWORD
const brand='0199a000-0000-7000-8000-000000000001'

// This read-only verification requires the owned fixture's failed/paid workflows
// to have completed. It never recreates a withdrawal, refunds it or resets data.
test('completed withdrawal projection export and historical inbox agree without changing funds',async({page,context},info)=>{
  test.skip(!adminUsername||!adminPassword||!userPassword,'Provide owned synthetic withdrawal fixture credentials')
  test.setTimeout(60_000)
  const errors:string[]=[]
  page.on('pageerror',e=>errors.push(e.message))
  await page.addInitScript(()=>localStorage.setItem('luma-language','en'))
  await page.goto(`${userOrigin}/login`)
  await page.getByLabel('Username or phone',{exact:true}).fill(info.project.name==='mobile'?'withdraw_paid_user':'withdraw_user')
  await page.getByLabel('Password',{exact:true}).fill(userPassword!)
  await page.locator('.auth-form').getByRole('button',{name:/Continue/}).click()
  await expect(page).toHaveURL(/\/account$/)
  const wallet=await responseData(await page.request.get(`${userOrigin}/api/v1/wallet`))
  const orders=await responseData(await page.request.get(`${userOrigin}/api/v1/withdrawals?state=${info.project.name==='mobile'?'paid':'failed'}`))
  expect(orders.items).toHaveLength(1)
  await page.goto(adminOrigin)
  await page.getByTestId('admin-language').selectOption('en')
  if(info.project.name==='mobile')await page.locator('.mobile-nav button').nth(1).click()
  else await page.locator('.side-nav').getByRole('button',{name:/Users and members/}).click()
  await page.getByLabel('Account',{exact:true}).fill(adminUsername!)
  await page.getByLabel('Password',{exact:true}).fill(adminPassword!)
  await page.getByRole('button',{name:'Sign in and load members',exact:true}).click()
  await page.locator('.directory-brand-bar select').selectOption(brand)
  await checkWithdrawalObservability(page,context,info,orders.items[0],{userOrigin,adminOrigin},errors,'0')
  expect(await responseData(await page.request.get(`${userOrigin}/api/v1/wallet`))).toEqual(wallet)
})
