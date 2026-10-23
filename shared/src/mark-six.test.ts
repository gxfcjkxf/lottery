import {describe,it,expect} from 'vitest'
import {createCatalogI18n} from './localization'
import {markSixWave,markSixCatalog,markSixSelectionLabel} from './mark-six'
describe('Mark Six versioned attributes',()=>{
 it('uses the supplied period snapshot rather than a fixed mapping',()=>{expect(markSixWave(1,{'01':'red'})).toBe('red');expect(markSixWave(1,{'01':'blue'})).toBe('blue');expect(()=>markSixWave(1,{})).toThrow();expect(()=>markSixWave(50,{})).toThrow()})
 it('renders historical combination and parlay labels from frozen selections',()=>{const {t}=createCatalogI18n(markSixCatalog,'zh-CN');expect(markSixSelectionLabel('zodiac_pair',[1,12],t)).toBe('鼠 · 猪');expect(markSixSelectionLabel('parlay',[100,201],t)).not.toContain('{')})
 it('has both supported translations'  ,()=>{expect(Object.keys(markSixCatalog.en).sort()).toEqual(Object.keys(markSixCatalog['zh-CN']).sort())})
})
