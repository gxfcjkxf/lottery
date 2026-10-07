import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { compileTemplate, parse } from 'vue/compiler-sfc'
import ts from 'typescript'

const translatedComponents = ['App', 'AccessManagement', 'AuthSettings', 'MemberProvision', 'BrandOperation', 'BrandPresentation', 'PresentationFields', 'FinanceManagement', 'PointPolicySettings', 'WithdrawalPolicySettings', 'CommissionPolicySettings', 'CommissionCyclesManagement', 'CommissionPaymentsManagement', 'BalanceRepair', 'ReconciliationManagement']
const han = /\p{Script=Han}/u
interface Node { type: number; tag?: string; arg?: { content?: string }; exp?: { content?: string }; content?: string | Node; name?: string; value?: { content: string }; children?: Node[]; props?: Node[] }

describe('declared bilingual administration pages', () => {
  it.each(translatedComponents)('%s compiles and has no untranslated static text or accessible labels', name => {
    const filename = `${name}.vue`
    const source = readFileSync(new URL(`./${filename}`, import.meta.url), 'utf8')
    const descriptor = parse(source, { filename }).descriptor
    const script = ts.createSourceFile(`${name}.ts`, descriptor.scriptSetup?.content ?? '', ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
    const catalog = new Map<string, string>()
    const problems: string[] = []
    function catalogVisit(node: ts.Node) {
      if (ts.isCallExpression(node) && node.expression.getText(script) === 'Object.assign' && node.arguments[0]?.getText(script) === 'englishUi' && node.arguments[1] && ts.isObjectLiteralExpression(node.arguments[1])) {
        for (const entry of node.arguments[1].properties) if (ts.isPropertyAssignment(entry) && ts.isStringLiteral(entry.name) && ts.isStringLiteral(entry.initializer)) {
          if (catalog.has(entry.name.text)) problems.push(`Duplicate translation: ${entry.name.text}`)
          catalog.set(entry.name.text, entry.initializer.text)
        }
      }
      if (ts.isVariableDeclaration(node) && node.name.getText(script) === 'englishUi' && node.initializer && ts.isObjectLiteralExpression(node.initializer)) {
        for (const entry of node.initializer.properties) {
          if (ts.isPropertyAssignment(entry) && ts.isStringLiteral(entry.name) && ts.isStringLiteral(entry.initializer)) {
            if (catalog.has(entry.name.text)) problems.push(`Duplicate translation: ${entry.name.text}`)
            catalog.set(entry.name.text, entry.initializer.text)
          }
        }
      }
      ts.forEachChild(node, catalogVisit)
    }
    catalogVisit(script)
    function checkCopyCalls(text: string) {
      const expression = ts.createSourceFile('copy.ts', text, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
      function visitCall(node: ts.Node) {
        if (ts.isCallExpression(node) && ts.isIdentifier(node.expression) && ['t', 'ui', 'message'].includes(node.expression.text) && node.arguments[0] && ts.isStringLiteral(node.arguments[0]) && han.test(node.arguments[0].text) && node.arguments[0].text !== '简体中文') {
          const key = node.arguments[0].text
          const english = node.expression.text === 'ui' ? catalog.get(key) : node.arguments[1] && ts.isStringLiteral(node.arguments[1]) ? node.arguments[1].text : undefined
          if (!english?.trim() || han.test(english)) problems.push(`Missing English copy: ${key}`)
        }
        ts.forEachChild(node, visitCall)
      }
      visitCall(expression)
    }
    checkCopyCalls(descriptor.scriptSetup?.content ?? '')
    expect(descriptor.template).not.toBeNull()
    const result = compileTemplate({ source: descriptor.template!.content, filename, id: `locale-${name}` })
    expect(result.errors).toEqual([])
    const leftovers: string[] = []
    function visit(node: Node) {
      if (node.type === 5 && typeof (node.content as Node)?.content === 'string') checkCopyCalls((node.content as Node).content as string)
      if (node.type === 7 && node.exp?.content) checkCopyCalls(node.exp.content)
      // TEXT nodes and fixed accessible attributes, not Chinese keys in
      // interpolation expressions or customer-provided values.
      if (node.type === 2 && typeof node.content === 'string' && han.test(node.content) && node.content.trim() !== '简体中文') leftovers.push(node.content.trim())
      if (node.type === 6 && ['aria-label', 'placeholder', 'title', 'alt'].includes(node.name ?? '') && node.value && han.test(node.value.content)) leftovers.push(`${node.name}: ${node.value.content}`)
      if (node.tag === 'option' && (node.children ?? []).some(child => child.type === 5 && /\b(?:t|ui)\s*\(/.test(String((child.content as Node)?.content ?? '')))) {
        const stableValue = (node.props ?? []).some(prop => prop.type === 6 && prop.name === 'value' || prop.type === 7 && prop.name === 'bind' && prop.arg?.content === 'value')
        if (!stableValue) leftovers.push('Translated option must retain an explicit, language-independent value')
      }
      for (const child of node.children ?? []) visit(child)
      for (const prop of node.props ?? []) visit(prop)
    }
    // Parse rather than traversing the optimized codegen tree, which can hide
    // text in compound expressions and static hoists.
    visit(descriptor.template!.ast as unknown as Node)
    expect(leftovers).toEqual([])
    expect(problems).toEqual([])
    expect(source).not.toMatch(/createTreeWalker|localizeStaticText|vLocalizeRoot/)
    expect(source).not.toMatch(/\bv-html\s*=/)
  })
})
