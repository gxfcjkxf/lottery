import { readFileSync } from 'node:fs'
import { compileScript, parse } from 'vue/compiler-sfc'
import ts from 'typescript'
import SiteIcon from '../../shared/src/SiteIcon'
import * as GlobalLoading from '../../shared/src/global-loading'
import * as Vue from 'vue'
import type { Component } from 'vue'
import * as Localization from '../../shared/src/localization'
import * as PaginationCatalog from '../../shared/src/pagination-catalog'

// Use the actual shared controls in business component tests as well as browsers.
const filename = '../../shared/src/TablePagination.vue'
const { descriptor } = parse(readFileSync(new URL(filename, import.meta.url), 'utf8'))
const compiled = ts.transpileModule(compileScript(descriptor, { id: filename, inlineTemplate: true }).content, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext },
}).outputText
const modules: Record<string, unknown> = { './SiteIcon': SiteIcon, './global-loading': GlobalLoading, vue: Vue, './localization': Localization, './pagination-catalog': PaginationCatalog }
const body = compiled.replace(/^import SelectControl from .*$/m, 'const SelectControl = { render: () => null };').replace(/^import\s+\{([\s\S]*?)\}\s+from\s+['"]([^'"]+)['"];?\s*$/gm,
  (_match, bindings: string, specifier: string) => `const {${bindings.replace(/\s+as\s+/g, ': ')}} = __modules[${JSON.stringify(specifier)}];`)
  .replace(/^import SiteIcon from .*$/m, 'const SiteIcon = __modules["./SiteIcon"];')
  .replace(/export\s+default\s+/, 'return ')
export const TablePaginationTestComponent = new Function('__modules', body)(modules) as Component
