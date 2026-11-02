export interface ManualRechargeIntent {brand:string;body:{member_id:string;points:string;remark:string;reason:string};key:string}
export const pendingManualRecharges = new Map<string,ManualRechargeIntent>()
