import { createRequire } from 'node:module'
const require = createRequire(new URL('../shared/package.json', import.meta.url))
const ts = require('typescript')
const { parse } = require('vue/compiler-sfc')
const settingsPage = 'platform-web/src/PlatformSettingsPanel.vue'
const zoneControl = 'shared/src/TimeZoneSelect.vue'
const zoneCatalog = 'shared/src/time-zones.ts'
const timezoneCopy = /时区|\b(?:UTC|GMT|time\s*zone|timezone)\b|\b(?:Africa|America|Antarctica|Arctic|Asia|Atlantic|Australia|Europe|Indian|Pacific)\/[A-Za-z_]+(?:\/[A-Za-z_]+)?|^[+-]\d{2}:\d{2}$/i
const settingsKeys = /^(?:common\.timeZone(?:[A-Z]\w*)?)$/
const catalogFile = /(?:catalog|i18n|permission-labels|archive-report|feed-mapping|platform-lottery)\.(?:ts|vue)$/

export function terminologyIssues(file, source) {
  const issues = []
  const report = (line, text) => issues.push(`${file}:${line}: ${text}`)
  function check(text, line, key = '', isCopy = true) {
    if (/策略|数据保留|保留策略/.test(text) || (isCopy && /\bstrateg(?:y|ies)\b/i.test(text))) report(line, text)
    if (isCopy && /\bpolic(?:y|ies)\b/i.test(text) && !/^common\.(?:privacyPolicy(?:Upper)?|policyPlaceholder|policyNotice)$/.test(key)) report(line, text)
    const platformZone = file === settingsPage || file === zoneCatalog || (file === 'platform-web/src/i18n.ts' && settingsKeys.test(key))
    if (!platformZone && (isCopy ? timezoneCopy : /时区/).test(text)) report(line, text)
  }
  function script(content, offset = 0, isCopy = catalogFile.test(file)) {
    const tree = ts.createSourceFile(file, content, ts.ScriptTarget.Latest, true)
    function visit(node) {
      if (ts.isStringLiteralLike(node) || ts.isTemplateHead(node) || ts.isTemplateMiddle(node) || ts.isTemplateTail(node)) {
        const parent = node.parent
        const line = tree.getLineAndCharacterOfPosition(node.getStart()).line + offset + 1
        if (ts.isImportDeclaration(parent) || ts.isExportDeclaration(parent)) {
          if (/TimeZoneSelect(?:\.vue)?$/.test(node.text) && file !== settingsPage) report(line, '时区选择器只能由平台设置引用')
          if (/(?:^|\/)time-zones$/.test(node.text) && file !== zoneControl) report(line, '时区目录只能由平台设置的时区控件引用')
        } else if (!((ts.isPropertyAssignment(parent) || ts.isPropertySignature(parent)) && parent.name === node)) {
          const key = ts.isPropertyAssignment(parent) ? parent.name.getText(tree).replace(/^['"]|['"]$/g, '') : ''
          let copyContext = isCopy || /^(?:title|label|placeholder|alt|message|description|text)$/.test(key)
          for (let owner = parent; owner && !copyContext; owner = owner.parent) {
            if (ts.isVariableDeclaration(owner) && /(?:copy|catalog|messages|labels|lang|options)/i.test(owner.name.getText(tree))) copyContext = true
          }
          check(node.text, line, key, copyContext)
        }
      }
      ts.forEachChild(node, visit)
    }
    visit(tree)
  }
  if (!file.endsWith('.vue')) script(source)
  else {
    const { descriptor, errors } = parse(source, { filename: file })
    if (errors.length) throw new Error(`${file}: ${errors.join(', ')}`)
    for (const block of [descriptor.script, descriptor.scriptSetup]) if (block) script(block.content, block.loc.start.line - 1)
    function template(node) {
      const line = node.loc?.start.line ?? 1
      if (node.type === 2) check(node.content, line)
      if (node.type === 5) {
        script(node.content.content, line - 1, true)
        if (file !== settingsPage && /^(?:[\w$]+\.)*(?:timezone|timeZone)\s*$/.test(node.content.content.trim())) report(line, '平台设置以外不得直接展示时区值')
      }
      if (node.type === 1) {
        if (node.tag === 'TimeZoneSelect' && file !== settingsPage) report(line, '时区选择器只能出现在平台设置')
        for (const prop of node.props) {
          if (prop.type === 6 && prop.value && /^(?:title|placeholder|label|alt|aria-label)$/.test(prop.name)) check(prop.value.content, prop.loc.start.line)
          if (prop.type === 7 && prop.exp && prop.name === 'bind' && /^(?:title|placeholder|label|alt|aria-label|text)$/.test(prop.arg?.content ?? '')) script(prop.exp.content, prop.loc.start.line - 1, true)
        }
      }
      for (const child of node.children ?? []) template(child)
    }
    if (descriptor.template) template(descriptor.template.ast)
  }
  return issues
}
