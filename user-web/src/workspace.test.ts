import {createSSRApp} from 'vue'
import {renderToString} from 'vue/server-renderer'
import {afterEach,describe,expect,it,vi} from 'vitest'
import UserPeriodStrip from './UserPeriodStrip.vue'
import {createUserI18n,userI18nKey} from './i18n'
import {setPlatformDisplaySettings} from '@lottery/shared'
import {rememberGame,recalledGame,gamePath,type UserGame} from './user-games'
import {readMemberSummary} from './member-summary-api'
afterEach(()=>vi.unstubAllGlobals())
describe('game workspace',()=>{
 it('remembers the game per brand and renders its current published name',()=>{
  const values=new Map<string,string>(),storage={getItem:(key:string)=>values.get(key)??null,setItem:(key:string,value:string)=>{values.set(key,value)}}
  const game:UserGame={id:'one',kind:'configured',name:'Canada'}
  expect(recalledGame(storage,'brand-a',[game])).toBeNull()
  rememberGame(storage,'brand-a',game)
  expect(recalledGame(storage,'brand-a',[{...game,name:'Renamed'}])?.name).toBe('Renamed')
  expect(recalledGame(storage,'brand-b',[game])).toBeNull()
  expect(recalledGame(storage,'brand-a',[])).toBeNull()
  expect(gamePath(game)).toBe('/configured/one')
  values.set('lottery:last-game:brand-a','invalid')
  expect(()=>recalledGame(storage,'brand-a',[game])).toThrow()
 })
 for(const locale of ['en','zh-CN'] as const)it(`uses exact result period and changes the countdown after closing (${locale})`,async()=>{
  vi.stubGlobal('navigator',{language:locale,languages:[locale]})
  setPlatformDisplaySettings({timezone:'Asia/Singapore',date_format:'DD/MM/YYYY',time_format:'hh:mm:ss A'})
  const i18n=createUserI18n();i18n.setLocale(locale)
  const render=(now:number)=>renderToString(createSSRApp(UserPeriodStrip,{previousNo:'3495516',periodNo:'3495517',closeAt:'2026-10-18T06:00:00Z',drawAt:'2026-10-18T06:01:00Z',now,clockOffset:0}).provide(userI18nKey,i18n))
  const open=await render(Date.parse('2026-10-18T05:59:51Z'))
  expect(open).toContain(locale==='en'?'Closes in':'封盘倒计时');expect(open).toContain('00:00:09')
  expect(open).toContain(`aria-label="${locale==='en'?'Closes in':'封盘倒计时'}"`)
  expect(open).not.toContain('period-results-link');expect(open).not.toContain('period-facts')
  expect(open).toMatch(/class="period-countdown"[^>]*>00:00:09<\/strong>/)
  expect(open).toContain(locale==='en'?'3495516':'3495516期');expect(open).not.toContain('上期')
  expect(open).toContain(locale==='en'?'02:00:00 PM':'02:00:00 下午')
  expect(open).toContain('18/10/2026')
  const closed=await render(Date.parse('2026-10-18T06:00:41Z'))
  expect(closed).toContain(locale==='en'?'Draws in':'开奖倒计时');expect(closed).toContain('00:00:19')
 })
 it('requests exact brand-scoped integer aggregates and rejects numeric money',async()=>{
  const result={brand_id:'brand',member_id:'member',unsettled_points:'9007199254740993',today_net_points:'-20'}
  const fetcher=vi.fn().mockResolvedValue(new Response(JSON.stringify({success:true,data:result})))
  vi.stubGlobal('fetch',fetcher)
  expect(await readMemberSummary('my brand')).toEqual(result)
  expect(fetcher.mock.calls[0][0]).toBe('/api/v1/b/my%20brand/betting-summary')
  fetcher.mockResolvedValue(new Response(JSON.stringify({success:true,data:{...result,unsettled_points:10}})))
  await expect(readMemberSummary()).rejects.toThrow('Invalid member betting summary')
 })
})
