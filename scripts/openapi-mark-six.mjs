const ref=name=>({$ref:`#/components/schemas/${name}`});
const obj = (properties, required = Object.keys(properties)) => ({ type: 'object', properties, required, additionalProperties: false });
const arr=items=>({type:'array',items});
const nullable=value=>({anyOf:[value,{type:'null'}]});
const str={type:'string'},int={type:'integer',minimum:0},bool={type:'boolean'},uuid=ref('UUID'),amount=ref('NonnegativeInt64String'),positive=ref('PositiveInt64String');
const codes=[...Array.from({length:49},(_,i)=>`number_${String(i+1).padStart(2,'0')}`),'big','small','odd','even','big_odd','big_even','small_odd','small_even','sum_odd','sum_even','sum_big','sum_small','tail_big','tail_small',...['red','blue','green'].map(v=>`wave_${v}`),...['rat','ox','tiger','rabbit','dragon','snake','horse','goat','monkey','rooster','dog','pig'].map(v=>`zodiac_${v}`),...['metal','wood','water','fire','earth'].map(v=>`element_${v}`)];
const seq=n=>Array.from({length:n},(_,i)=>i);
codes.push(...seq(49).map(i=>`regular_${String(i+1).padStart(2,'0')}`),...seq(6).flatMap(p=>seq(49).map(n=>`position_${p+1}_${String(n+1).padStart(2,'0')}`)), 'linked_2','linked_3_2','linked_3_2_full','linked_3',...seq(5).map(i=>`zodiac_link_${i+1}`),...seq(4).flatMap(i=>[`tail_link_${i+2}`,`tail_link_${i+2}_zero`]),...seq(6).map(i=>`miss_${i+5}`),...seq(5).map(i=>`head_${i}`),...seq(10).flatMap(i=>[`special_tail_${i}`,`draw_tail_${i}`]),'domestic','wild','seven_red','seven_blue','seven_green','seven_tie',...['red','blue','green'].flatMap(w=>['big','small','odd','even'].map(s=>`half_${w}_${s}`)),...seq(7).map(i=>`zodiac_count_${i+1}`));
const attributes=['big','small','odd','even','big_odd','big_even','small_odd','small_even','sum_odd','sum_even','sum_big','sum_small','tail_big','tail_small','wave_red','wave_blue','wave_green'];
codes.push('total_big','total_small','total_odd','total_even','linked_4','linked_2_special','linked_2_special_hit','linked_special','zodiac_count_odd','zodiac_count_even','heaven','earth','front','back',...seq(6).flatMap(p=>attributes.map(v=>`position_attribute_${p+1}_${v}`)),...['rat','ox','tiger','rabbit','dragon','snake','horse','goat','monkey','rooster','dog','pig'].map(v=>`regular_zodiac_${v}`),...['red','blue','green'].flatMap(w=>['big_odd','big_even','small_odd','small_even'].map(v=>`quarter_${w}_${v}`)),...seq(10).flatMap(i=>[i===0?'zodiac_pair':`zodiac_hit_${i+2}`,`zodiac_miss_${i+2}`]),...seq(5).flatMap(i=>[`zodiac_nohit_${i+1}`,`tail_nohit_${i+1}`]));
const selectable=[...codes.filter(v=>v!=='linked_3_2_full'&&v!=='linked_2_special_hit'&&!v.endsWith('_zero')),'parlay'];
const selection={...arr({type:'integer',minimum:0,maximum:616}),maxItems:49,uniqueItems:true};
const odds=obj(Object.fromEntries(codes.map(code=>[code,{type:'string',pattern:'^[0-9]+(\\.[0-9]{1,6})?$'}])));
const plate={type:'string',enum:['A','B']};
const item={...obj({code:{type:'string',enum:selectable},points:positive,odds:str,outcome:{type:'string',enum:['','won','lost','push']},payout:amount,selection,odds_code:{type:'string',enum:codes},odds_full:str,winning_odds:str,leg_odds:arr(str),hit_count:{type:'integer',minimum:1,maximum:6}}),required:['code','points','odds','outcome','payout']};
const purchase=obj({plate,attributes:ref('MarkSixAttributes'),game_id:uuid,period_id:uuid,rule_version:int,sales_version:int,items:arr(item),total_points:positive});
const order=obj({...purchase.properties,id:uuid,brand_id:uuid,member_id:uuid,period_no:str,prize_points:amount,refund_points:amount,effective_points:amount,status:{type:'string',enum:['placed','settled']},placed_at:ref('DateTime'),settled_at:nullable(ref('DateTime'))});
order.required=Object.keys(order.properties);
const numberMap=values=>obj(Object.fromEntries(Array.from({length:49},(_,i)=>[String(i+1).padStart(2,'0'),{type:'string',enum:values}])));
const attributeInput={year:{type:'integer',minimum:1900,maximum:9999},version:int,effective_at:ref('DateTime'),expires_at:ref('DateTime'),waves:numberMap(['red','blue','green']),zodiac:numberMap(['rat','ox','tiger','rabbit','dragon','snake','horse','goat','monkey','rooster','dog','pig']),elements:numberMap(['metal','wood','water','fire','earth'])};
export const schemas={
 MarkSixAttributes:obj({id:uuid,...attributeInput}),
 MarkSixOrder:order,
};
const page=[{name:'offset',in:'query',schema:int},{name:'limit',in:'query',schema:{type:'integer',minimum:1,maximum:100}}];
const op=(method,path,operationId,summary,data,extra={})=>({method,path,operationId,summary,data,tag:'mark-six',auth:'user',...extra});
export const operations=[
 op('POST','/api/v1/platform/platform-games/{id}/mark-six/attributes','markSixPublishAttributes','Publish immutable number attributes with an explicit validity window',ref('MarkSixAttributes'),{auth:'admin',permissions:['platform_game.write.platform'],requestBody:obj({...attributeInput,reason:ref('Reason')}, Object.keys(attributeInput)),idempotency:true}),
 op('GET','/api/v1/mark-six/orders','markSixAllMyOrders','Read all of this member’s Mark Six tickets',obj({items:arr(ref('MarkSixOrder'))}),{parameters:page}),
 op('GET','/api/v1/mark-six/games/{id}/orders','markSixMyOrders','Read this member’s Mark Six tickets',obj({items:arr(ref('MarkSixOrder'))}),{parameters:page}),
 op('GET','/api/v1/admin/platform-games/{id}/mark-six/orders','markSixBrandOrders','Read this brand’s Mark Six tickets',obj({items:arr(ref('MarkSixOrder'))}),{auth:'admin',brandHeader:true,permissions:['platform_game.view.brand'],parameters:page}),
];
