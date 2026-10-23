import { readFileSync } from 'node:fs'
import { compileScript, parse } from 'vue/compiler-sfc'
import ts from 'typescript'
import * as Vue from 'vue'
import * as Catalog from '../../shared/src/business-details-catalog'
import * as Display from '../../shared/src/display-settings'
const descriptor = parse(readFileSync(new URL('../../shared/src/BusinessDetails.vue', import.meta.url), 'utf8'), { filename: 'BusinessDetails.vue' }).descriptor
const script = compileScript(descriptor, { id: 'business-details-test', inlineTemplate: true }).content
const js = ts.transpileModule(script, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText
const source = js.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, bindings: string, specifier: string) => `const {${bindings.replace(/\s+as\s+/g, ': ')}}=modules[${JSON.stringify(specifier)}];`).replace(/export\s+default\s+/, 'return ')
export const BusinessDetailsTestComponent = new Function('modules', source)({ vue: Vue, './business-details-catalog': Catalog, './display-settings': Display }) as Vue.Component
