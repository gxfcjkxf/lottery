import {describe,expect,it} from 'vitest'
import {clearStandardIntent,readStandardIntent,saveStandardIntent,type StandardBetReview} from './standard-bet-intent'
describe('standard bet frozen retry',()=>{
 it('survives game unmount with the exact body and key, separate for each membership',()=>{
  const data=new Map<string,string>(),storage={getItem:(key:string)=>data.get(key)??null,setItem:(key:string,value:string)=>{data.set(key,value)},removeItem:(key:string)=>{data.delete(key)}}
  const actor={userId:'user',memberId:'member',brandId:'brand'}
  const review={actor,key:'immutable-key',body:{actor_context:'actor-snapshot',period_id:'period',multiplier:'9007199254740993'},quote:{period_id:'period',period:{game_id:'game'}},definition:{schema_version:1}} as StandardBetReview
  saveStandardIntent(storage,'game',review)
  const before=[...data.values()][0]
  expect(readStandardIntent(storage,actor,'game')).toEqual(review)
  expect(readStandardIntent(storage,{...actor,brandId:'another-brand'},'game')).toBeNull()
  expect(readStandardIntent(storage,actor,'other-game')).toBeNull()
  expect([...data.values()][0]).toBe(before)
  clearStandardIntent(storage,'game',review);expect(readStandardIntent(storage,actor,'game')).toBeNull()
 })
 it('does not treat corrupted pending data as a new empty bet',()=>{
  const actor={userId:'user',memberId:'member',brandId:'brand'}
  expect(()=>readStandardIntent({getItem:()=>'{bad',setItem:()=>{},removeItem:()=>{}},actor,'game')).toThrow()
  expect(()=>readStandardIntent({getItem:()=>JSON.stringify({actor}),setItem:()=>{},removeItem:()=>{}},actor,'game')).toThrow()
 })
})
