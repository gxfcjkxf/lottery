import {test,expect,type APIResponse} from '@playwright/test';
const origin=process.env.TEST_ADMIN_ORIGIN??'http://localhost:5174';
const userOrigin=process.env.TEST_USER_ORIGIN??'http://localhost:5173';
const admin=origin+'/api/v1/admin',user=userOrigin+'/api/v1';
const brand='0199a000-0000-7000-8000-000000000001';
type Content={en:{title:string;body:string};'zh-CN':{title:string;body:string}};
type Message={id:string;event_type:string;template_version:number;content:Content;payload:{resource_id:string;points:string};read_at:string|null};
async function data<T>(response:Pick<APIResponse,'text'|'status'>,status=200):Promise<T>{const raw=await response.text();expect(response.status(),raw).toBe(status);const body=JSON.parse(raw);expect(body.success).toBe(true);return body.data as T;}

test('real reward inbox preserves pending and reversal facts, template snapshots and exact read receipts',async({page},info)=>{
 test.skip(process.env.REWARD_UI_FIXTURE_CONFIRM!=='owned_synthetic_database'||process.env.REWARD_NOTIFICATION_WORKER_ENABLED!=='true'||!process.env.TEST_REWARD_ADMIN_PASSWORD,'Requires the owned reward database and real inbox worker');
 test.setTimeout(60_000);expect(process.env.REWARD_UI_VIEWPORT).toBe(info.project.name);
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message));
 const get=async<T>(path:string)=>data<T>(await page.request.get(admin+path,{headers:{'X-Brand-ID':brand}}));
 const write=async<T>(path:string,body:unknown,actor?:string)=>data<T>(await page.request.post(admin+path,{headers:{Origin:origin,'X-Brand-ID':brand,'Idempotency-Key':crypto.randomUUID(),...(actor?{'X-Reward-Actor-ID':actor}:{})},data:body}),path==='/reward-orders'?201:200);
 await data(await page.request.post(admin+'/auth/login',{headers:{Origin:origin,'Idempotency-Key':crypto.randomUUID()},data:{identifier:'reward_s22_operator',password:process.env.TEST_REWARD_ADMIN_PASSWORD}}));
 const {account}=await get<{account:{id:string}}>('/me');
 const registered=await data<{member:{id:string}}>(await page.request.post(user+'/auth/register',{headers:{Origin:userOrigin,'Idempotency-Key':crypto.randomUUID()},data:{username:`rwmsg_${info.project.name}_${crypto.randomUUID().replaceAll('-','').slice(0,6)}`,password:'owned-reward-notification-user-2026',privacy_policy_version:'dev-1',service_terms_version:'dev-1'}}),201);
 const member=registered.member.id;
 const templates=await get<{items:Array<{key:string;version:number;content:Content}>}>('/notification-templates');expect(templates.items).toHaveLength(22);
 expect(templates.items.filter(item=>item.key.startsWith('draw.result.')).map(item=>item.key).sort()).toEqual(['draw.result.corrected','draw.result.published']);
 const original=templates.items.find(item=>item.key==='reward.order.revocation_pending')!;expect(original).toBeTruthy();
 const copy:Content={en:{title:'Edited reward message',body:'Edited text says {points} points moved.'},'zh-CN':{title:'编辑的奖励消息',body:'编辑文本称 {points} 积分已移动。'}};
 const publish=async(content:Content,version:number)=>data<{version:number}>(await page.request.put(admin+'/notification-templates/reward.order.revocation_pending',{headers:{Origin:origin,'X-Brand-ID':brand,'Idempotency-Key':crypto.randomUUID()},data:{version,content,reason:'Owned synthetic test proves mandatory pending fact survives custom copy'}}));
 const revision=await publish(copy,original.version);
 const order=await write<{id:string}>('/reward-orders',{member_id:member,points:'40',reason:'private browser reward grant reason'},account.id);
 const hold=await write<{id:string}>(`/wallets/${member}/freeze`,{points:'40',reason:'private browser hold reason'});
 expect((await write<{state:string}>(`/reward-orders/${order.id}/revoke`,{version:1,reason:'private operator pending reason'},account.id)).state).toBe('revocation_pending');
 await write(`/wallets/${member}/unfreeze`,{entry_id:hold.id,reason:'explicit browser hold release'});
 expect((await write<{state:string}>(`/reward-orders/${order.id}/retry-revocation`,{version:2,reason:'explicit original browser reversal'},account.id)).state).toBe('revoked');
 const inbox=async()=>data<{items:Message[]}>(await page.request.get(user+'/notifications?limit=100'));
 await expect.poll(async()=>(await inbox()).items.filter(item=>item.event_type.startsWith('reward.order.')).length,{timeout:10_000}).toBe(3);
 const messages=(await inbox()).items.filter(item=>item.event_type.startsWith('reward.order.'));
 expect(messages.map(item=>item.event_type).sort()).toEqual(['reward.order.granted','reward.order.revocation_pending','reward.order.revoked']);
 const pending=messages.find(item=>item.event_type==='reward.order.revocation_pending')!;expect(pending.template_version).toBe(revision.version);expect(pending.content).toEqual(copy);
 for(const message of messages){expect(message.payload).toEqual({resource_id:order.id,points:'40'});expect(message.content).toBeTruthy();}
 const economic=async()=>JSON.stringify({wallet:await get(`/wallets/${member}`),ledger:await get(`/wallets/${member}/ledger?limit=100&offset=0`)});
 const before=await economic();await publish(original.content,revision.version);
 expect((await inbox()).items.find(item=>item.id===pending.id)).toEqual(pending);
 await page.goto(userOrigin+'/notifications');const panel=page.locator('.notifications-panel');
 await expect(panel.getByRole('heading',{name:'Edited reward message',exact:true})).toBeVisible();
 await expect(panel.locator('.notification-protected-note').filter({hasText:'No points moved in this attempt'})).toHaveCount(1);
 await expect(panel).not.toContainText('private browser');await expect(panel).not.toContainText('private operator');
 await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
 await panel.screenshot({path:info.outputPath('reward-inbox-panel.png')});
 await page.getByRole('button',{name:'Switch to Chinese',exact:true}).click();
 await expect(panel.locator('.notification-protected-note').filter({hasText:'本次没有积分变动'})).toHaveCount(1);
 await expect(panel.getByRole('heading',{name:'编辑的奖励消息',exact:true})).toBeVisible();
 await page.getByRole('button',{name:'Switch to English',exact:true}).click();
 const writes:Array<{body:string|null;key:string|undefined}>=[];
 await page.route('**/api/v1/notifications/read',async route=>{writes.push({body:route.request().postData(),key:route.request().headers()['idempotency-key']});const response=await route.fetch();await data(response);if(writes.length===1)await route.abort('failed');else{expect(writes[1]).toEqual(writes[0]);await route.fulfill({response});}});
 await panel.getByRole('button',{name:'Mark this page read',exact:true}).click();await expect(panel.getByRole('button',{name:'Retry the same request',exact:true})).toBeVisible();
 await panel.getByRole('button',{name:'Reload',exact:true}).click();await expect(panel.getByRole('button',{name:'Retry the same request',exact:true})).toBeVisible();
 await panel.getByRole('button',{name:'Retry the same request',exact:true}).click();await expect(panel.getByRole('button',{name:'Retry the same request',exact:true})).toHaveCount(0);
 expect(writes).toHaveLength(2);expect(await economic()).toBe(before);
 const after=(await inbox()).items.filter(item=>item.event_type.startsWith('reward.order.'));expect(after).toHaveLength(3);
 for(const message of after){expect(message.read_at).not.toBeNull();expect(message.content).toEqual(messages.find(item=>item.id===message.id)?.content);}
 expect(errors).toEqual([]);
});
