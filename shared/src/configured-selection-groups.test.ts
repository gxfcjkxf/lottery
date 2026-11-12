import {readFileSync} from 'node:fs'
import {describe,it,expect} from 'vitest'
import {configuredDemo} from './configured-rules'
import {configuredSelectionGroups,configuredSelectionOptionLabel,configuredSelectionRequests} from './configured-selection-groups'
const snapshot=(name:string)=>configuredDemo(JSON.parse(readFileSync(new URL(`../../backend/internal/rules/config/${name}.json`,import.meta.url),'utf8')))
describe('configured selection presentation groups',()=>{
 it('combines all 57 Canada28 entries into four groups without changing rules or identifiers',()=>{
  const data=snapshot('canada_28'),before=JSON.stringify(data),groups=configuredSelectionGroups(data)
  expect(groups.map(g=>g.label['zh-CN'])).toEqual(['两面','组合','总和','尾数'])
  expect(groups.map(g=>g.plays.length)).toEqual([10,3,28,16])
  expect(new Set(groups.flatMap(g=>g.plays.map(p=>p.id)))).toEqual(new Set(data.catalog.plays.map(p=>p.id)))
  expect(groups[0]!.plays[3]).toBe(data.catalog.plays[3])
  expect(groups[3]!.plays.slice(0,8).map(p=>p.id)).toEqual(['tail_1','tail_2','tail_3','tail_4','tail_5','tail_6','tail_7','tail_8'])
  const labels=groups[3]!.plays.slice(8).map(p=>configuredSelectionOptionLabel(p,data.catalog.domains[p.selection.domain]!.options[0]!))
  expect(labels.map(label=>label['zh-CN'])).toEqual(['大','小','单','双','大单','大双','小单','小双'])
  expect(labels.map(label=>label.en)).toEqual(['Big','Small','Odd','Even','Big odd','Big even','Small odd','Small even'])
  expect(JSON.stringify(data)).toBe(before)
 })
 it('builds independent lines for mixed selections at the entered stake and plate',()=>{
  const data=snapshot('canada_28'),request={play_id:'big',options:[],plate:'B',points:'0.25',multiplier:'1'}
  const requests=configuredSelectionRequests(data,request,['big','extreme_small','sum_14','tail_1'])
  expect(requests).toEqual(['big','extreme_small','sum_14','tail_1'].map(id=>({...request,play_id:id,options:[id]})))
  expect(request.options).toEqual([])
  expect(configuredSelectionRequests(data,request,[])).toEqual([])
  expect(()=>configuredSelectionRequests(data,request,['missing'])).toThrow('Invalid independent selection')
 })
 it('shows sum options as numbers in both languages without rewriting their source labels',()=>{
  const data=snapshot('canada_28')
  for(const play of data.catalog.plays.filter(p=>p.group==='sums')){
   const option=data.catalog.domains[play.selection.domain]!.options[0]!,before=JSON.stringify(option)
   const label=configuredSelectionOptionLabel(play,option)
   expect(label).toEqual({en:play.id.slice(4),'zh-CN':play.id.slice(4)})
   expect(JSON.stringify(option)).toBe(before)
  }
 })
 it('retains individual entries for plays with their own multi-option selection rules',()=>{
  const data=snapshot('mark_six_v2'),groups=configuredSelectionGroups(data)
  expect(groups.flatMap(g=>g.plays.map(p=>p.id))).toEqual(data.catalog.plays.map(p=>p.id))
  for(const p of data.catalog.plays.filter(p=>data.catalog.domains[p.selection.domain]!.options.length>1))expect(groups.find(g=>g.plays.includes(p))!.plays).toEqual([p])
 })
 it('keeps custom group names and unit-multiplier plays individually addressable',()=>{
  const data=snapshot('canada_28');data.catalog.plays=data.catalog.plays.slice(0,2)
  for(const p of data.catalog.plays)p.group='operator_custom'
  expect(configuredSelectionGroups(data).map(g=>g.label)).toEqual(data.catalog.plays.map(p=>p.label))
  for(const p of data.catalog.plays){p.group='sides';p.stake_mode='unit_multiplier'}
  expect(configuredSelectionGroups(data)).toHaveLength(2)
 })
})
