import {test} from "node:test";
import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import {operations,schemas} from "../../docs/openapi/lottery.mjs";

const built=JSON.parse(readFileSync(new URL("../../docs/openapi.json",import.meta.url),"utf8"));
const componentSchemas={...built.components.schemas,...schemas};
const ajv=new Ajv2020({strict:false,allErrors:true});
addFormats(ajv);
ajv.addFormat("int64",{type:"number",validate:Number.isInteger});
ajv.addSchema({$id:"urn:lottery:notification-contract",components:{schemas:componentSchemas}});
const validate=name=>ajv.compile({$ref:`urn:lottery:notification-contract#/components/schemas/${name}`});
const id="11111111-1111-4111-8111-111111111111";
const content=(title="Notice",body="Update {points} points.")=>({
  en:{title,body},"zh-CN":{title:"通知",body:"更新 {points} 积分。"},
});

test("all notification template components compile and routes match the implemented contract",()=>{
  const names=Object.keys(schemas).filter(name=>name.startsWith("LotteryNotificationTemplate"));
  assert.deepEqual(names.sort(),[
    "LotteryNotificationTemplate","LotteryNotificationTemplateContent","LotteryNotificationTemplateCopy",
    "LotteryNotificationTemplateHistoryPage","LotteryNotificationTemplateRevision",
    "LotteryNotificationTemplatesPage","LotteryNotificationTemplateUpdateInput",
  ].sort());
  for(const name of [...names,"LotteryNotification"])validate(name);

  const expected=[
    ["GET","/api/v1/admin/notification-templates","adminListNotificationTemplates"],
    ["GET","/api/v1/admin/notification-templates/{key}/history","adminListNotificationTemplateHistory"],
    ["PUT","/api/v1/admin/notification-templates/{key}","adminUpdateNotificationTemplate"],
  ];
  for(const [method,path,operationId] of expected){
    const operation=operations.find(item=>item.method===method&&item.path===path);
    assert.ok(operation,`${method} ${path}`);
    assert.equal(operation.operationId,operationId);
    assert.equal(operation.data.$ref.startsWith("#/components/schemas/LotteryNotificationTemplate"),true);
    if(path.includes("{key}")){
      const keyParameter=operation.parameters?.find(parameter=>parameter.in==="path"&&parameter.name==="key");
      assert.ok(keyParameter?.required,"event keys need an explicit path declaration, not a UUID fallback");
      assert.deepEqual(keyParameter.schema.enum,schemas.LotteryNotificationTemplate.properties.key.enum);
    }
  }
  const write=operations.find(item=>item.operationId==="adminUpdateNotificationTemplate");
  assert.deepEqual(write.permissions,["notification_template.write.brand"]);
  assert.equal(write.idempotency,true);
  assert.equal(write.requestBody.$ref,"#/components/schemas/LotteryNotificationTemplateUpdateInput");
});

test("template and revision schemas enforce closed structures and legacy version-one audit nullability",()=>{
  const template=validate("LotteryNotificationTemplate");
  const revision=validate("LotteryNotificationTemplateRevision");
  const base={brand_id:id,key:"member.joined",version:1,content:content("Welcome","Your membership is ready."),updated_at:"2026-10-07T00:00:00Z",audit_log_id:null};
  assert.ok(template(base),JSON.stringify(template.errors));
  assert.ok(!template({...base,key:"unknown"}));
  assert.ok(!template({...base,unexpected:true}));
  assert.ok(!template({...base,audit_log_id:id}));
  assert.ok(template({...base,version:2,audit_log_id:id}));
  assert.ok(!template({...base,version:2,audit_log_id:null}));

  const first={id,brand_id:id,key:"member.joined",version:1,content:base.content,changed_by:null,reason:"Initial in-app notification template",audit_log_id:null,created_at:"2026-10-07T00:00:00Z"};
  assert.ok(revision(first),JSON.stringify(revision.errors));
  assert.ok(!revision({...first,changed_by:id}));
  assert.ok(revision({...first,version:2,changed_by:id,audit_log_id:id}));
  assert.ok(!revision({...first,version:2,changed_by:null,audit_log_id:id}));
});

test("notification snapshots pair event and template keys and allow null content only for legacy v1",()=>{
  const check=validate("LotteryNotification");
  const row={id,brand_id:id,member_id:id,event_type:"member.joined",template_key:"member.joined",template_version:1,content:null,payload:{resource_id:id,points:null},created_at:"2026-10-07T00:00:00Z",read_at:null};
  assert.ok(check(row),JSON.stringify(check.errors));
  assert.ok(!check({...row,event_type:"bet.order.placed"}));
  assert.ok(!check({...row,template_key:"private.template"}));
  assert.ok(!check({...row,template_version:2}));
  assert.ok(check({...row,event_type:"recharge.confirmed",template_key:"recharge.confirmed",template_version:2,content:content("Recharge","Added {points} points.")}));
  assert.ok(!check({...row,event_type:"recharge.confirmed",template_key:"recharge.confirmed",template_version:2,content:null}));
  assert.ok(!check({...row,content:{...content("Welcome","Ready."),private_key:"secret"}}));
});

test("template copy schemas reject unknown placeholders and private fields",()=>{
  const check=validate("LotteryNotificationTemplateContent");
  const good=content();
  assert.ok(check(good),JSON.stringify(check.errors));
  assert.ok(!check({...good,en:{...good.en,body:"Unknown {customer_name} {points}"}}));
  assert.ok(!check({...good,en:{...good.en,private_key:"secret"}}));
  assert.ok(!check({...good,fr:{title:"Notice",body:"Body"}}));
  assert.ok(!check({...good,en:{...good.en,title:"<b>Notice</b>"}}));

  const joined={en:{title:"Welcome",body:"Membership ready."},"zh-CN":{title:"欢迎",body:"会员已就绪。"}};
  assert.ok(check(joined),JSON.stringify(check.errors));
  assert.match(schemas.LotteryNotificationTemplateContent.description,/seven event templates require \{points\} in each language body/);
  assert.match(schemas.LotteryNotificationTemplateCopy.properties.title.description,/120 UTF-8 bytes/);
  assert.match(schemas.LotteryNotificationTemplateCopy.properties.body.description,/1200 UTF-8 bytes/);
});
