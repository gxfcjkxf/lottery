const ref=name=>({$ref:`#/components/schemas/${name}`});
const obj = (properties, required = Object.keys(properties)) => ({ type: 'object', properties, required, additionalProperties: false });
const arr=items=>({type:'array',items});
const nullable=value=>({anyOf:[value,{type:'null'}]});
const str={type:'string'},int={type:'integer',minimum:0},bool={type:'boolean'},uuid=ref('UUID'),amount=ref('NonnegativeInt64String'),positive=ref('PositiveInt64String');
const profile={type:'string',enum:['canada_28','canada_996','canada_vip','canada_42','canada_web','canada_50','canada_baccarat','canada_niuniu']};
const odds={type:'object',additionalProperties:{type:'string',pattern:'^[0-9]+(\\.[0-9]{1,6})?$'},description:'The complete price catalog for the selected immutable rule profile.'};
const item={...obj({risk_points:positive,loss_points:amount,code:str,points:positive,odds:str,outcome:{type:'string',enum:['','won','lost','push']},payout:amount,special_odds:str,winning_odds:str,effective:bool}),required:['code','points','odds','outcome','payout','effective','risk_points','loss_points']};
const purchase=obj({reserved_points:positive,profile,game_id:uuid,period_id:uuid,rule_version:int,sales_version:int,items:arr(item),total_points:positive});
const order=obj({...purchase.properties,lost_points:amount,id:uuid,brand_id:uuid,member_id:uuid,period_no:str,prize_points:amount,refund_points:amount,effective_points:amount,status:{type:'string',enum:['placed','settled']},placed_at:ref('DateTime'),settled_at:nullable(ref('DateTime'))});
order.required=Object.keys(order.properties);
export const schemas={
 CanadaOrder:order,
};
const page=[{name:'offset',in:'query',schema:int},{name:'limit',in:'query',schema:{type:'integer',minimum:1,maximum:100}}];
const op=(method,path,operationId,summary,data,extra={})=>({method,path,operationId,summary,data,tag:'canada',auth:'user',...extra});
export const operations=[
 op('GET','/api/v1/canada/orders','canadaAllMyOrders','Read all of this member’s Canada tickets',obj({items:arr(ref('CanadaOrder'))}),{parameters:page}),
 op('GET','/api/v1/canada/games/{id}/orders','canadaMyOrders','Read this member’s Canada tickets',obj({items:arr(ref('CanadaOrder'))}),{parameters:page}),
 op('GET','/api/v1/admin/platform-games/{id}/canada/orders','canadaBrandOrders','Read this brand’s Canada tickets',obj({items:arr(ref('CanadaOrder'))}),{auth:'admin',brandHeader:true,permissions:['platform_game.view.brand'],parameters:page}),
];
