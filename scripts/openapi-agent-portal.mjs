const ref=n=>({$ref:`#/components/schemas/${n}`}),text={type:'string'},uuid=ref('UUID'),obj=p=>({type:'object',properties:p,required:Object.keys(p),additionalProperties:false});
const identity=obj({agent_account_id:uuid,brand_id:uuid,username:text});
export const schemas={
 AgentWalletRecord:obj({id:uuid,kind:{type:'string',enum:['deposit','withdrawal','to_points','from_points','transfer']},created_at:ref('DateTime'),order:{anyOf:[ref('CashOrder'),{type:'null'}]},transfer:{anyOf:[ref('AgentTransferRecord'),{type:'null'}]}}),
 AgentWalletRecordPage:obj({items:{type:'array',items:ref('AgentWalletRecord')},total_count:ref('NonnegativeInt64String'),limit:{type:'integer'},offset:{type:'integer'}}),
 AgentTransferRecord:obj({id:uuid,member_id:uuid,member_account:{type:['string','null']},amount:ref('NonnegativePoints'),created_at:ref('DateTime')}),
 AgentTransferPage:obj({items:{type:'array',items:ref('AgentTransferRecord')},total_count:ref('NonnegativeInt64String'),limit:{type:'integer'},offset:{type:'integer'}}),
 AgentTransferInput:obj({member_id:uuid,amount:ref('NonnegativePoints')}),
 AgentTransferReceipt:obj({id:uuid,member_id:uuid,amount:ref('NonnegativePoints')}),
 AgentPasswordInput:obj({current_password:{type:'string',writeOnly:true},new_password:{type:'string',minLength:6,maxLength:30,writeOnly:true}}),
 AgentPasswordReceipt:obj({audit_log_id:uuid}),
 AgentCommissionPosting:obj({id:uuid,created_at:ref('DateTime'),entry_type:text,points:ref('Points')}),
 AgentCommissionPage:obj({items:{type:'array',items:ref('AgentCommissionPosting')},total_count:ref('NonnegativeInt64String'),limit:{type:'integer'},offset:{type:'integer'},credited_points:ref('NonnegativePoints'),debited_points:ref('NonnegativePoints')}),
 AgentMemberIdentity:obj({id:uuid,username:{type:['string','null']},display_name:text,phone:text}),
 AgentLoginInput:{...obj({captcha_id:ref("UUID"),captcha_answer:{type:"string",maxLength:128},username:{type:'string',pattern:'^[a-z][a-z0-9_]{2,31}$'},password:{type:'string',minLength:6,maxLength:30,writeOnly:true}}),required:['username','password']},
 AgentAuthentication:obj({agent_account_id:uuid,brand_id:uuid,username:text,access_token:{type:'string',const:'',description:'Browser sessions use an HttpOnly agent cookie scoped to the management entry.'},expires_at:ref('DateTime')}),
 AgentIdentity:identity,
 AgentMe:obj({account:identity,agent:ref('FinanceAgentNode'),code:{type:['string','null']},user_origin:{type:['string','null'],format:'uri'},brand_code:text}),
 AgentWallet:obj({agent_account_id:uuid,account_id:uuid,available_points:ref('NonnegativePoints'),commission_points:ref('NonnegativePoints')}),
 AgentMember:obj({id:uuid,username:{type:['string','null']},phone:text,display_name:text,status:text,joined_at:ref('DateTime'),available_points:ref('NonnegativePoints'),agent_id:uuid,agent_username:text,is_direct:{type:'boolean'}}),
 AgentLineSummary:obj(Object.fromEntries(['member_count','available_points','bet_count','stake_points','unsettled_points','net_points'].map(k=>[k,{type:'string',pattern:k==='net_points'?'^-?[0-9]+$':'^[0-9]+$'}]))),
 AgentMemberPage:obj({scope:{type:'string',enum:['direct','line']},q:text,summary:ref('AgentLineSummary'),items:{type:'array',items:ref('AgentMember')},total_count:ref('NonnegativeInt64String'),limit:{type:'integer'},offset:{type:'integer'}}),
 AgentMemberSummary:obj({member:ref('AgentMemberIdentity'),wallet:ref('FinanceWallet'),unsettled_points:ref('NonnegativePoints'),net_points:ref('Points')}),
 AgentLogout:obj({logged_out:{type:'boolean'}})
};
const op=(method,path,operationId,data,extra={})=>({method,path:'/api/v1/agent'+path,operationId,summary:operationId,tag:'agency',auth:'agent',data,description:'Fixed central agent entry; globally unique usernames identify the account brand. Central login uses platform CAPTCHA settings. Authenticated data remains brand-scoped; member and administrator sessions are rejected. Member reads accept scope=direct (default) or line (all descendants). The list and totals share the same scope and search, independent of pagination; each member and order is counted once. Descendant details and bet history require scope=line. All levels use the same portal. Child writes require the parent development grant and retain depth, mode and ratio limits.',...extra});
const scope={name:'scope',in:'query',schema:{type:'string',enum:['direct','line'],default:'direct'}};
const page=[{name:'limit',in:'query',schema:{type:'integer',minimum:1,maximum:100}},{name:'offset',in:'query',schema:{type:'integer',minimum:0}}];
export const operations=[
 op('GET','/auth/config','getAgentLoginConfig',obj({captcha_enabled:{type:'boolean'}}),{auth:'public'}),
 op('GET','/auth/challenge','getAgentLoginCaptcha',ref('IdentityChallenge'),{auth:'public'}),
 op('POST','/login','agentLogin',ref('AgentAuthentication'),{auth:'public',requestBody:ref('AgentLoginInput')}),
 op('POST','/logout','agentLogout',ref('AgentLogout'),{requestBody:ref('EmptyObject')}),
 op('GET','/me','getAgentIdentity',ref('AgentMe')),
 op('GET','/wallet/records','listAgentWalletRecords',ref('AgentWalletRecordPage'),{parameters:page,description:'Own currency orders and point transfers paged together by created_at descending, then id descending.'}),
 op('GET','/transfers','listAgentTransfers',ref('AgentTransferPage'),{parameters:page}),
 op('POST','/transfers','agentTransferPoints',ref('AgentTransferReceipt'),{requestBody:ref('AgentTransferInput'),idempotency:true,description:'One-way transfer from agent available points to a member in this agent line and brand. The member receives recharge points; frozen funds cannot be transferred. Both ledger entries commit atomically.'}),
 op('GET','/wallet','getAgentWallet',ref('AgentWallet')),
 op('GET','/commission','getAgentCommission',ref('AgentCommissionPage'),{parameters:page}),
 op('POST','/password','changeAgentPassword',ref('AgentPasswordReceipt'),{requestBody:ref('AgentPasswordInput'),idempotency:true,description:'Verifies the current password; atomically updates this brand-scoped agent credential, writes a redacted audit event and revokes all its agent sessions. Sign in again after success.'}),
 op('GET','/children','listAgentChildren',ref('FinanceAgentTree'),{parameters:page}),
 op('POST','/children','createAgentChild',ref('FinanceAgentNode'),{requestBody:ref('FinanceAgentCreateInput'),idempotency:true,successStatus:201}),
 op('PUT','/children/{id}/config','updateAgentChild',ref('FinanceAgentNode'),{requestBody:ref('FinanceAgentChildUpdateInput'),idempotency:true}),
 op('GET','/members','listAgentMembers',ref('AgentMemberPage'),{parameters:[scope,...page,{name:'q',in:'query',schema:text}]}),
 op('POST','/members','createAgentMember',ref('AdminOperatorCreateResult'),{requestBody:ref('AdminOperatorCreateRequest'),idempotency:true,successStatus:201}),
 op('GET','/members/{id}/summary','getAgentMemberSummary',ref('AgentMemberSummary'),{parameters:[scope]}),
 op('GET','/members/{id}/bets','getAgentMemberBets',ref('MemberBettingHistoryPage'),{parameters:[scope,...page.filter(p=>p.name==='limit'),{name:'cursor',in:'query',schema:text},{name:'game_id',in:'query',schema:uuid}]})
];
