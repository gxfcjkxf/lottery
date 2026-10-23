const ref=n=>({$ref:`#/components/schemas/${n}`}),text={type:'string'},uuid=ref('UUID'),obj=p=>({type:'object',properties:p,required:Object.keys(p),additionalProperties:false});
const identity=obj({agent_account_id:uuid,brand_id:uuid,username:text});
export const schemas={
 AgentPasswordInput:obj({current_password:{type:'string',writeOnly:true},new_password:{type:'string',minLength:6,maxLength:30,writeOnly:true}}),
 AgentPasswordReceipt:obj({audit_log_id:uuid}),
 AgentCommissionPosting:obj({id:uuid,created_at:ref('DateTime'),entry_type:text,points:ref('Int64String')}),
 AgentCommissionPage:obj({items:{type:'array',items:ref('AgentCommissionPosting')},total_count:ref('NonnegativeInt64String'),limit:{type:'integer'},offset:{type:'integer'},credited_points:ref('NonnegativeInt64String'),debited_points:ref('NonnegativeInt64String')}),
 AgentMemberIdentity:obj({id:uuid,username:{type:['string','null']},display_name:text,phone:text}),
 AgentLoginInput:obj({username:{type:'string',pattern:'^[a-z][a-z0-9_]{2,31}$'},password:{type:'string',minLength:6,maxLength:30,writeOnly:true}}),
 AgentAuthentication:obj({agent_account_id:uuid,brand_id:uuid,username:text,access_token:{type:'string',const:'',description:'Browser sessions use a brand-scoped HttpOnly agent cookie.'},expires_at:ref('DateTime')}),
 AgentIdentity:identity,
 AgentMe:obj({account:identity,agent:ref('FinanceAgentNode'),code:{type:['string','null']}}),
 AgentWallet:obj({agent_account_id:uuid,account_id:uuid,available_points:ref('NonnegativeInt64String'),commission_points:ref('NonnegativeInt64String')}),
 AgentMember:obj({id:uuid,username:{type:['string','null']},phone:text,display_name:text,status:text,joined_at:ref('DateTime'),available_points:ref('NonnegativeInt64String'),agent_id:uuid,agent_username:text,is_direct:{type:'boolean'}}),
 AgentLineSummary:obj(Object.fromEntries(['member_count','available_points','bet_count','stake_points','unsettled_points','net_points'].map(k=>[k,{type:'string',pattern:k==='net_points'?'^-?[0-9]+$':'^[0-9]+$'}]))),
 AgentMemberPage:obj({scope:{type:'string',enum:['direct','line']},q:text,summary:ref('AgentLineSummary'),items:{type:'array',items:ref('AgentMember')},total_count:ref('NonnegativeInt64String'),limit:{type:'integer'},offset:{type:'integer'}}),
 AgentMemberSummary:obj({member:ref('AgentMemberIdentity'),wallet:ref('FinanceWallet'),unsettled_points:ref('NonnegativeInt64String'),net_points:ref('Int64String')}),
 AgentLogout:obj({logged_out:{type:'boolean'}})
};
const op=(method,path,operationId,data,extra={})=>({method,path:'/api/v1/agent'+path,operationId,summary:operationId,tag:'agency',auth:'agent',data,description:'Independent brand-scoped agent credentials and sessions; member and administrator sessions are rejected. Member reads accept scope=direct (default) or line (all descendants). The list and totals share the same scope and search, independent of pagination; each member and order is counted once. Descendant details and bet history require scope=line. All levels use the same portal. Child writes require the parent development grant and retain depth, mode and ratio limits.',...extra});
const scope={name:'scope',in:'query',schema:{type:'string',enum:['direct','line'],default:'direct'}};
const page=[{name:'limit',in:'query',schema:{type:'integer',minimum:1,maximum:100}},{name:'offset',in:'query',schema:{type:'integer',minimum:0}}];
export const operations=[
 op('POST','/login','agentLogin',ref('AgentAuthentication'),{auth:'public',requestBody:ref('AgentLoginInput')}),
 op('POST','/logout','agentLogout',ref('AgentLogout'),{requestBody:ref('EmptyObject')}),
 op('GET','/me','getAgentIdentity',ref('AgentMe')),
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
