import {describe,it,expect} from 'vitest'
import {calendarWeeks,shiftCalendarMonth} from './official-calendar'
describe('official monthly calendar',()=>{
 it('lays out the entire month Sunday first, including adjacent dates',()=>{
  const weeks=calendarWeeks('2026-10')
  expect(weeks).toHaveLength(5);expect(weeks[0]!.map(day=>day.day)).toEqual([27,28,29,30,1,2,3])
  expect(weeks.flat().filter(day=>day.inMonth)).toHaveLength(31)
  expect(weeks[4]![6]).toEqual({date:'2026-10-31',day:31,inMonth:true})
 })
 it('keeps leap days and crosses annual month boundaries',()=>{
  expect(calendarWeeks('2028-02').flat().filter(day=>day.inMonth)).toHaveLength(29)
  expect(shiftCalendarMonth('2026-12',1)).toBe('2027-01')
  expect(shiftCalendarMonth('2027-01',-1)).toBe('2026-12')
 })
})
