import { readonly, shallowRef, watch, type WatchSource } from 'vue'

export type OperationToastTone = 'success' | 'error' | 'info'
type OperationToastPosition = 'corner' | 'center'
interface OperationToast { id: number; text: () => string; tone: OperationToastTone; position: OperationToastPosition; duration: number; source?: symbol }
const notices = shallowRef<OperationToast[]>([])
let sequence = 0
export const operationToasts = readonly(notices)

export function showOperationToast(text: () => string, tone: OperationToastTone = 'info', source?: symbol, position: OperationToastPosition = 'corner', duration = 3600) {
  notices.value = [...notices.value.filter(item => source === undefined || item.source !== source), { id: ++sequence, text, tone, source, position, duration }]
}
export function dismissOperationToast(id?: number) {
  notices.value = id === undefined ? [] : notices.value.filter(item => item.id !== id)
}

// Capture the message before its form closes, while formatting it in the current language.
export function useOperationToast<T>(source: WatchSource<T>, format: (value: NonNullable<T>) => string, tone: OperationToastTone = 'info', position: OperationToastPosition = 'corner', duration = 3600) {
  const identity = Symbol('operation-feedback')
  watch(source, value => {
    if (value) showOperationToast(() => format(value), tone, identity, position, duration)
    else notices.value = notices.value.filter(item => item.source !== identity)
  }, { flush: 'sync', immediate: true })
  return () => { notices.value = notices.value.filter(item => item.source !== identity) }
}
