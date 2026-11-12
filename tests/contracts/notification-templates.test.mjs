import {test} from "node:test";
import { notificationCatalog } from "../../shared/src/notification-catalog.ts";
import assert from "node:assert/strict";
import {execFileSync} from "node:child_process";
import {readFileSync} from "node:fs";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import {operations,schemas} from "../../docs/openapi/lottery.mjs";

const built=JSON.parse(readFileSync(new URL("../../docs/openapi.json",import.meta.url),"utf8"));
const references=JSON.parse(readFileSync(new URL("../../backend/internal/notification/template_defaults.json",import.meta.url),"utf8"));
const defaults=notificationCatalog;
const baseline=readFileSync(new URL("../../backend/migrations/0001_baseline.up.sql",import.meta.url),"utf8");
const componentSchemas={...built.components.schemas,...schemas};
const ajv=new Ajv2020({strict:false,allErrors:true});
addFormats(ajv);
ajv.addFormat("int64",{type:"number",validate:Number.isInteger});
ajv.addSchema({$id:"urn:lottery:notification-contract",components:{schemas:componentSchemas}});
const validate=name=>ajv.compile({$ref:`urn:lottery:notification-contract#/components/schemas/${name}`});
const id="11111111-1111-4111-8111-111111111111";
const notificationKeys=[
  "member.joined","recharge.confirmed","bet.order.placed","bet.order.cancelled",
  "bet.order.judged_cancelled","bet.order.abnormal","bet.order.won","bet.order.prize_reversed",
  "withdrawal.order.reviewing","withdrawal.order.processing","withdrawal.order.paid",
  "withdrawal.order.rejected","withdrawal.order.failed","withdrawal.order.cancelled",
  "reward.order.granted","reward.order.revoked",
  "commission.paid","commission.adjusted","commission.corrected",
  "draw.result.published","draw.result.corrected",
];
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
    ["GET","/api/v1/platform/notification-templates","platformListNotificationTemplates"],
    ["GET","/api/v1/platform/notification-templates/{key}/history","platformListNotificationTemplateHistory"],
    ["PUT","/api/v1/platform/notification-templates/{key}","platformUpdateNotificationTemplate"],
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
  const write=operations.find(item=>item.operationId==="platformUpdateNotificationTemplate");
  assert.deepEqual(write.permissions,["notification_template.write.platform"]);
  assert.equal(write.idempotency,true);
  assert.equal(write.requestBody.$ref,"#/components/schemas/LotteryNotificationTemplateUpdateInput");
  assert.deepEqual(schemas.LotteryNotificationTemplate.properties.key.enum,[...notificationKeys].sort());
});

test("the shared i18n catalog defines all twenty-one bilingual system notifications",()=>{
  assert.deepEqual(Object.keys(defaults).sort(),[...notificationKeys].sort());
  const check=validate("LotteryNotificationTemplateContent");
  for (const [key, translations] of Object.entries(defaults)) {
    assert.ok(check(translations), `${key}: ${JSON.stringify(check.errors)}`);
    for (const locale of ["en", "zh-CN"]) {
      const copy=translations[locale];
      assert.ok(copy.title.length > 0);
      assert.doesNotMatch(copy.body, /Historical record|this record is retained|历史记录|此记录会保留/);
      if (key!=="member.joined" && !key.startsWith("draw.")) assert.match(copy.body, /\{points\}/);
    }
  }
  assert.equal(defaults["bet.order.won"]["zh-CN"].title,"派奖入账");
  assert.equal(defaults["bet.order.prize_reversed"]["zh-CN"].title,"奖金冲正");

});

test("system templates contain only i18n identities and the migration converts stored defaults",()=>{
  assert.deepEqual(Object.keys(references).sort(),[...notificationKeys].sort());
  for(const key of notificationKeys) assert.deepEqual(references[key],{i18n_key:key,i18n_version:1});
  const migration=readFileSync(new URL("../../backend/migrations/0001_baseline.up.sql",import.meta.url),"utf8");
  assert.match(migration,/jsonb_build_object\('i18n_key',k,'i18n_version',1\)/);
  assert.match(migration,/UPDATE notifications SET content=notification_template_defaults\(\)->template_key WHERE template_version=1/);
  const check=validate("LotteryNotificationTemplateContent");
  for(const key of notificationKeys) assert.ok(check(references[key]));
  assert.ok(!check({i18n_key:"missing",i18n_version:1}));
  assert.ok(!check({i18n_key:"member.joined",i18n_version:2}));
  assert.ok(!check({...references["member.joined"],en:defaults["member.joined"].en}));
});

test("reward defaults distinguish gift credits from reversals in both languages",()=>{
  for(const key of ["reward.order.granted","reward.order.revoked"]){
    assert.match(defaults[key].en.body,/gift balance/);
    assert.match(defaults[key]["zh-CN"].body,/赠送可用积分/);
  }
  assert.match(defaults["reward.order.granted"].en.body,/credited/);
  assert.match(defaults["reward.order.revoked"].en.body,/reversed/);
  assert.match(defaults["reward.order.granted"]["zh-CN"].body,/入账/);
  assert.match(defaults["reward.order.revoked"]["zh-CN"].body,/撤销/);
});

test("template and revision schemas enforce closed structures and initial-version audit nullability",()=>{
  const template=validate("LotteryNotificationTemplate");
  const revision=validate("LotteryNotificationTemplateRevision");
  const base={key:"member.joined",version:1,content:content("Welcome","Your membership is ready."),updated_at:"2026-10-07T00:00:00Z",audit_log_id:null};
  assert.ok(template(base),JSON.stringify(template.errors));
  assert.ok(!template({...base,key:"unknown"}));
  assert.ok(!template({...base,unexpected:true}));
  assert.ok(!template({...base,audit_log_id:id}));
  assert.ok(template({...base,version:2,audit_log_id:id}));
  assert.ok(!template({...base,version:2,audit_log_id:null}));

  const first={id,key:"member.joined",version:1,content:base.content,changed_by:null,reason:"Initial in-app notification template",audit_log_id:null,created_at:"2026-10-07T00:00:00Z"};
  assert.ok(revision(first),JSON.stringify(revision.errors));
  assert.ok(!revision({...first,changed_by:id}));
  assert.ok(revision({...first,version:2,changed_by:id,audit_log_id:id}));
  assert.ok(!revision({...first,version:2,changed_by:null,audit_log_id:id}));
});

test("notification snapshots pair event and template keys and require current content for every version",()=>{
  const check=validate("LotteryNotification");
  const row={id,brand_id:id,member_id:id,event_type:"member.joined",template_key:"member.joined",template_version:1,content:content("Welcome","Your membership is ready."),payload:{resource_id:id,points:null},created_at:"2026-10-07T00:00:00Z",read_at:null};
  assert.ok(check(row),JSON.stringify(check.errors));
  assert.ok(!check({...row,event_type:"bet.order.placed"}));
  assert.ok(!check({...row,template_key:"private.template"}));
  assert.ok(check({...row,event_type:"recharge.confirmed",template_key:"recharge.confirmed",template_version:2,content:content("Recharge","Added {points} points."),payload:{resource_id:id,points:"1"}}));
  assert.ok(!check({...row,content:null}));
  assert.ok(!check({...row,content:undefined}));
  assert.ok(!check({...row,content:{...content("Welcome","Ready."),private_key:"secret"}}));
});

test("withdrawal snapshots cover six historical states with positive int64 payloads only",()=>{
  const check=validate("LotteryNotification");
  const states=["reviewing","processing","paid","rejected","failed","cancelled"];
  for(const state of states){
    const key=`withdrawal.order.${state}`;
    const row={
      id,brand_id:id,member_id:id,event_type:key,template_key:key,template_version:2,
      content:content("Withdrawal status recorded",`Historical status: ${state}. {points}.`),
      payload:{resource_id:id,points:"92233720368547758.07"},created_at:"2026-10-07T00:00:00Z",read_at:null,
    };
    assert.ok(check(row),`${key}: ${JSON.stringify(check.errors)}`);
    assert.ok(!check({...row,content:null}),`${key} requires its immutable snapshot`);
    assert.ok(!check({...row,payload:{resource_id:id,points:null}}),`${key} requires positive points`);
    assert.ok(!check({...row,payload:{resource_id:id,points:"92233720368547758.08"}}),`${key} rejects int64 overflow`);
    assert.ok(!check({...row,payload:{...row.payload,reason:"raw reason"}}),`${key} rejects private reason fields`);
  }
  const unknown={id,brand_id:id,member_id:id,event_type:"withdrawal.order.unknown",template_key:"withdrawal.order.unknown",template_version:2,content:content(),payload:{resource_id:id,points:"1"},created_at:"2026-10-07T00:00:00Z",read_at:null};
  assert.ok(!check(unknown));
});

test("commission notifications expose only resource and signed point facts",()=>{
  const check=validate("LotteryNotification");
  const paid={id,brand_id:id,member_id:id,event_type:"commission.paid",template_key:"commission.paid",template_version:2,content:content("Commission credit recorded","Historical credit: {points}."),payload:{resource_id:id,points:"92233720368547758.07"},created_at:"2026-10-07T00:00:00Z",read_at:null};
  assert.ok(check(paid),JSON.stringify(check.errors));
  assert.ok(!check({...paid,payload:{...paid.payload,points:"-1"}}));
  assert.ok(!check({...paid,payload:{...paid.payload,ledger_entry_id:id}}));
  const adjusted={...paid,event_type:"commission.adjusted",template_key:"commission.adjusted",content:content("Commission adjustment recorded","Historical adjustment: {points}."),payload:{resource_id:id,points:"-92233720368547758.08"}};
  assert.ok(check(adjusted),JSON.stringify(check.errors));
  assert.ok(check({...adjusted,payload:{...adjusted.payload,points:"92233720368547758.07"}}));
  for(const points of ["0","-0","+1","01","-01","92233720368547758.08","-92233720368547758.09"]){
    assert.ok(!check({...adjusted,payload:{...adjusted.payload,points}}),`accepted invalid signed amount ${points}`);
  }
  const corrected={...adjusted,event_type:"commission.corrected",template_key:"commission.corrected",content:content("Commission correction recorded","Historical correction: {points}."),payload:{resource_id:id,points:"-1"}};
  assert.ok(check(corrected),JSON.stringify(check.errors));
  assert.ok(check({...corrected,payload:{...corrected.payload,points:"92233720368547758.07"}}));
  assert.ok(!check({...corrected,payload:{...corrected.payload,resource_id:"correction-id"}}));
  for(const field of ["member_id","ledger_entry_id","target_id","audit_log_id","version"]){
    assert.ok(!check({...corrected,payload:{...corrected.payload,[field]:id}}),`commission.corrected exposes private ${field}`);
  }
  for(const points of ["0","-0","+1","01","-01","92233720368547758.08","-92233720368547758.09"]){
    assert.ok(!check({...corrected,payload:{...corrected.payload,points}}),`corrected accepted invalid signed amount ${points}`);
  }
  assert.ok(!check({...adjusted,payload:{...adjusted.payload,created_by:id}}));
  assert.ok(!check({...corrected,payload:{...corrected.payload,target_id:id}}));
  for (const row of [paid, adjusted, corrected]) assert.ok(!check({...row,template_version:1,content:null}), `${row.event_type} requires content at version 1`);
});

test("draw notifications have closed typed payloads and require immutable content",()=>{
  const check=validate("LotteryNotification");
  const at="2026-10-07T00:00:00Z";
  const draw={game_id:id,period_id:id,period_no:"20261007001",result:{regular:[0,12,999999,1000000],special:[0],digits:[]},drawn_at:at,previous_draw_id:null};
  const published={id,brand_id:id,member_id:id,event_type:"draw.result.published",template_key:"draw.result.published",template_version:1,content:defaults["draw.result.published"],payload:{resource_id:id,points:null,draw},created_at:at,read_at:null};
  assert.ok(check(published),JSON.stringify(check.errors));
  assert.ok(!check({...published,content:null}),"draw notifications require content at template version 1");
  assert.ok(!check({...published,payload:{...published.payload,points:"0"}}));
  assert.ok(!check({...published,payload:{...published.payload,private:"value"}}));
  assert.ok(!check({...published,payload:{...published.payload,draw:{...draw,unknown:true}}}));
  assert.ok(!check({...published,payload:{...published.payload,draw:{...draw,period_no:""}}}));
  assert.ok(!check({...published,payload:{...published.payload,draw:{...draw,period_no:"x".repeat(81)}}}));
  assert.ok(!check({...published,payload:{...published.payload,draw:{...draw,previous_draw_id:id}}}));
  assert.ok(!check({...published,payload:{...published.payload,draw:{...draw,result:{...draw.result,regular:[1000001]}}}}));
  assert.ok(!check({...published,payload:{...published.payload,draw:{...draw,result:{...draw.result,regular:Array(11).fill(1)}}}}));
  assert.ok(!check({...published,payload:{...published.payload,draw:{...draw,result:{...draw.result,special:Array(11).fill(1)}}}}));
  assert.ok(!check({...published,payload:{...published.payload,draw:{...draw,result:{regular:[],special:[],digits:[]}}}}));
  assert.ok(!check({...published,payload:{...published.payload,draw:{...draw,result:{regular:[1],special:[],digits:[1]}}}}));
  assert.ok(!check({...published,payload:{...published.payload,draw:{...draw,result:{regular:[],special:[],digits:Array(11).fill(1)}}}}));
  assert.ok(!check({...published,payload:{...published.payload,draw:{...draw,result:{regular:[],special:[],digits:[10]}}}}));

  const corrected={...published,event_type:"draw.result.corrected",template_key:"draw.result.corrected",content:defaults["draw.result.corrected"],payload:{...published.payload,draw:{...draw,previous_draw_id:id}}};
  assert.ok(check(corrected),JSON.stringify(check.errors));
  assert.ok(!check({...corrected,payload:{...corrected.payload,draw}}));
  assert.ok(!check({...corrected,payload:{...corrected.payload,draw:{...corrected.payload.draw,previous_draw_id:"not-a-uuid"}}}));
  assert.ok(!check({...corrected,template_version:1,content:null}));

  const digitDraw={...published,payload:{...published.payload,draw:{...draw,result:{regular:[],special:[],digits:[0,9]}}}};
  assert.ok(check(digitDraw),JSON.stringify(check.errors));
});

test("real Go draw notification DTO examples satisfy both event-specific schemas",()=>{
  const output=execFileSync(process.env.LOTTERY_GO_BIN??"go",["run","-buildvcs=false","./cmd/contract-examples"],{
    cwd:new URL("../../backend/",import.meta.url),encoding:"utf8",env:{...process.env,CGO_ENABLED:"0"},
  });
  const examples=JSON.parse(output);
  const check=validate("LotteryNotification");
  for(const [key,eventType] of [["LotteryNotificationDrawPublished","draw.result.published"],["LotteryNotificationDrawCorrected","draw.result.corrected"]]){
    const item=examples[key];
    assert.ok(item,`contract-examples missing ${key}`);
    assert.equal(item.event_type,eventType);
    assert.ok(check(item),`${key}: ${JSON.stringify(check.errors)}`);
    const eventCheck=validate(key);
    assert.ok(eventCheck(item),`${key} event-specific schema: ${JSON.stringify(eventCheck.errors)}`);
    assert.equal(item.payload.points,null);
    assert.ok(item.content,"draw notification DTO must contain the immutable content snapshot");
    const otherEvent=eventType==="draw.result.published"?"draw.result.corrected":"draw.result.published";
    assert.ok(!eventCheck({...item,event_type:otherEvent}),`${key} must constrain event_type to ${eventType}`);
  }
});

test("reward notifications require frozen content and expose only UUID plus positive int64 points",()=>{
  const check=validate("LotteryNotification");
  const rewards=notificationKeys.filter(key=>key.startsWith("reward.order."));
  for(const key of rewards){
    const row={id,brand_id:id,member_id:id,event_type:key,template_key:key,template_version:1,content:defaults[key],payload:{resource_id:id,points:"92233720368547758.07"},created_at:"2026-10-07T00:00:00Z",read_at:null};
    assert.ok(check(row),`${key}: ${JSON.stringify(check.errors)}`);
    for(const content of [null,undefined])assert.ok(!check({...row,content}),`${key} requires snapshot content for v1`);
    for(const points of [0,-1,1,"0","-1","01","92233720368547758.08",null,undefined])assert.ok(!check({...row,payload:{resource_id:id,points}}),`${key} rejects ${String(points)}`);
    for(const field of ["actor_id","reason","ledger_entry_id","member_id","status"])assert.ok(!check({...row,payload:{...row.payload,[field]:"private"}}),`${key} rejects ${field}`);
    assert.ok(!check({...row,template_key:key==="reward.order.revoked"?"reward.order.granted":"reward.order.revoked"}),`${key} event and template key must match`);
  }
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
  assert.match(schemas.LotteryNotificationCustomContent.description,/nineteen event templates require \{points\} in each language body/);
  assert.match(schemas.LotteryNotificationTemplateCopy.properties.title.description,/120 characters/);
  assert.match(schemas.LotteryNotificationTemplateCopy.properties.body.description,/1200 characters/);
});
