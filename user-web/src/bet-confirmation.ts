import type {BetQuote} from './betting-api'
import type {RuleDefinition,RuleTicketSelection} from '../../shared/src/rules'

export function selectionReady(definition:RuleDefinition,selection:RuleTicketSelection){
 const rule=definition.selection
 switch(rule.mode){
  case 'numbers':return definition.model.model==='DIGITS_0_9'?Array.from({length:definition.model.length},(_,index)=>selection.digits?.[index]?.length??0).every(count=>count>0):(selection.regular?.length??0)>=rule.regular_count&&(selection.special?.length??0)>=rule.special_count
  case 'exclude':return (selection.exclude?.length??0)===rule.exclude_count
  case 'attributes':return (rule.attribute_groups??[]).every(group=>(selection.attributes?.[group]?.length??0)>0)
  case 'features':return Object.keys(rule.feature_choices??{}).every(name=>(selection.features?.[name]?.length??0)>0)
  default:throw new Error('Unsupported selection mode')
 }
}

export function standardQuoteChanged(before:BetQuote,after:BetQuote){
 return ['bet_points','unit_points','multiplier','combination_count','period_id','play_id','rule_version_id','definition_hash'].some(key=>before[key as keyof BetQuote]!==after[key as keyof BetQuote])||JSON.stringify(before.normalized)!==JSON.stringify(after.normalized)||JSON.stringify(before.policy_versions)!==JSON.stringify(after.policy_versions)
}
