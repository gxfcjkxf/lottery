const ref=name=>({$ref:`#/components/schemas/${name}`});
const object=(properties,required=Object.keys(properties))=>({type:'object',additionalProperties:false,properties,required});
export const schemas={
 BrandContact:object({id:ref('UUID'),kind:{type:'string',enum:['wechat','telegram','whatsapp']},value:{type:'string',maxLength:120},image_url:{type:['string','null'],pattern:'^/api/v1/brand-assets/[a-f0-9]{64}\\.(png|jpg)$'}}),
 BrandContactConfig:object({items:{type:'array',items:ref('BrandContact')},default_id:{anyOf:[ref('UUID'),{type:'null'}]}}),
 BrandContactInput:object({version:{type:'integer',minimum:1},config:ref('BrandContactConfig')}),
 BrandContactRecord:object({brand_id:ref('UUID'),version:{type:'integer',minimum:1},status:{type:'string',enum:['active','paused','disabled']},config:ref('BrandContactConfig'),audit_log_id:ref('UUID')},['brand_id','version','status','config']),
};
export const operations=[
 {method:'GET',path:'/api/v1/brand-contacts',operationId:'publicBrandContacts',summary:'Read current brand customer-service contacts',tag:'identity',auth:'public',data:ref('BrandContactConfig'),description:'Resolved brand only. Empty config is items=[] and default_id=null. User profile contacts and Telegram authentication are independent.'},
 {method:'GET',path:'/api/v1/admin/brand-contacts',operationId:'readBrandContacts',summary:'Read brand customer-service contacts',tag:'administration',auth:'admin',brandHeader:true,permissions:['brand_presentation.view.brand'],data:ref('BrandContactRecord')},
 {method:'PUT',path:'/api/v1/admin/brand-contacts',operationId:'updateBrandContacts',summary:'Save brand customer-service contacts',tag:'administration',auth:'admin',brandHeader:true,permissions:['brand_presentation.write.brand'],idempotency:true,requestBody:ref('BrandContactInput'),data:ref('BrandContactRecord'),description:'Versioned, audited replacement with checked idempotent replay. Each item has a number or brand-owned uploaded image, or both. Nonempty lists require exactly one matching default_id. Clearing image_url retains value. Disabled brands are read-only.'},
 {method:'POST',path:'/api/v1/admin/brand-contacts/image',operationId:'uploadBrandContactImage',summary:'Upload a brand contact image',tag:'administration',auth:'admin',brandHeader:true,permissions:['brand_presentation.write.brand'],data:object({url:{type:'string'}}),requestContent:{'image/png':{schema:{type:'string',format:'binary'}},'image/jpeg':{schema:{type:'string',format:'binary'}}},description:'Uses the existing brand asset store. PNG or JPG, up to 1 MiB and 4096 pixels per side. Brand-scoped content hash makes upload retries repeatable. Upload does not publish contact settings; save them separately.'},
];
