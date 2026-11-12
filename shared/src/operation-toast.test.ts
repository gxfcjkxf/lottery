import { afterEach, expect, it } from 'vitest'
import { effectScope, ref } from 'vue'
import { createCatalogI18n } from './localization'
import { dismissOperationToast, operationToasts, showOperationToast, useOperationToast } from './operation-toast'
afterEach(() => dismissOperationToast())
it('keeps the captured bilingual receipt after its form closes and follows language changes', () => {
 const i18n = createCatalogI18n({ en: { saved: 'Saved' }, 'zh-CN': { saved: '已保存' } }, 'en')
 const receipt = ref<ReturnType<typeof i18n.message> | ''>(''), scope = effectScope()
 scope.run(() => useOperationToast(receipt, i18n.displayError, 'success'))
 receipt.value = i18n.message('saved'); scope.stop(); receipt.value = ''
 expect(operationToasts.value[0]?.text()).toBe('Saved')
 i18n.setLocale('zh-CN'); expect(operationToasts.value[0]?.text()).toBe('已保存')
})
it('updates one source, keeps separate error and outcome notices, and dismisses by identity', () => {
 const source = ref(''), scope = effectScope()
 scope.run(() => useOperationToast(source, value => value, 'error'))
 source.value = 'Read failed'; const first = operationToasts.value[0]!.id
 source.value = ''; source.value = 'Read failed again'
 expect(operationToasts.value).toHaveLength(1)
 showOperationToast(() => 'Write confirmed', 'success')
 expect(operationToasts.value.map(item => item.text())).toEqual(['Read failed again', 'Write confirmed'])
 dismissOperationToast(first); expect(operationToasts.value).toHaveLength(2)
 dismissOperationToast(operationToasts.value[0]!.id); expect(operationToasts.value).toHaveLength(1)
 scope.stop()
})

it('shows a restored pending result and clears its error when an explicit retry starts', () => {
 const result = ref('Original request requires review'), scope = effectScope()
 scope.run(() => useOperationToast(result, value => value, 'error'))
 expect(operationToasts.value[0]?.text()).toBe('Original request requires review')
 result.value = ''; expect(operationToasts.value).toHaveLength(0)
 scope.stop()
})

it('coalesces identical errors across sources without renewing the toast and allows later errors', () => {
 const child = ref(''), parent = ref(''), scope = effectScope()
 scope.run(() => {
  useOperationToast(child, value => value, 'error')
  useOperationToast(parent, value => value, 'error')
 })
 child.value = 'Session expired'
 const first = operationToasts.value[0]!.id
 parent.value = 'Session expired'
 showOperationToast(() => 'Session expired', 'error')
 expect(operationToasts.value).toHaveLength(1)
 expect(operationToasts.value[0]!.id).toBe(first)
 dismissOperationToast(first)
 parent.value = ''; parent.value = 'Session expired'
 expect(operationToasts.value).toHaveLength(1)
 expect(operationToasts.value[0]!.id).not.toBe(first)
 showOperationToast(() => 'Permission denied', 'error')
 showOperationToast(() => 'Session expired', 'success')
 expect(operationToasts.value.map(item => [item.text(), item.tone])).toEqual([
  ['Session expired', 'error'], ['Permission denied', 'error'], ['Session expired', 'success'],
 ])
 scope.stop()
})

it('compares errors in the current language and preserves their live translations', () => {
 const i18n = createCatalogI18n({ en: { expired: 'Session expired' }, 'zh-CN': { expired: '登录已失效' } }, 'en')
 showOperationToast(() => i18n.t('expired'), 'error', Symbol())
 i18n.setLocale('zh-CN')
 showOperationToast(() => i18n.t('expired'), 'error', Symbol())
 expect(operationToasts.value).toHaveLength(1)
 expect(operationToasts.value[0]!.text()).toBe('登录已失效')
})

it('removes a source’s previous error when it changes to an already displayed error', () => {
 const first = Symbol(), second = Symbol()
 showOperationToast(() => 'Read failed', 'error', first)
 showOperationToast(() => 'Session expired', 'error', second)
 const expiredId = operationToasts.value[1]!.id
 showOperationToast(() => 'Session expired', 'error', first)
 expect(operationToasts.value.map(item => item.id)).toEqual([expiredId])
})
