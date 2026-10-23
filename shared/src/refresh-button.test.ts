import { afterEach, expect, it, vi } from 'vitest';
import { vRefreshButton } from './refresh-button';
import { render } from 'vue';

vi.mock('vue', async importOriginal => ({ ...await importOriginal<typeof import('vue')>(), render: vi.fn() }));

class Button {
  tagName = 'BUTTON';
  disabled = true;
  onclick = vi.fn();
  attributes: Record<string, string> = {};
  children: { parentElement: Button | null; setAttribute: ReturnType<typeof vi.fn> }[] = [];
  ownerDocument = { createElement: () => ({ parentElement: null, setAttribute: vi.fn() }) };
  append(child: typeof this.children[number]) { child.parentElement = this; this.children.push(child); }
  setAttribute(key: string, value: string) { this.attributes[key] = value; }
}
const directive = vRefreshButton as Record<string, (...args: any[]) => any>;
afterEach(() => vi.unstubAllGlobals());

it('updates translated labels without replacing handlers or disabled state', () => {
  vi.stubGlobal('HTMLElement', Button);
  const button = new Button(), click = button.onclick;
  directive.mounted(button, { value: 'Refresh' });
  expect(button.attributes).toEqual({ 'data-refresh-button': 'true', 'aria-label': 'Refresh', title: 'Refresh' });
  directive.updated(button, { value: '刷新' });
  expect(button.attributes['aria-label']).toBe('刷新');
  expect(button.attributes.title).toBe('刷新');
  expect(button.onclick).toBe(click);
  expect(button.disabled).toBe(true);
  expect(button.children).toHaveLength(1);
  expect(render).toHaveBeenCalled();
  expect(click).not.toHaveBeenCalled();
});
it('renders the same accessible attributes on the server and rejects invalid usage', () => {
  expect(directive.getSSRProps({ value: '刷新历史' })).toEqual({ 'data-refresh-button': 'true', 'aria-label': '刷新历史', title: '刷新历史' });
  expect(() => directive.getSSRProps({ value: '' })).toThrow('translated accessible label');
  vi.stubGlobal('HTMLElement', Button);
  const element = new Button(); element.tagName = 'DIV';
  expect(() => directive.mounted(element, { value: 'Refresh' })).toThrow('must be used on a button');
});
