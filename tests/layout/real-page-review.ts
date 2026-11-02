import { expect, type Page, type TestInfo } from '@playwright/test'
import { expectNoPageOverflow, origins } from './fixture'

// Only application entry points qualify for whole-page acceptance. Component
// harnesses remain useful for local state tests, but cannot satisfy this check.
export async function expectRealEntry(page: Page, surface: 'brand' | 'platform' | 'user') {
  expect(new URL(page.url()).origin).toBe(origins[surface])
  const resources = await page.evaluate(() => performance.getEntriesByType('resource').map(entry => entry.name))
  expect(resources.some(url => new URL(url).pathname === '/src/main.ts')).toBe(true)
  const foreignStyles = resources.filter(url => /(?:admin|platform|user|agent)-web\/src\/(?:style|directory|workspace)\.css/.test(url))
  expect(foreignStyles.filter(url => !url.includes(`/${surface === 'brand' ? 'admin' : surface}-web/`))).toEqual([])
}

export async function expectListLayout(page: Page) {
  await expectNoPageOverflow(page)
  const issues = await page.evaluate(() => {
    const issues: string[] = []
    const visible = (el: Element) => el.getBoundingClientRect().width > 1 && el.getBoundingClientRect().height > 1
    const pageTitle=document.querySelector('.management-console .breadcrumbs b,.management-console .breadcrumb button:last-child')?.textContent?.trim()
    if(pageTitle)for(const heading of [...document.querySelectorAll('.management-console .page-content h1,.management-console .page-content h2,.management-console .content h1,.management-console .content h2')].filter(visible)){
      if(!heading.closest('dialog') && heading.textContent?.trim()===pageTitle)issues.push(`page title repeated below navigation: ${pageTitle}`)
    }
    for (const list of [...document.querySelectorAll('.management-list,.user-record-list')].filter(visible)) {
      const name = list.getAttribute('data-testid') ?? list.className
      const own = (selector: string) => [...list.querySelectorAll(selector)].filter(el => el.closest('.management-list,.user-record-list') === list && el.closest('dialog') === list.closest('dialog') && visible(el))
      if ([...list.querySelectorAll('.management-list,.user-record-list')].some(child=>child.closest('dialog')===list.closest('dialog'))) issues.push(`${name}: nested list surface`)
      const style = getComputedStyle(list)
      if (parseFloat(style.paddingLeft) || parseFloat(style.paddingRight)) issues.push(`${name}: extra list padding`)
      for (const pager of own('.table-pagination')) {
        if (pager.closest('.table-wrap,.reports-table-wrap')) issues.push(`${name}: pagination inside table scroll area`)
        if (!pager.querySelector('.table-pagination-summary') || !pager.querySelector('.table-pagination-actions')) issues.push(`${name}: incomplete pagination`)
        const arrows = pager.querySelectorAll('.table-pagination-arrow')
        if (arrows.length !== 2 || [...arrows].some(button => !['‹', '›'].includes(button.textContent!.trim()))) issues.push(`${name}: nonstandard pagination arrows`)
        const table = own('.table-wrap,.reports-table-wrap').at(-1)
        if (table && (pager.compareDocumentPosition(table) & Node.DOCUMENT_POSITION_FOLLOWING)) issues.push(`${name}: pager before records`)
      }
      for (const filter of own('.ui-filter-bar')) {
        if ((list.matches('.management-list') || filter.querySelector('button[type=submit]')) && filter.tagName !== 'FORM') issues.push(`${name}: filter is not a form`)
        if (list.matches('.management-list') && !filter.closest('.management-list-filters')) issues.push(`${name}: filter outside toolbar slot`)
      }
      for(const button of own('.ui-filter-bar button'))if(/^(Reset|重置)$/.test(button.textContent?.trim()??''))issues.push(`${name}: filter clear label differs from the shared contract`)
      const toolbar = list.querySelector(':scope > .management-list-toolbar')
      if (toolbar) {
        const query = toolbar.querySelector('.management-list-filters button[type=submit]')
        const actions = toolbar.querySelector(':scope > .console-page-actions')
        if (query && actions && visible(query) && visible(actions) && toolbar.clientWidth > 0) {
          const q = query.getBoundingClientRect(), a = actions.getBoundingClientRect()
          const filter = query.closest('.ui-filter-bar')!, children = [...filter.children].filter(visible), filterStyle = getComputedStyle(filter), toolbarStyle = getComputedStyle(toolbar)
          const required = children.reduce((sum,child)=>sum+child.getBoundingClientRect().width,0) + Math.max(0,children.length-1)*parseFloat(filterStyle.columnGap) + a.width + parseFloat(toolbarStyle.columnGap) + parseFloat(toolbarStyle.paddingLeft) + parseFloat(toolbarStyle.paddingRight)
          if (toolbar.clientWidth >= required && Math.abs(q.bottom - a.bottom) > 2) issues.push(`${name}: global action is detached from query row`)
        }
      }
      for(const wrap of own('.table-wrap,.reports-table-wrap')){
        const css=getComputedStyle(wrap)
        if(parseFloat(css.borderLeftWidth)>0||parseFloat(css.borderRightWidth)>0)issues.push(`${name}: second border around table`)
      }
      for(const empty of own('td.empty-state')){
        const range=document.createRange();range.selectNodeContents(empty)
        const text=range.getBoundingClientRect(),box=list.getBoundingClientRect()
        if(text.left<box.left-1||text.right>box.right+1)issues.push(`${name}: empty message is outside the visible list`)
      }
      for (const table of own('table')) {
        if (!table.querySelector('thead')) issues.push(`${name}: missing table headers`)
        for (const empty of table.querySelectorAll('td.empty-state')) {
          if (Number(empty.getAttribute('colspan')) !== table.querySelectorAll('thead tr:first-child th').length) issues.push(`${name}: empty colspan does not match columns`)
        }
      }
    }
    return issues
  })
  expect(issues).toEqual([])
}

export async function reviewRealPage(page: Page, info: TestInfo, label: string) {
  const preset = page.viewportSize()!
  for (const width of info.project.name.endsWith('reflow') ? [320, 620, 900, 1080, 1440] : [preset.width]) {
    if (info.project.name.endsWith('reflow')) await page.setViewportSize({ width, height: preset.height })
    await expectListLayout(page)
    await page.screenshot({ path: info.outputPath(`${label}-${width}.png`), fullPage: true })
  }
  if (info.project.name.endsWith('reflow')) await page.setViewportSize(preset)
}
