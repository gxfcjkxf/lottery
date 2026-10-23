import type {BetInput,BetQuote} from './betting-api'
import type {RuleDefinition} from '../../shared/src/rules'
export interface StandardBetReview {body:Readonly<BetInput>;quote:BetQuote;key:string;actor:{userId:string;memberId:string;brandId:string};definition:RuleDefinition}
type Store=Pick<Storage,'getItem'|'setItem'|'removeItem'>
export const standardIntentKey=(actor:StandardBetReview['actor'],game:string)=>`lottery.standard.intent:${actor.brandId}:${actor.userId}:${actor.memberId}:${game}`
export function saveStandardIntent(storage:Store,game:string,review:StandardBetReview){storage.setItem(standardIntentKey(review.actor,game),JSON.stringify(review))}
export function readStandardIntent(storage:Store,actor:StandardBetReview['actor'],game:string):StandardBetReview|null{
 const raw=storage.getItem(standardIntentKey(actor,game));if(raw===null)return null
 const value=JSON.parse(raw) as StandardBetReview
 if(!value||value.actor?.brandId!==actor.brandId||value.actor.userId!==actor.userId||value.actor.memberId!==actor.memberId||typeof value.key!=='string'||!value.key||!value.body?.actor_context||!value.body.period_id||value.body.period_id!==value.quote?.period_id||!value.definition||value.quote.period?.game_id!==game)throw new Error('Invalid saved bet intent')
 return value
}
export function clearStandardIntent(storage:Store,game:string,review:StandardBetReview){storage.removeItem(standardIntentKey(review.actor,game))}
