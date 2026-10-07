import {test} from "node:test";
import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import {operations,schemas} from "../../docs/openapi/lottery.mjs";

const built=JSON.parse(readFileSync(new URL("../../docs/openapi.json",import.meta.url),"utf8"));
const defaults=JSON.parse(readFileSync(new URL("../../backend/internal/notification/template_defaults.json",import.meta.url),"utf8"));
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
  assert.deepEqual(schemas.LotteryNotificationTemplate.properties.key.enum,[...notificationKeys].sort());
});

test("the fixture has exactly the immutable eight legacy and six withdrawal defaults",()=>{
  assert.deepEqual(Object.keys(defaults).sort(),[...notificationKeys].sort());
  const legacy={
    "member.joined":{en:{title:"Welcome",body:"Your membership is ready. Welcome aboard."},"zh-CN":{title:"欢迎",body:"您的会员账户已准备就绪，欢迎加入。"}},
    "recharge.confirmed":{en:{title:"Recharge confirmed",body:"Recharge confirmed: {points} points."},"zh-CN":{title:"充值已确认",body:"充值已确认：{points} 积分。"}},
    "bet.order.placed":{en:{title:"Order submitted",body:"Order submitted: {points} points."},"zh-CN":{title:"注单已提交",body:"注单已提交，涉及 {points} 积分。"}},
    "bet.order.cancelled":{en:{title:"Order cancelled",body:"Cancelled. {points} points were returned to your original balance."},"zh-CN":{title:"注单已取消",body:"注单已取消，{points} 积分已原路退回。"}},
    "bet.order.judged_cancelled":{en:{title:"Order cancelled after review",body:"Cancelled after review. {points} points were returned to your original balance."},"zh-CN":{title:"注单已判定取消",body:"注单经判定已取消，{points} 积分已原路退回。"}},
    "bet.order.abnormal":{en:{title:"Order needs review",body:"Your order needs manual review. Points involved: {points}."},"zh-CN":{title:"注单待人工处理",body:"您的注单需要人工处理，涉及积分：{points}。"}},
    "bet.order.won":{en:{title:"Prize credit recorded",body:"Historical record: {points} points were credited as this order's prize. This records the credit, not your current wallet balance or a guaranteed final outcome. Any correction will appear as a separate prize event; this record is retained."},"zh-CN":{title:"派奖入账记录",body:"历史记录：此注单的 {points} 积分奖金已记入账本。此记录仅表示该笔入账，不代表当前钱包余额，也不保证最终结果。任何更正都会作为单独的奖金事件记录；此记录会保留。"}},
    "bet.order.prize_reversed":{en:{title:"Prize reversal recorded",body:"Historical record: the full original prize amount of {points} points for this order was reversed. This records the reversal, not your current wallet balance. Any later prize correction will appear as a separate event; this record is retained."},"zh-CN":{title:"奖金冲正记录",body:"历史记录：此注单原奖金全额 {points} 积分已冲回。此记录仅表示该笔冲正，不代表当前钱包余额。之后如有奖金更正，会作为单独事件记录；此记录会保留。"}},
  };
  for(const [key,value] of Object.entries(legacy))assert.deepEqual(defaults[key],value,`immutable default ${key}`);
  const states={reviewing:"审核中",processing:"提现中",paid:"已提现",rejected:"已驳回",failed:"失败",cancelled:"已取消"};
  for(const [state,stateZh] of Object.entries(states)){
    const entry=defaults[`withdrawal.order.${state}`];
    assert.deepEqual(entry,{
      en:{title:"Withdrawal status recorded",body:`Historical withdrawal status: ${state}. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state.`},
      "zh-CN":{title:"提现状态记录",body:`历史提现状态：${stateZh}，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。`},
    });
  }
  for(const key of notificationKeys.slice(0,8))assert.ok(defaults[key],`preserved legacy default ${key}`);
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

test("withdrawal snapshots cover six historical states with positive int64 payloads only",()=>{
  const check=validate("LotteryNotification");
  const states=["reviewing","processing","paid","rejected","failed","cancelled"];
  for(const state of states){
    const key=`withdrawal.order.${state}`;
    const row={
      id,brand_id:id,member_id:id,event_type:key,template_key:key,template_version:2,
      content:content("Withdrawal status recorded",`Historical status: ${state}. {points}.`),
      payload:{resource_id:id,points:"9223372036854775807"},created_at:"2026-10-07T00:00:00Z",read_at:null,
    };
    assert.ok(check(row),`${key}: ${JSON.stringify(check.errors)}`);
    assert.ok(!check({...row,content:null}),`${key} requires its immutable snapshot`);
    assert.ok(!check({...row,payload:{resource_id:id,points:null}}),`${key} requires positive points`);
    assert.ok(!check({...row,payload:{resource_id:id,points:"9223372036854775808"}}),`${key} rejects int64 overflow`);
    assert.ok(!check({...row,payload:{...row.payload,reason:"raw reason"}}),`${key} rejects private reason fields`);
  }
  const unknown={id,brand_id:id,member_id:id,event_type:"withdrawal.order.unknown",template_key:"withdrawal.order.unknown",template_version:2,content:content(),payload:{resource_id:id,points:"1"},created_at:"2026-10-07T00:00:00Z",read_at:null};
  assert.ok(!check(unknown));
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
  assert.match(schemas.LotteryNotificationTemplateContent.description,/thirteen event templates require \{points\} in each language body/);
  assert.match(schemas.LotteryNotificationTemplateCopy.properties.title.description,/120 UTF-8 bytes/);
  assert.match(schemas.LotteryNotificationTemplateCopy.properties.body.description,/1200 UTF-8 bytes/);
});
