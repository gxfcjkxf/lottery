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
