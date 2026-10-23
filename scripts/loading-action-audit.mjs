import { readFileSync, readdirSync } from 'node:fs'
import { createRequire } from 'node:module'
const root = new URL('../', import.meta.url)
const require = createRequire(new URL('user-web/package.json', root))
const { parse } = require('vue/compiler-sfc')
const ts = require('typescript')

// Only explicit UI handlers are foreground operations. Timer/watch callbacks are
// deliberately not instrumented: request URLs and global fetch are not a UI intent.
export function loadingActionCandidates(onlyPath) {
  const candidates = []
  for (const surface of ['user-web', 'admin-web', 'platform-web', 'shared']) {
    for (const file of readdirSync(new URL(`${surface}/src/`, root)).filter(name => name.endsWith('.vue'))) {
      const path = `${surface}/src/${file}`, source = readFileSync(new URL(path, root), 'utf8')
      if (onlyPath && path !== onlyPath) continue
      if (/@withGlobalLoading\b/.test(source)) throw new Error(`${path}: loading helper must be inside the event expression, not the event name`)
      const parsed = parse(source)
      if (parsed.errors.length) throw new Error(`${path}: ${parsed.errors.map(error => error.message ?? error).join('; ')}`)
      const descriptor = parsed.descriptor
      if (!descriptor.template || !descriptor.scriptSetup) continue
      const script = ts.createSourceFile(file + '.ts', descriptor.scriptSetup.content, ts.ScriptTarget.Latest, true)
      const functions = new Map()
      const async = node => node.modifiers?.some(modifier => modifier.kind === ts.SyntaxKind.AsyncKeyword)
      function collect(node) {
        // Native clipboard/share panels own their own UI, not application data loading.
        const nativeDeviceAction = node.body && /navigator\.(?:clipboard|share)/.test(node.body.getText(script))
        const awaits = []
        function collectAwaits(part) {
          if (ts.isAwaitExpression(part)) awaits.push(part.expression)
          ts.forEachChild(part, collectAwaits)
        }
        if (node.body) collectAwaits(node.body)
        // Waiting for Vue to position a menu is local UI work, not a data request.
        const domUpdateOnly = awaits.length > 0 && awaits.every(expression => ts.isCallExpression(expression) && ts.isIdentifier(expression.expression) && expression.expression.text === 'nextTick')
        if (ts.isFunctionDeclaration(node) && node.name && async(node) && !nativeDeviceAction && !domUpdateOnly) functions.set(node.name.text, node.parameters)
        if (ts.isVariableDeclaration(node) && ts.isIdentifier(node.name) && node.initializer && (ts.isArrowFunction(node.initializer) || ts.isFunctionExpression(node.initializer)) && async(node.initializer)) functions.set(node.name.text, node.initializer.parameters)
        ts.forEachChild(node, collect)
      }
      collect(script)
      function visit(node) {
        if (node.type === 1) for (const prop of node.props) {
          if (prop.type !== 7 || prop.name !== 'on' || !prop.exp || prop.exp.content.includes('withGlobalLoading')) continue
          const expression = prop.exp.content.trim()
          const handler = ts.createSourceFile('handler.ts', `async function handler(){${expression}}`, ts.ScriptTarget.Latest, true)
          const calls = []
          function find(part) {
            if (ts.isCallExpression(part) && ts.isIdentifier(part.expression) && functions.has(part.expression.text)) calls.push(part)
            ts.forEachChild(part, find)
          }
          find(handler)
          const method = functions.get(expression)
          const refresh = node.tag === 'button' && node.props.some(p => p.type === 7 && p.name === 'refresh-button')
          if (method || calls.length || prop.arg?.content === 'submit' || refresh) candidates.push({ path, source, descriptor, prop, expression, functions, method, handler, calls })
        }
        for (const child of node.children ?? []) visit(child)
      }
      visit(descriptor.template.ast)
    }
  }
  return candidates
}
