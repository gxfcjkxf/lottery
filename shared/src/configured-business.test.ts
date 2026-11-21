import { expect, it, vi } from 'vitest'
import { createConfiguredBusinessClient } from './configured-business'

it('submits the whole selection once and returns all independent order receipts', async () => {
  const input = { period_id:'period', rule_id:'rule', sales_version:1, snapshot_hash:'snapshot', actor_context:'actor', requests:[{play_id:'numbers', options:['01','02'], plate:'A', points:'10', multiplier:'1'}] }
  const items = [{id:'first', quote:{lines:[{options:['01']}]}}, {id:'second', quote:{lines:[{options:['02']}]}}]
  const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({success:true,data:{items}}),{status:201}))
  const receipt = await createConfiguredBusinessClient('user','brand','aurora',fetcher).place(input,'batch-idempotency-key')
  expect(receipt.items).toEqual(items)
  expect(fetcher).toHaveBeenCalledOnce()
  const [url, init] = fetcher.mock.calls[0]!
  expect(url).toBe('/api/v1/b/aurora/configured/orders')
  expect(JSON.parse(init!.body as string)).toEqual(input)
  expect(new Headers(init!.headers).get('Idempotency-Key')).toBe('batch-idempotency-key')
})

it('uses the current Canada groups for every historical option without changing the purchase snapshot', async () => {
 const {readFileSync}=await import('node:fs')
 const {configuredDemo}=await import('./configured-rules')
 const {configuredTicketLabel}=await import('./configured-business')
 const {configuredSelectionGroups,configuredSelectionOptionLabel}=await import('./configured-selection-groups')
 for(const name of ['canada_28','canada_996','canada_vip','canada_42','canada_web','canada_50','canada_baccarat','canada_niuniu']){
  const snapshot=configuredDemo(JSON.parse(readFileSync(new URL(`../../backend/internal/rules/config/${name}.json`,import.meta.url),'utf8'))),before=JSON.stringify(snapshot)
  for(const group of configuredSelectionGroups(snapshot))for(const play of group.plays){
   const option=snapshot.catalog.domains[play.selection.domain]!.options[0]!
   for(const locale of ['en','zh-CN'] as const)expect(configuredTicketLabel(snapshot,{play_id:play.id,options:[option.id]},locale)).toBe(`${group.label[locale]} · ${configuredSelectionOptionLabel(play,option)[locale]}`)
  }
  expect(JSON.stringify(snapshot)).toBe(before)
 }
 const snapshot=configuredDemo(JSON.parse(readFileSync(new URL('../../backend/internal/rules/config/canada_28.json',import.meta.url),'utf8')))
 expect(configuredTicketLabel(snapshot,{play_id:'sum_24',options:['sum_24']},'zh-CN')).toBe('总和 · 24')
 expect(configuredTicketLabel(snapshot,{play_id:'big',options:['big']},'zh-CN')).toBe('两面 · 大')
 expect(configuredTicketLabel(snapshot,{play_id:'tail_big',options:['tail_big']},'en')).toBe('Tails · Big')
})
