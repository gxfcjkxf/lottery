import { describe,expect,it } from 'vitest'
import {archiveNumbers,specialFacts,totalFacts} from './draw-archive-presentation'
describe('compact draw history',()=>{
 it('preserves digit order and zeroes and separates the special group',()=>{
  expect(archiveNumbers({regular:[],special:[],digits:[0,1,0]}).map(n=>n.value)).toEqual([0,1,0])
  expect(archiveNumbers({regular:[3],special:[7],digits:[]})[1]).toEqual({value:7,special:true})
 })
 it('uses current Mark Six size, digit-sum and tail boundaries including 49 pushes',()=>{
  expect(specialFacts({regular:[],special:[25],digits:[]},'special')).toEqual(['odd','big'])
  expect(specialFacts({regular:[],special:[16],digits:[]},'tail')).toEqual(['sum_big','sum_odd','tail_big','even'])
  expect(specialFacts({regular:[],special:[49],digits:[]},'tail')).toEqual(['push','push','push','push'])
 })
 it('weights the special ball by three for total wave and never invents missing attributes',()=>{
  const result={regular:[1,2,3,4,5,6],special:[7],digits:[]}
  const attributes=Object.fromEntries([1,2,3,4,5,6,7].map(n=>[String(n).padStart(2,'0'),{wave:n<=3?'blue':'red',zodiac:'rat',element:'wood'}]))
  expect(totalFacts(result,attributes)).toEqual({total:28,parity:'even',size:'small',wave:'red'})
  expect(totalFacts(result,{}).wave).toBeNull()
 })
})

import {displayedArchiveNumbers,numberFacts,zodiacFamily} from './draw-archive-presentation'
it('sorts only the regular balls and keeps the special last without mutating saved data',()=>{
 const result={regular:[9,25,26,28,44,45],special:[20],digits:[]},order=[44,9,25,26,28,45]
 expect(displayedArchiveNumbers(result,order,'drawn')!.map(n=>n.value)).toEqual([44,9,25,26,28,45,20])
 expect(displayedArchiveNumbers(result,order,'size')!.map(n=>n.value)).toEqual([9,25,26,28,44,45,20])
 expect(result.regular).toEqual([9,25,26,28,44,45]);expect(order).toEqual([44,9,25,26,28,45])
 expect(displayedArchiveNumbers(result,null,'drawn')).toBeNull()
 expect(displayedArchiveNumbers(result,null,'size')).toHaveLength(7)
})
it('computes independent per-number facts and uses saved zodiac for animal classification',()=>{
 expect(numberFacts(49)).toEqual({size:'big',parity:'odd',sumParity:'odd'})
 expect(numberFacts(24)).toEqual({size:'small',parity:'even',sumParity:'even'})
 expect(zodiacFamily('pig')).toBe('domestic');expect(zodiacFamily('snake')).toBe('wild');expect(zodiacFamily('')).toBeNull()
})
