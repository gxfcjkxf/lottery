const ref=name=>({$ref:`#/components/schemas/${name}`});
const amounts={type:'array',maxItems:5,items:{type:'string',pattern:'^[1-9][0-9]{0,5}$',maxLength:6,description:'Quick stake points from 1 to 999999.'}};
export const schemas={
 MemberQuickAmountsInput:{type:'object',additionalProperties:false,required:['quick_amounts'],properties:{quick_amounts:amounts}},
 MemberQuickAmounts:{type:'object',additionalProperties:false,required:['brand_id','member_id','quick_amounts'],properties:{brand_id:ref('UUID'),member_id:ref('UUID'),quick_amounts:amounts}},
};
export const operations=[
 {method:'GET',path:'/api/v1/me/betting-preferences',operationId:'memberQuickAmountsRead',summary:'Read account quick stake amounts',tag:'identity',auth:'user',data:ref('MemberQuickAmounts'),description:'Current authenticated brand member only. Stored in the database independently of device or login session. Starts as an empty array; no query filters.'},
 {method:'PUT',path:'/api/v1/me/betting-preferences',operationId:'memberQuickAmountsSave',summary:'Replace account quick stake amounts',tag:'identity',auth:'user',data:ref('MemberQuickAmounts'),requestBody:ref('MemberQuickAmountsInput'),description:'Idempotently replace the current member list with zero to five integer strings from 1 to 999999. Identity comes only from the authenticated brand session; accepts no member or brand selectors. Repeating the same replacement has the same result. Last successful save wins; does not modify wallets, bets, profile data, or other member settings.'},
];
