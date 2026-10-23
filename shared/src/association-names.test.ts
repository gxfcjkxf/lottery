import { reactive } from 'vue'
import { describe, expect, it } from 'vitest'
import { associationLabel, copyAssociationNames, readApiJson } from './association-names'

describe('authorized association display metadata', () => {
 const id = '11111111-1111-4111-8111-111111111111'
 const role = '22222222-2222-4222-8222-222222222222'
 it('shows associated names through Vue proxies without changing DTO keys or write bodies', async () => {
  const body = { success: true, data: { id, role_ids: [role], brand_id: id, member_id: id }, associations: { [`role:${role}`]: { name: 'Support' }, [`brand:${id}`]: { name: 'Aurora' }, [`member:${id}`]: { name: 'test001' } } }
  const result = await readApiJson(new Response(JSON.stringify(body)))
  const record = reactive(result.data)
  expect(associationLabel(record,'role',role,'en')).toBe('Support')
  expect(associationLabel(record,'member',id,'zh-CN')).toBe('test001')
  expect(associationLabel(record,'brand',id,'en')).toBe('Aurora')
  expect(Object.keys(record)).toEqual(Object.keys(body.data))
  expect(JSON.stringify(record)).toBe(JSON.stringify(body.data))
  const frozenBody = JSON.parse(JSON.stringify(record))
  copyAssociationNames(record, frozenBody)
  expect(associationLabel(frozenBody,'member',id,'en')).toBe('test001')
  expect(JSON.stringify(frozenBody)).toBe(JSON.stringify(body.data))
 })
 it('keeps equally shaped records from separate responses isolated', async () => {
  const parse = (name: string) => readApiJson(new Response(JSON.stringify({success:true,data:{member_id:id},associations:{[`member:${id}`]:{name}}})))
  const first = await parse('first-account'), second = await parse('second-account')
  expect(associationLabel(first.data,'member',id,'en')).toBe('first-account')
  expect(associationLabel(second.data,'member',id,'en')).toBe('second-account')
  expect(associationLabel({member_id:id},'member',id,'en')).toBe('Unable to load associated information.')
 })
 it('localizes built-in roles and explicitly reports missing or failed names without exposing IDs', async () => {
  const builtIn = await readApiJson(new Response(JSON.stringify({success:true,data:{role_ids:[role]},associations:{[`role:${role}`]:{name:'Bootstrap administrator',built_in:true}}})))
  expect(associationLabel(builtIn.data,'role',role,'zh-CN')).toBe('系统管理员')
  expect(associationLabel(builtIn.data,'role',role,'en')).toBe('System administrator')
  const empty = await readApiJson(new Response(JSON.stringify({success:true,data:{member_id:id},associations:{}})))
  expect(associationLabel(empty.data,'member',id,'zh-CN')).toBe('关联记录当前不可用。')
  const failed = await readApiJson(new Response(JSON.stringify({success:true,data:{member_id:id},association_error:true})))
  expect(failed.success).toBe(true)
  expect(associationLabel(failed.data,'member',id,'en')).not.toContain(id)
  expect(associationLabel(failed.data,'member',id,'en')).toBe('Unable to load associated information.')
 })
 it('does not turn a confirmed write into a failed write when display metadata is malformed', async () => {
  const result = await readApiJson(new Response(JSON.stringify({success:true,data:{member_id:id,audit_log_id:role},associations:{[`member:${id}`]:{name:123}}})))
  expect(result.data.audit_log_id).toBe(role)
  expect(result.success).toBe(true)
  expect(associationLabel(result.data,'member',id,'en')).toBe('Unable to load associated information.')
 })
})
