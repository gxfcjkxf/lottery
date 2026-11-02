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
