const ref = name => ({ $ref: `#/components/schemas/${name}` });
export const schemas = {
 AssociationSelection: {type:'object',additionalProperties:false,required:['id','member_id','brand_id','username'],properties:{id:ref('UUID'),member_id:ref('UUID'),brand_id:ref('UUID'),username:{type:'string'}}},
};
export const operations = [{
 method:'GET',path:'/api/v1/admin/association-selection',operationId:'selectAssociatedAccount',summary:'Select an associated account by exact username',tag:'administration',auth:'admin',brandHeader:true,data:ref('AssociationSelection'),
 parameters:[
  {name:'kind',in:'query',required:true,schema:{type:'string',enum:['member','agent']}},
  {name:'resource',in:'query',required:true,schema:{type:'string',enum:['agent','join_code','commission','report_commission','report_attribution']}},
  {name:'action',in:'query',required:true,schema:{type:'string',enum:['view','write']}},
  {name:'username',in:'query',required:true,schema:{type:'string',minLength:1,maxLength:100}},
 ],
 permissions:['agent.write.brand','join_code.write.brand','commission.view.brand','report_commission.view.brand','report_attribution.view.brand'],
 description:'Read-only exact username selection, within the selected brand. Requires the named target-operation grant: agent/write, join_code/write, commission/view, report_commission/view or report_attribution/view; these alternatives are not cumulative. Usernames are trimmed and lowercased. Duplicate or unknown parameters are rejected. Does not grant user or agent browsing and cannot select arbitrary UUIDs. Existing mutation endpoints independently enforce all original association, scope and version checks.',
}];
