import assert from 'node:assert/strict'
import { readFileSync, readdirSync } from 'node:fs'
import { test } from 'node:test'

test('association labels never directly render foreign identifiers or role codes',()=>{
 const raw=/\{\{\s*[\w.]+\.(?:brand_id|game_id|period_id|play_id|member_id|agent_id|global_user_id|role_ids|role_codes|brandId|actorId|accountId)\s*\}\}/
 for(const root of ['user-web/src','admin-web/src','platform-web/src'])for(const file of readdirSync(root)){
  if(!file.endsWith('.vue'))continue
  const template=readFileSync(`${root}/${file}`,'utf8').split('</script>')[1]??''
  assert.doesNotMatch(template,raw,`${root}/${file}`)
 }
})

test('name metadata stays outside business DTOs and is documented for all new pages',()=>{
 const shared=readFileSync('shared/src/association-names.ts','utf8')
 assert.match(shared,/WeakMap<object/)
 assert.doesNotMatch(shared,/localStorage|sessionStorage|return id\b|return .*\.code\b/)
 const doc=readFileSync('docs/48-ui-architecture-and-design-system.md','utf8')
 assert.match(doc,/三端关联信息展示/)
 assert.match(doc,/订单号、审计编号/)
 assert.match(doc,/copyAssociationNames/)
})
