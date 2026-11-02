import {createSSRApp} from 'vue'
import {renderToString} from 'vue/server-renderer'
import {expect,it} from 'vitest'
import ConfiguredOrderTicket from '../../shared/src/ConfiguredOrderTicket.vue'
import {configuredDemo,type ConfiguredLiveOrder} from '@lottery/shared'
import {readFileSync} from 'node:fs'
it('configured receipts preserve exact decimal strings in both languages',async()=>{
 const snapshot=configuredDemo(JSON.parse(readFileSync('../backend/internal/rules/config/mark_six_v2.json','utf8')))
 const play=snapshot.catalog.plays[0]!,option=snapshot.catalog.domains[play.selection.domain]!.options[0]!
 const order={id:'order',period_no:'20261028',snapshot,status:'placed',quote:{bet_points:'9007199254740993',lines:[{play_id:play.id,options:[option.id],stake:'9007199254740993',prices:{hit:'2.5'}}]},result:null} as unknown as ConfiguredLiveOrder
 for(const locale of ['en','zh-CN'] as const){const html=await renderToString(createSSRApp(ConfiguredOrderTicket,{order,locale}));expect(html).toContain('9007199254740993');expect(html).not.toContain('9007199254740992')}
})
