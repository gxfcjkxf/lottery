import {describe,it,expect} from 'vitest'
import {registrationShareLink} from './registration-link'
const code='A1B2C3D4E5F607182930ABCD'
describe('shared registration links',()=>{
 it('targets the user register route with exactly the agent attribution field',()=>{
  expect(registrationShareLink('http://127.0.0.1:5183','agent',code)).toBe(`http://127.0.0.1:5183/register?agent_code=${code}`)
 })
 it('keeps referral links on the same registration route and normalizes the code',()=>{
  expect(registrationShareLink('https://brand.example','referral',code.toLowerCase())).toBe(`https://brand.example/register?referral_code=${code}`)
 })
 it.each(['','javascript:alert(1)','https://user:password@brand.example','https://brand.example/admin','https://brand.example?agent_code=wrong','https://brand.example/#/agent'])('rejects an unconfigured or invalid user origin %s',origin=>{
  expect(()=>registrationShareLink(origin,'agent',code)).toThrow()
 })
 it.each(['','AGENT-CODE','A'.repeat(23)])('rejects unusable source codes %s',value=>{
  expect(()=>registrationShareLink('https://brand.example','agent',value)).toThrow()
 })
})
