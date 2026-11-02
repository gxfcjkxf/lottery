import { beforeEach } from 'vitest'
import { dismissOperationToast, operationToasts } from '@lottery/shared'

// Component renderers do not mount App's portaled Toast host. Assert its shared
// messages alongside component content; browser tests cover the rendered host.
beforeEach(() => dismissOperationToast())
export const operationToastText = () => operationToasts.value.map(notice => notice.text()).join('\n')

export const operationToastHasError = () => operationToasts.value.some(notice => notice.tone === 'error')
