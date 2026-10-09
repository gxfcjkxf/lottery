import {test} from "node:test";
import assert from "node:assert/strict";
import {execFileSync} from "node:child_process";
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
  "reward.order.granted","reward.order.revocation_pending","reward.order.revoked",
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

test("the actual backend JSON preserves the previous defaults and contains exactly twenty-two keys",()=>{
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
  assert.deepEqual(defaults["commission.paid"],{
    en:{title:"Commission credit recorded",body:"Historical record: {points} points were credited to your commission wallet. This records a past credit, not new income or an external payment forecast. Check your current wallet balance; this record is retained."},
    "zh-CN":{title:"佣金入账记录",body:"历史记录：{points} 积分曾记入佣金钱包。此记录表示过去的入账，不是新收入预测或外部付款承诺。请查看当前钱包余额；此记录会保留。"},
  });
  assert.deepEqual(defaults["commission.adjusted"],{
    en:{title:"Commission adjustment recorded",body:"Historical record: a commission adjustment of {points} points was recorded. This records a past adjustment, not new income or an external payment forecast. Check your current wallet balance; this record is retained."},
    "zh-CN":{title:"佣金调整记录",body:"历史记录：曾调整 {points} 积分。此记录表示过去的调整，不是新收入预测或外部付款承诺。请查看当前钱包余额；此记录会保留。"},
  });
  assert.deepEqual(defaults["commission.corrected"],{
    en:{title:"Commission correction recorded",body:"Historical record: a commission correction of {points} points was posted to your commission available balance. Positive points record a past additional credit; negative points record a past recovery. This is not your current balance, new income or an external payment. Check your current wallet; this record is retained."},
    "zh-CN":{title:"佣金更正记录",body:"历史记录：佣金可用积分曾发生 {points} 积分更正。正数表示过去的补发，负数表示过去的追回；不代表当前余额、新收入或外部付款。请查看当前钱包；此记录会保留。"},
  });
  for(const key of ["draw.result.published","draw.result.corrected"]){
    const {en,"zh-CN":zh}=defaults[key];
    assert.match(en.body,/Historical result ID \{resource_id\}/);
    assert.match(en.body,/not a guarantee of a win or prize payment/i);
    assert.match(en.body,/users cannot edit this notice/i);
    assert.match(zh.body,/历史开奖结果 ID：\{resource_id\}/);
    assert.match(zh.body,/不代表中奖或派奖保证/);
    assert.match(zh.body,/用户不能编辑此通知/);
    for(const locale of [en,zh]){
      assert.doesNotMatch(locale.title,/\{points\}/);
      assert.doesNotMatch(locale.body,/\{points\}/);
      assert.match(locale.body,/\{resource_id\}/);
    }
  }
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

test("SQL 0060 adds the corrected default and guards only real applied correction targets",()=>{
  const migration=readFileSync(new URL("../../backend/migrations/0060_commission_correction_notifications.up.sql",import.meta.url),"utf8");
  const literal=/defaults\s*:=\s*notification_template_defaults\(\)\s*\|\|\s*'((?:[^']|'')*)'::jsonb/.exec(migration);
  assert.ok(literal,"SQL 0060 must extend the existing defaults with a JSON literal");
  const sqlDefaults=JSON.parse(literal[1].replaceAll("''","'"));
  assert.deepEqual(sqlDefaults,{"commission.corrected":defaults["commission.corrected"]});
  assert.match(migration,/CREATE UNIQUE INDEX commission_correction_notification_once[\s\S]*?WHERE event_type='commission\.corrected'/);
  assert.match(migration,/CREATE TRIGGER commission_correction_notification AFTER UPDATE ON commission_correction_execution_targets/);
  assert.match(migration,/OLD\.state='pending' AND NEW\.state='applied' AND NEW\.delta_points<>0/);
  assert.match(migration,/CREATE CONSTRAINT TRIGGER commission_correction_notification_commit AFTER UPDATE[\s\S]*?DEFERRABLE INITIALLY DEFERRED/);
  assert.match(migration,/CREATE TRIGGER guarded_commission_correction_notification_outbox BEFORE INSERT OR UPDATE OR DELETE ON outbox_events/);
  assert.match(migration,/to_jsonb\(NEW\)-'published_at'[\s\S]*?correction notification event immutable except published_at/);
  assert.match(migration,/resource_id',NEW\.id::text/);
});

test("actual backend JSON and SQL 0056 each contain three reward defaults with the required historical meaning",()=>{
  const rewardKeys=["reward.order.granted","reward.order.revocation_pending","reward.order.revoked"];
  const migration=readFileSync(new URL("../../backend/migrations/0056_reward_notifications.up.sql",import.meta.url),"utf8");
  const literal=/defaults\s*:=\s*notification_template_defaults\(\)\s*\|\|\s*'((?:[^']|'')*)'::jsonb/.exec(migration);
  assert.ok(literal,"SQL 0056 must extend the existing defaults with a JSON literal");
  const sqlDefaults=JSON.parse(literal[1].replaceAll("''","'"));
  const check=validate("LotteryNotificationTemplateContent");
  for(const [label,source] of [["actual backend JSON",defaults],["SQL 0056",sqlDefaults]]){
    assert.deepEqual(Object.keys(source).filter(key=>key.startsWith("reward.order.")).sort(),rewardKeys,label);
    for(const key of rewardKeys){
      assert.ok(Object.hasOwn(source,key),`${label} owns ${key}`);
      assert.ok(check(source[key]),`${label} ${key}: ${JSON.stringify(check.errors)}`);
      for(const locale of ["en","zh-CN"])assert.match(source[key][locale].body,/\{points\}/,`${label} ${key} ${locale} retains exact point facts`);
    }
    for(const key of ["reward.order.granted","reward.order.revoked"]){
      const {en,"zh-CN":zh}=source[key];
      assert.match(en.body,/historical.*past/i);
      assert.match(en.body,/gift available balance/i);
      assert.match(en.body,/not.*current wallet balance.*external payment/i);
      assert.match(zh.body,/历史记录.*过去/);
      assert.match(zh.body,/赠送可用积分/);
      assert.match(zh.body,/不代表当前钱包余额或外部付款/);
    }
    assert.match(source["reward.order.granted"].en.body,/credited/i);
    assert.match(source["reward.order.granted"]["zh-CN"].body,/记入/);
    assert.match(source["reward.order.revoked"].en.body,/full original reward.*reversed/i);
    assert.match(source["reward.order.revoked"]["zh-CN"].body,/原奖励全额.*撤销/);
    const {en,"zh-CN":zh}=source["reward.order.revocation_pending"];
    assert.match(en.body,/full.*reversal.*requested.*awaiting operator/i);
    assert.match(en.body,/no points (?:have )?moved/i);
    assert.match(en.body,/(?:does not|no)[^.]*retry[^.]*unfreeze[^.]*(?:deduct|debit)[^.]*automatically/i);
    assert.match(zh.body,/历史记录.*申请全额.*待运营处理/);
    assert.match(zh.body,/没有积分变动/);
    assert.match(zh.body,/不会自动重试、解冻或扣除/);
  }
});

test("SQL 0069 adds only the two draw defaults, keeps validator OID and extends event keys",()=>{
  const migration=readFileSync(new URL("../../backend/migrations/0069_draw_notification_templates.up.sql",import.meta.url),"utf8");
  const literal=/defaults\s*:=\s*notification_template_defaults\(\)\s*\|\|\s*'((?:[^']|'')*)'::jsonb/.exec(migration);
  assert.ok(literal,"SQL 0069 must extend the existing defaults with a JSON literal");
  const sqlDefaults=JSON.parse(literal[1].replaceAll("''","'"));
  assert.deepEqual(sqlDefaults,{"draw.result.published":defaults["draw.result.published"],"draw.result.corrected":defaults["draw.result.corrected"]});
  assert.match(migration,/CREATE OR REPLACE FUNCTION %I\.notification_template_defaults\(\)/);
  assert.match(migration,/CREATE OR REPLACE FUNCTION valid_notification_template_content\(/);
  assert.match(migration,/p\.proname IN\('notification_template_defaults','valid_notification_template_content'\)/);
  assert.match(migration,/ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp/);
  assert.match(migration,/OR title ~ '\[\[:cntrl:\]<>\]' OR replace\(replace\(body,chr\(10\),''\),chr\(9\),''\) ~ '\[\[:cntrl:\]<>\]'/);
  assert.match(migration,/position\('\{resource_id\}' IN body\)=0/);
  assert.match(migration,/position\('\{points\}' IN title\|\|body\)>0/);
  assert.match(migration,/INSERT INTO notification_templates[\s\S]*?WHERE d\.key IN\('draw\.result\.published','draw\.result\.corrected'\)/);
  assert.match(migration,/INSERT INTO notification_template_revisions[\s\S]*?WHERE template_key IN\('draw\.result\.published','draw\.result\.corrected'\)/);
  assert.match(migration,/ALTER TABLE notifications DROP CONSTRAINT notifications_event_type_check/);
  assert.ok(migration.includes("'draw.result.published','draw.result.corrected'));"));
  assert.doesNotMatch(migration,/CREATE OR REPLACE FUNCTION enqueue_in_app_event/);
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
  assert.ok(check({...row,event_type:"recharge.confirmed",template_key:"recharge.confirmed",template_version:2,content:content("Recharge","Added {points} points."),payload:{resource_id:id,points:"1"}}));
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

test("commission notifications expose only resource and signed point facts",()=>{
  const check=validate("LotteryNotification");
  const paid={id,brand_id:id,member_id:id,event_type:"commission.paid",template_key:"commission.paid",template_version:2,content:content("Commission credit recorded","Historical credit: {points}."),payload:{resource_id:id,points:"9223372036854775807"},created_at:"2026-10-07T00:00:00Z",read_at:null};
  assert.ok(check(paid),JSON.stringify(check.errors));
  assert.ok(!check({...paid,payload:{...paid.payload,points:"-1"}}));
  assert.ok(!check({...paid,payload:{...paid.payload,ledger_entry_id:id}}));
  const adjusted={...paid,event_type:"commission.adjusted",template_key:"commission.adjusted",content:content("Commission adjustment recorded","Historical adjustment: {points}."),payload:{resource_id:id,points:"-9223372036854775808"}};
  assert.ok(check(adjusted),JSON.stringify(check.errors));
  assert.ok(check({...adjusted,payload:{...adjusted.payload,points:"9223372036854775807"}}));
  for(const points of ["0","-0","+1","01","-01","9223372036854775808","-9223372036854775809"]){
    assert.ok(!check({...adjusted,payload:{...adjusted.payload,points}}),`accepted invalid signed amount ${points}`);
  }
  const corrected={...adjusted,event_type:"commission.corrected",template_key:"commission.corrected",content:content("Commission correction recorded","Historical correction: {points}."),payload:{resource_id:id,points:"-1"}};
  assert.ok(check(corrected),JSON.stringify(check.errors));
  assert.ok(check({...corrected,payload:{...corrected.payload,points:"9223372036854775807"}}));
  assert.ok(!check({...corrected,payload:{...corrected.payload,resource_id:"correction-id"}}));
  for(const field of ["member_id","ledger_entry_id","target_id","audit_log_id","version"]){
    assert.ok(!check({...corrected,payload:{...corrected.payload,[field]:id}}),`commission.corrected exposes private ${field}`);
  }
  for(const points of ["0","-0","+1","01","-01","9223372036854775808","-9223372036854775809"]){
    assert.ok(!check({...corrected,payload:{...corrected.payload,points}}),`corrected accepted invalid signed amount ${points}`);
  }
  assert.ok(!check({...adjusted,payload:{...adjusted.payload,created_by:id}}));
  assert.ok(!check({...corrected,payload:{...corrected.payload,target_id:id}}));
  for (const row of [paid, adjusted, corrected]) assert.ok(!check({...row,template_version:1,content:null}), `${row.event_type} is never a legacy snapshotless record`);
});

test("draw notifications have closed typed payloads and require immutable content",()=>{
  const check=validate("LotteryNotification");
  const at="2026-10-07T00:00:00Z";
  const draw={game_id:id,period_id:id,period_no:"20261007001",result:{regular:[0,12,999999,1000000],special:[0],digits:[]},drawn_at:at,previous_draw_id:null};
  const published={id,brand_id:id,member_id:id,event_type:"draw.result.published",template_key:"draw.result.published",template_version:1,content:defaults["draw.result.published"],payload:{resource_id:id,points:null,draw},created_at:at,read_at:null};
  assert.ok(check(published),JSON.stringify(check.errors));
  assert.ok(!check({...published,content:null}),"draw v1 requires a content snapshot");
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
    const row={id,brand_id:id,member_id:id,event_type:key,template_key:key,template_version:1,content:defaults[key],payload:{resource_id:id,points:"9223372036854775807"},created_at:"2026-10-07T00:00:00Z",read_at:null};
    assert.ok(check(row),`${key}: ${JSON.stringify(check.errors)}`);
    for(const content of [null,undefined])assert.ok(!check({...row,content}),`${key} requires snapshot content for v1`);
    for(const points of [0,-1,1,"0","-1","01","9223372036854775808",null,undefined])assert.ok(!check({...row,payload:{resource_id:id,points}}),`${key} rejects ${String(points)}`);
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
  assert.match(schemas.LotteryNotificationTemplateContent.description,/nineteen event templates require \{points\} in each language body/);
  assert.match(schemas.LotteryNotificationTemplateCopy.properties.title.description,/120 UTF-8 bytes/);
  assert.match(schemas.LotteryNotificationTemplateCopy.properties.body.description,/1200 UTF-8 bytes/);
});
