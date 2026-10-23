import { createApiClient } from '../../shared/src/api'
export interface MemberBettingSummary {brand_id:string;member_id:string;unsettled_points:string;today_net_points:string}
export async function readMemberSummary(brandCode?:string,signal?:AbortSignal):Promise<MemberBettingSummary>{
 const base=brandCode?`/api/v1/b/${encodeURIComponent(brandCode)}`:'/api/v1'
 const value=await createApiClient({brandCode}).request<MemberBettingSummary>(`${base}/betting-summary`,{signal})
 if(!value||typeof value.brand_id!=='string'||typeof value.member_id!=='string'||typeof value.unsettled_points!=='string'||typeof value.today_net_points!=='string'||!/^(0|[1-9]\d*)$/.test(value.unsettled_points)||!/^(0|-?[1-9]\d*)$/.test(value.today_net_points))throw new Error('Invalid member betting summary')
 return value
}
