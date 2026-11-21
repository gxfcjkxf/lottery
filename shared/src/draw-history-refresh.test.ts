import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createRenderer, defineComponent } from 'vue'
import { useDrawHistoryRefresh } from './draw-history-refresh'
const renderer = createRenderer({patchProp(){},insert(){},remove(){},createElement:()=>({}),createText:()=>({}),createComment:()=>({}),setText(){},setElementText(){},parentNode:()=>null,nextSibling:()=>null})
beforeEach(() => {
 vi.useFakeTimers()
 vi.stubGlobal('document', Object.assign(new EventTarget(), {visibilityState:'visible'}))
})
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals() })
it('pauses historical, hidden and in-flight reads, refreshes on return, and cleans up', async () => {
 let enabled=true, busy=false
 const refresh=vi.fn(async()=>{})
 const app=renderer.createApp(defineComponent({setup(){useDrawHistoryRefresh(()=>enabled,()=>busy,refresh);return ()=>null}}))
 app.mount({})
 try {
  await vi.advanceTimersByTimeAsync(5000)
  expect(refresh).toHaveBeenCalledTimes(1)
  busy=true
  await vi.advanceTimersByTimeAsync(10000)
  expect(refresh).toHaveBeenCalledTimes(1)
  busy=false;enabled=false
  await vi.advanceTimersByTimeAsync(5000)
  expect(refresh).toHaveBeenCalledTimes(1)
  enabled=true;Object.assign(document,{visibilityState:'hidden'})
  await vi.advanceTimersByTimeAsync(10000)
  expect(refresh).toHaveBeenCalledTimes(1)
  Object.assign(document,{visibilityState:'visible'})
  document.dispatchEvent(new Event('visibilitychange'))
  expect(refresh).toHaveBeenCalledTimes(2)
 } finally { app.unmount() }
 await vi.advanceTimersByTimeAsync(10000)
 document.dispatchEvent(new Event('visibilitychange'))
 expect(refresh).toHaveBeenCalledTimes(2)
})
