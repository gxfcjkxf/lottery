import type { AgentNode } from '../../admin-web/src/agents-api'
export interface Member{id:string;username:string;phone:string;display_name:string;status:string;joined_at:string;available_points:string;agent_username:string;is_direct:boolean}
export interface Page<T>{items:T[];total_count:string;limit:number;offset:number}
export type MemberScope='direct'|'line'
export interface ScopeSummary{member_count:string;available_points:string;bet_count:string;stake_points:string;unsettled_points:string;net_points:string}
export interface MemberPage extends Page<Member>{summary:ScopeSummary}
export interface Me{account:{agent_account_id:string;username:string};agent:AgentNode;code:string|null}
export interface Wallet{available_points:string;commission_points:string}
export type Bet = import("../../shared/src/order-history").OrderHistoryRecord
