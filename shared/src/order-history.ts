import type { RuleTicketSelection } from './rules'
import type { ConfiguredLiveOrder } from './configured-business'
import type { MarkSixOrder } from './mark-six'
import type { CanadaOrder } from './canada'
export interface StandardHistoryOrder {
  status:string; total_points:string; multiplier:string; prize_points:string
  settled_at?:string|null; payout_entry_id?:string|null; selection_normalized:RuleTicketSelection
}
export type OrderHistoryRecord = {id:string;game_name:string;period_no:string;placed_at:string} & (
 {kind:'standard';order:StandardHistoryOrder}|{kind:'configured';order:ConfiguredLiveOrder}|
 {kind:'mark-six';order:MarkSixOrder}|{kind:'canada';order:CanadaOrder})
