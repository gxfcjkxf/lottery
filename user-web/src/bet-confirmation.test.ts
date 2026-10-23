import {describe,it,expect} from 'vitest'
import {standardQuoteChanged,selectionReady} from './bet-confirmation'
import type {RuleDefinition,RuleTicketSelection} from '../../shared/src/rules'
import type {BetQuote} from './betting-api'
describe('standard betting confirmation',()=>{
it('does not quote incomplete selections while the user is still choosing',()=>{
  const definition={model:{model:'DIGITS_0_9',length:3},selection:{mode:'numbers'}} as RuleDefinition
  const ticket:RuleTicketSelection={regular:[],special:[],digits:[[1],[],[]],exclude:[],attributes:{},features:{}}
  expect(selectionReady(definition,ticket)).toBe(false)
  expect(selectionReady(definition,{...ticket,digits:[[1],[2],[3]]})).toBe(true)
  const regular={model:{model:'X_PLUS_Y'},selection:{mode:'numbers',regular_count:2,special_count:1}} as RuleDefinition
  expect(selectionReady(regular,{...ticket,regular:[1,2],special:[]})).toBe(false)
  expect(selectionReady(regular,{...ticket,regular:[1,2],special:[3]})).toBe(true)
 })
it('compares server-calculated standard stakes, selection and the quoted rule version',()=>{
  const value={bet_points:'9007199254740993',unit_points:'1',multiplier:'1',combination_count:1,period_id:'p',play_id:'play',rule_version_id:'v',definition_hash:'hash',normalized:{digits:[[1]]},policy_versions:{brand:1,game:1}} as BetQuote
  expect(standardQuoteChanged(value,structuredClone(value))).toBe(false)
  expect(standardQuoteChanged(value,{...value,bet_points:'9007199254740994'})).toBe(true)
  expect(standardQuoteChanged(value,{...value,definition_hash:'new'})).toBe(true)
 })
})
