import type { Directive } from 'vue';
import { h, render } from 'vue';
import SiteIcon from './SiteIcon';

const icons = new WeakMap<HTMLButtonElement, HTMLSpanElement>();

function attributes(label: string) {
  if (typeof label !== 'string' || !label.trim()) throw new TypeError('Refresh buttons require a translated accessible label.');
  return { 'data-refresh-button': 'true', 'aria-label': label, title: label };
}

function sync(element: HTMLButtonElement, label: string) {
  const props = attributes(label);
  // Attribute hooks apply to browser elements; SSR uses getSSRProps.
  if (typeof HTMLElement === 'undefined' || !(element instanceof HTMLElement)) return;
  if (element.tagName !== 'BUTTON') throw new TypeError('v-refresh-button must be used on a button.');
  for (const [key, value] of Object.entries(props)) element.setAttribute(key, value);
  let icon = icons.get(element);
  if (!icon) {
    icon = element.ownerDocument.createElement('span');
    icon.setAttribute('data-refresh-icon', '');
    icons.set(element, icon);
    render(h(SiteIcon, { name: 'refresh' }), icon);
  }
  // Vue may replace button text during a locale or loading-state update.
  if (icon.parentElement !== element) element.append(icon);
}

/** Preserve native click/disabled semantics while sharing one icon and label. */
export const vRefreshButton: Directive<HTMLButtonElement, string> = {
  mounted: (element, binding) => sync(element, binding.value),
  updated: (element, binding) => sync(element, binding.value),
  beforeUnmount(element) {
    const icon = icons.get(element);
    if (icon) render(null, icon);
    icons.delete(element);
  },
  getSSRProps: binding => attributes(binding.value),
};
