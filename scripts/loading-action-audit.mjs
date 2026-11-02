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
      const path = `${surface}/src/${file}`
      if (onlyPath && path !== onlyPath) continue
      candidates.push(...loadingActionCandidatesInSource(path, readFileSync(new URL(path, root), 'utf8')))
    }
  }
  return candidates
}

export function loadingActionCandidatesInSource(path, source) {
  if (/@withGlobalLoading\b/.test(source)) throw new Error(`${path}: loading helper must be inside the event expression, not the event name`)
  const parsed = parse(source)
  if (parsed.errors.length) throw new Error(`${path}: ${parsed.errors.map(error => error.message ?? error).join('; ')}`)
  const descriptor = parsed.descriptor
  if (!descriptor.template || !descriptor.scriptSetup) return []
  const script = ts.createSourceFile(path + '.ts', descriptor.scriptSetup.content, ts.ScriptTarget.Latest, true)
  const functions = new Map(), awaited = new Map(), nativeActions = new Set()
  const async = node => node.modifiers?.some(modifier => modifier.kind === ts.SyntaxKind.AsyncKeyword)
  function collect(node) {
    const declaration = ts.isFunctionDeclaration(node) && node.name && async(node) ? node :
      ts.isVariableDeclaration(node) && ts.isIdentifier(node.name) && node.initializer &&
      (ts.isArrowFunction(node.initializer) || ts.isFunctionExpression(node.initializer)) && async(node.initializer) ? node.initializer : null
    if (declaration) {
      const name = node.name.text
      functions.set(name, declaration.parameters)
      const awaits = []
      function collectAwaits(part) {
        if (ts.isAwaitExpression(part)) awaits.push(part.expression)
        ts.forEachChild(part, collectAwaits)
      }
      if (declaration.body) collectAwaits(declaration.body)
      awaited.set(name, awaits)
      // Native clipboard/share panels own their own UI, not application data loading.
      if (declaration.body && /navigator\.(?:clipboard|share)/.test(declaration.body.getText(script))) nativeActions.add(name)
    }
    ts.forEachChild(node, collect)
  }
  collect(script)
  // A local async handler may delegate menu positioning to another local helper.
  // Resolve only chains ending in nextTick; unknown waits and cycles stay audited.
  const domUpdates = new Set()
  let changed = true
  while (changed) {
    changed = false
    for (const [name, awaits] of awaited) {
      if (domUpdates.has(name) || !awaits.length) continue
      if (awaits.every(expression => ts.isCallExpression(expression) && ts.isIdentifier(expression.expression) &&
        (expression.expression.text === 'nextTick' || domUpdates.has(expression.expression.text)))) {
        domUpdates.add(name); changed = true
      }
    }
  }
  for (const name of [...nativeActions, ...domUpdates]) functions.delete(name)
  const candidates = []
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
  return candidates
}
