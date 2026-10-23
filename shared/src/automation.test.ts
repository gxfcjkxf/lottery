import {describe,expect,it} from 'vitest'
import {automationCatalog} from './automation'
import {createCatalogI18n} from './localization'
describe('draw source messages',()=>{
 it('provides both languages for source modes, schedule states and errors',()=>{
  expect(Object.keys(automationCatalog.en).sort()).toEqual(Object.keys(automationCatalog['zh-CN']).sort())
  for(const locale of ['en','zh-CN'] as const){const i18n=createCatalogI18n(automationCatalog,locale);for(const key of ['hongKongSource','retiredSource','viewSource','sourceSchedule','ready','waiting_result','invalid_result','schedule_conflict','attributes_missing'] as const){expect(i18n.t(key).trim()).not.toBe('');expect(i18n.t(key)).not.toBe(key)}}
 })
})
