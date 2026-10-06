/** Keep fixed navigation inside the visible viewport, including classic scrollbars. */
export function installVisibleViewport(window: Window, document: Document): () => void {
  const viewport = window.visualViewport
  const sync = () => {
    const visualWidth = viewport?.width
    const width = visualWidth && Number.isFinite(visualWidth) && visualWidth > 0 ? visualWidth : document.documentElement.clientWidth
    if (Number.isFinite(width) && width > 0) document.documentElement.style.setProperty('--admin-visible-width', `${width}px`)
  }
  sync()
  window.addEventListener('resize', sync)
  viewport?.addEventListener('resize', sync)
  return () => {
    window.removeEventListener('resize', sync)
    viewport?.removeEventListener('resize', sync)
  }
}
