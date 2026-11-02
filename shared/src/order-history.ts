import type { ConfiguredLiveOrder } from './configured-business'
export interface OrderHistoryRecord {id:string;game_name:string;period_no:string;placed_at:string;kind:'configured';order:ConfiguredLiveOrder}
