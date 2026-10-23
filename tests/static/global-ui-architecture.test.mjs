import { readFileSync, readdirSync, realpathSync } from 'node:fs'
import { createRequire } from 'node:module'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { loadingActionCandidates } from '../../scripts/loading-action-audit.mjs'
const require = createRequire(new URL('../../user-web/package.json', import.meta.url))
const { parse } = require('vue/compiler-sfc')
const read = path => readFileSync(new URL('../../' + path, import.meta.url), 'utf8')

test('agent management uses design primitives that are also used by the existing management apps', () => {
  const walk = dir => readdirSync(new URL('../../' + dir, import.meta.url), { withFileTypes: true }).flatMap(entry => entry.isDirectory() ? walk(dir + '/' + entry.name) : [dir + '/' + entry.name])
  const files = walk('agent-web/src')
  assert.deepEqual(files.filter(file => file.endsWith('.css')), [])
  for (const file of files.filter(file => file.endsWith('.vue'))) assert.equal(parse(read(file)).descriptor.styles.length, 0, file + ': private style block')
  const entry = read('agent-web/src/main.ts')
  for (const css of ['management', 'controls', 'key-value', 'empty-state', 'login-entry']) assert.ok(entry.includes(`@lottery/shared/${css}.css`))
  for (const file of ['admin-web/src/App.vue', 'platform-web/src/CommissionPanel.vue']) {
    const source = read(file)
    for (const cls of ['console-panel', 'console-stack']) assert.ok(source.includes(cls), file + ': shared primitive not actually used: ' + cls)
  }
  assert.ok(read('admin-web/src/App.vue').includes('@lottery/shared/BrandLockup.vue'))
  assert.ok(read('shared/src/ManagementShell.vue').includes('./BrandLockup.vue'))
  assert.equal(parse(read('shared/src/ManagementShell.vue')).descriptor.styles.length, 0)
})

test('account dialogs, order records and ledger tables are actually shared with existing apps', () => {
  for(const app of ['admin-web','platform-web','agent-web']) {
    const source=read(`${app}/src/App.vue`)
    for(const component of ['AccountProfileDialog','ChangePasswordDialog']) {
      assert.ok(source.includes(`@lottery/shared/${component}.vue`))
      assert.ok(source.includes(`<${component}`))
    }
    assert.doesNotMatch(source, /<DetailModal[^>]+class="console-account-dialog"/)
  }
  for(const file of ['user-web/src/BettingHistory.vue','agent-web/src/pages/MemberBetsPage.vue']) assert.ok(read(file).includes('@lottery/shared/OrderHistoryRecord.vue'))
  for(const file of ['platform-web/src/WalletPanel.vue','agent-web/src/pages/CommissionPage.vue']) assert.ok(read(file).includes('@lottery/shared/ManagementTable.vue'))
  const dialog=read('shared/src/ChangePasswordDialog.vue')
  assert.match(dialog,/v-write-guard="busy"/)
  assert.ok(dialog.includes('WriteFormActions'))
  assert.match(read('shared/src/WriteFormActions.vue'),/:disabled="busy\|\|frozen"/)
})

test('agent UI has no private table, paging, filter, or creation field renderer', () => {
  const pairs=[
    ['ManagementFrame','shared/src/ManagementShell.vue','admin-web/src/App.vue'],
    ['ManagementTable','agent-web/src/pages/MembersPage.vue','admin-web/src/App.vue'],
    ['ManagementTable','agent-web/src/pages/ChildrenPage.vue','platform-web/src/WalletPanel.vue'],
    ['DirectorySearch','agent-web/src/pages/MembersPage.vue','platform-web/src/App.vue'],
    ['ConsoleTabs','agent-web/src/pages/MembersPage.vue','admin-web/src/AgentManagement.vue'],
    ['PaginationControls','agent-web/src/pages/CommissionPage.vue','admin-web/src/App.vue'],
    ['AccountCreateFields','agent-web/src/App.vue','admin-web/src/MemberProvision.vue'],
    ['MemberDetailsFields','agent-web/src/App.vue','admin-web/src/MemberProvision.vue'],
    ['AgentRatioField','agent-web/src/App.vue','admin-web/src/AgentManagement.vue'],
    ['WriteFormActions','agent-web/src/App.vue','shared/src/ChangePasswordDialog.vue'],
    ['SummaryFacts','agent-web/src/pages/MemberBetsPage.vue','shared/src/AccountProfileDialog.vue'],
  ]
  for(const [component,...files] of pairs)for(const file of files)assert.ok(read(file).includes(`<${component}`),`${file}: ${component} must actually be rendered`)
  for(const file of ['MembersPage','ChildrenPage','MemberBetsPage','CommissionPage']) {
    const source=read(`agent-web/src/pages/${file}.vue`)
    assert.doesNotMatch(source,/<(?:table|thead|tbody|th|td|dl|dt|dd)\b/)
    assert.ok(source.includes('<PaginationControls'))
  }
  for(const app of ['user-web','admin-web','platform-web','agent-web']) {
    assert.ok(read(`${app}/index.html`).includes('/favicon.svg?v=20261022'))
    assert.equal(read(`${app}/public/favicon.svg`),read('shared/src/favicon.svg'))
    assert.equal(realpathSync(new URL('../../'+app+'/public/favicon.svg',import.meta.url)),realpathSync(new URL('../../shared/src/favicon.svg',import.meta.url)))
  }
  const agent=read('agent-web/src/App.vue')
  assert.match(agent,/applyBrowserBranding\(t\('title'\),brand.logoText,safeBrandAssetUrl\(brand.faviconUrl\),brand.primary\)/)
  assert.match(agent,/watch\(\[brandContext,locale\]/)
})

test('agent invitations reuse the referral link renderer and the user register protocol', () => {
  const agent=read('agent-web/src/App.vue'),user=read('user-web/src/JoinCodesPanel.vue'),component=read('shared/src/RegistrationLink.vue')
  for(const source of [agent,user])assert.ok(source.includes('@lottery/shared/RegistrationLink.vue'))
  assert.ok(agent.includes("registrationShareLink(import.meta.env.VITE_USER_APP_ORIGIN,'agent',me.value.code)"))
  assert.ok(user.includes("registrationShareLink(window.location.origin,'referral',code.code)"))
  assert.doesNotMatch(agent,/navigator\.clipboard|navigator\.share/)
  assert.ok(component.includes('navigator.clipboard.writeText(props.url)'))
  assert.ok(component.includes('QRCode.toDataURL(props.url'))
  assert.ok(component.includes('download="registration-qr.png"'))
  assert.match(component,/max-width:100%/)
  assert.doesNotMatch(component,/@contextmenu|preventDefault|html2canvas|window\.open/)
})

test('background fetch is not globally intercepted and explicit UI operations own their loading lifecycle', () => {
  const overlay = read('shared/src/LoadingOverlay.vue')
  assert.doesNotMatch(overlay, /window\.fetch|originalFetch|pending\.value/)
  assert.match(overlay, /globalLoadingActive/)
  const loading = read('shared/src/global-loading.ts')
  assert.match(loading, /finally\s*\{\s*foregroundOperations\.value--/)
  assert.deepEqual(loadingActionCandidates().map(({ path, prop }) => `${path}:${prop.loc.start.line}`), [])
  assert.match(read('user-web/src/BettingPanel.vue'), /void loadCatalog\(true\)/)
  assert.match(read('user-web/src/BettingHistory.vue'), /void load\(pageIndex\.value, cursors\.value\[pageIndex\.value\] \?\? null, true\)/)
  assert.doesNotMatch(read('shared/src/session-replacement.ts'), /withGlobalLoading/)
  assert.doesNotMatch(read('user-web/src/HeaderPoints.vue'), /withGlobalLoading/)
})

test('all three apps use the documented platform configuration gate and shared loading implementation', () => {
  for (const app of ['user-web', 'admin-web', 'platform-web']) {
    assert.match(read(`${app}/src/main.ts`), /DisplaySettingsGate/)
    assert.match(read(`${app}/src/${app === 'user-web' ? 'Page' : 'App'}.vue`), /GlobalLoadingOverlay/)
  }
  const gate = read('shared/src/DisplaySettingsGate.vue')
  assert.match(gate, /<LoadingOverlay v-else/)
  assert.doesNotMatch(gate, /<p v-else[^>]*role="status"/)
  const doc = read('docs/48-ui-architecture-and-design-system.md')
  for (const symbol of ['formatDisplayDateTime', 'v-global-loading', 'DisplaySettingsGate', '/api/v1/display-settings']) assert.ok(doc.includes(symbol))
})
test('business components cannot introduce browser-default timestamp display', () => {
  for (const dir of ['user-web/src', 'admin-web/src', 'platform-web/src', 'shared/src']) {
    for (const name of readdirSync(new URL('../../' + dir, import.meta.url)).filter(n => n.endsWith('.vue'))) {
      const source = read(`${dir}/${name}`)
      assert.doesNotMatch(source, /new Date\([^\n]*?\)\.toLocale(?:String|DateString|TimeString)\(/, `${dir}/${name}`)
      assert.doesNotMatch(source, /dateStyle\s*:|timeStyle\s*:/, `${dir}/${name}`)
      assert.doesNotMatch(source, /<input\b[^>]*type="(?:date|time|datetime-local)"/, `${dir}/${name}: use PlatformTemporalInput`)
    }
  }
})
test('pure page loading placeholders must publish through the global status directive', () => {
  for (const dir of ['user-web/src', 'admin-web/src', 'platform-web/src', 'shared/src']) {
    for (const file of readdirSync(new URL('../../' + dir, import.meta.url)).filter(f => f.endsWith('.vue') && !['LoadingOverlay.vue', 'DisplaySettingsGate.vue'].includes(f))) {
      const ast = parse(read(`${dir}/${file}`)).descriptor.template?.ast
      function visit(node) {
        if (node.type === 1) {
          const condition = node.props.find(p => p.type === 7 && ['if', 'else-if'].includes(p.name))?.exp?.content ?? ''
          const progress = [...condition.matchAll(/[a-zA-Z_$][\w$.]*/g)].some(m => /(loading|busy|reading|refreshing)/i.test(m[0]) && condition.slice(0, m.index).trimEnd().slice(-1) !== '!') || /===\s*['"]loading['"]/.test(condition)
          const plain = node.children.every(c => [2, 3, 5].includes(c.type) || node.tag === 'tr' && c.type === 1 && c.tag === 'td' && c.children.every(child => [2, 3, 5].includes(child.type)))
          if (progress && plain && !['button', 'input', 'option', 'template', 'h1', 'h2', 'h3'].includes(node.tag) && node.children.some(c => c.type !== 2 || c.content.trim())) assert.ok(node.props.some(p => p.type === 7 && p.name === 'global-loading'), `${dir}/${file}: ${condition}`)
        }
        for (const child of node.children ?? []) visit(child)
      }
      if (ast) visit(ast)
    }
  }
})
test('account refresh starts loading rather than showing an empty error container', () => {
  const page = read('user-web/src/Page.vue')
  assert.match(page, /const profileLoading = ref\(true\)/)
  assert.match(page, /<p v-if="profileError" class="auth-error"/)
  assert.doesNotMatch(page, /<div v-if="profileLoading" class="detail-panel"/)
})
