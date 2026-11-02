import { expect, type Page } from '@playwright/test'

export async function expectFilterSpacing(page: Page) {
  await expect.poll(() => page.evaluate(() => {
    const issues: string[] = []
    const visible = (element: Element) => { const rect = element.getBoundingClientRect(); return rect.width > 0 && rect.height > 0 && getComputedStyle(element).visibility !== 'hidden' }
    for (const bar of document.querySelectorAll<HTMLElement>('.ui-filter-bar')) {
      if (!visible(bar)) continue
      let frame: HTMLElement | null = bar
      while (frame && frame !== document.body) {
        const style = getComputedStyle(frame)
        if (parseFloat(style.borderLeftWidth) > 0 && parseFloat(style.borderTopWidth) > 0) break
        frame = frame.parentElement
      }
      const bounds = bar.getBoundingClientRect()
      for (const control of bar.querySelectorAll<HTMLElement>('input:not([type=hidden],[type=checkbox],[type=radio]), select, button')) {
        if (!visible(control) || control.closest('[popover]') || control.closest('dialog') !== bar.closest('dialog')) continue
        const rect = control.getBoundingClientRect()
        const label = `${bar.className}: ${control.getAttribute('aria-label') ?? control.textContent?.trim().slice(0, 30) ?? control.tagName}`
        if (rect.left < bounds.left - 1 || rect.right > bounds.right + 1) issues.push(`${label}: outside filter bar`)
        if (frame && frame !== document.body) {
          const box = frame.getBoundingClientRect()
          const content = control.closest<HTMLElement>('.detail-modal-content')
          const scrolls = frame.matches('dialog') && content && content.scrollHeight > content.clientHeight + 1
          const gaps = [rect.left - box.left, box.right - rect.right]
          // A tall modal clips its scrolling body; that clipping is not field padding.
          if (!scrolls) gaps.push(rect.top - box.top, box.bottom - rect.bottom)
          if (Math.min(...gaps) < 7.5) issues.push(`${label}: touches ${frame.className}; gaps=${gaps.map(value=>value.toFixed(1)).join(',')}; control=${rect.x.toFixed(1)},${rect.y.toFixed(1)},${rect.width.toFixed(1)},${rect.height.toFixed(1)}; frame=${box.x.toFixed(1)},${box.y.toFixed(1)},${box.width.toFixed(1)},${box.height.toFixed(1)}`)
        }
      }
      const groups = [...bar.children].filter(visible).map(element => element.getBoundingClientRect())
      for (let a = 0; a < groups.length; a++) for (let b = a + 1; b < groups.length; b++) {
        if (Math.min(groups[a].right, groups[b].right) - Math.max(groups[a].left, groups[b].left) > 1 && Math.min(groups[a].bottom, groups[b].bottom) - Math.max(groups[a].top, groups[b].top) > 1) issues.push(`${bar.className}: overlapping filter fields`)
      }
    }
    return issues
  }), { message: 'Filter fields must fit their bar and stay clear of panel borders' }).toEqual([])
}
