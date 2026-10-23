import type { ObjectDirective } from 'vue'
import type { LocaleCode } from './brand'
import { createCatalogI18n, type TranslationCatalog } from './localization'

export const formValidationCatalog = {
  en: {
    required: 'Complete the required field.',
    format: 'Enter a value in the required format.',
    number: 'Enter a valid number.',
    short: 'The value is too short.',
    long: 'The value is too long.',
    minimum: 'The value is below the allowed minimum.',
    maximum: 'The value exceeds the allowed maximum.',
    step: 'Enter a value using the allowed increment.',
    invalid: 'Check the highlighted field.',
  },
  'zh-CN': {
    required: '请填写或选择必填项。',
    format: '请按要求的格式填写。',
    number: '请输入有效数字。',
    short: '填写内容过短。',
    long: '填写内容过长。',
    minimum: '填写的数值低于允许的最小值。',
    maximum: '填写的数值超过允许的最大值。',
    step: '请按允许的步长填写数值。',
    invalid: '请检查标记的字段。',
  },
} satisfies TranslationCatalog

const i18n = createCatalogI18n(formValidationCatalog)
let localeInitialized = false
type Control = HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement
type State = {
  form: HTMLFormElement
  control?: Control
  message?: HTMLParagraphElement
  ariaInvalid: string | null
  ariaDescribedby: string | null
  submit: (event: Event) => void
  edit: (event: Event) => void
  reset: () => void
}
const states = new Map<HTMLFormElement, State>()
let sequence = 0

export function formConstraintMessage(validity: ValidityState, locale: LocaleCode): string {
  const key = validity.badInput ? 'number'
    : validity.valueMissing ? 'required'
    : validity.typeMismatch || validity.patternMismatch ? 'format'
    : validity.tooShort ? 'short' : validity.tooLong ? 'long'
    : validity.rangeUnderflow ? 'minimum' : validity.rangeOverflow ? 'maximum'
    : validity.stepMismatch ? 'step' : 'invalid'
  return formValidationCatalog[locale][key]
}

function clear(state: State) {
  const control = state.control
  if (control) {
    for (const [name, value] of [['aria-invalid', state.ariaInvalid], ['aria-describedby', state.ariaDescribedby]]) {
      if (value === null) control.removeAttribute(name!)
      else control.setAttribute(name!, value!)
    }
    control.classList.remove('inline-validation-invalid')
  }
  state.message?.remove()
  state.control = undefined
  state.message = undefined
}

function refresh(state: State) {
  if (!state.control) return
  if (!state.form.contains(state.control) || !state.control.willValidate || state.control.validity.valid) {
    clear(state)
    return
  }
  if (!localeInitialized) throw new Error('Form validation locale has not been initialized')
  state.message!.textContent = formConstraintMessage(state.control.validity, i18n.locale.value)
}

export function setInlineValidationLocale(locale: LocaleCode) {
  i18n.setLocale(locale)
  localeInitialized = true
  for (const state of states.values()) refresh(state)
}

/** Keep browser constraints, but display errors in the page instead of native bubbles. */
export const vInlineValidation: ObjectDirective<HTMLFormElement> = {
  getSSRProps: () => ({ novalidate: true }),
  mounted(form) {
    form.noValidate = true
    const state: State = {
      form, ariaInvalid: null, ariaDescribedby: null,
      submit(event) {
        clear(state)
        const control = Array.from(form.elements).find((element): element is Control =>
          ['INPUT', 'SELECT', 'TEXTAREA'].includes(element.tagName)
          && (element as Control).willValidate && !(element as Control).validity.valid)
        if (!control) return
        event.preventDefault()
        event.stopImmediatePropagation()
        state.control = control
        state.ariaInvalid = control.getAttribute('aria-invalid')
        state.ariaDescribedby = control.getAttribute('aria-describedby')
        const message = form.ownerDocument.createElement('p')
        state.message = message
        message.id = `inline-validation-${++sequence}`
        message.className = 'inline-validation-error'
        message.setAttribute('role', 'alert')
        message.tabIndex = -1
        control.setAttribute('aria-invalid', 'true')
        control.setAttribute('aria-describedby', [state.ariaDescribedby, message.id].filter(Boolean).join(' '))
        control.classList.add('inline-validation-invalid')
        const submitter = (event as SubmitEvent).submitter
          ?? Array.from(form.elements).find(element =>
            (element.tagName === 'BUTTON' || element.tagName === 'INPUT')
            && (element as HTMLButtonElement).type === 'submit' && !(element as HTMLButtonElement).disabled)
        const group = submitter?.closest('.form-actions, .action-row, .actions, .withdrawal-actions, .auth-actions')
        if (group && form.contains(group)) group.before(message)
        else if (submitter && form.contains(submitter)) submitter.before(message)
        else form.append(message)
        refresh(state)
        message.focus()
      },
      edit(event) { if (event.target === state.control) refresh(state) },
      reset() { clear(state) },
    }
    states.set(form, state)
    form.addEventListener('submit', state.submit, true)
    form.addEventListener('input', state.edit)
    form.addEventListener('change', state.edit)
    form.addEventListener('reset', state.reset)
  },
  updated(form) { refresh(states.get(form)!) },
  beforeUnmount(form) {
    const state = states.get(form)!
    clear(state)
    form.removeEventListener('submit', state.submit, true)
    form.removeEventListener('input', state.edit)
    form.removeEventListener('change', state.edit)
    form.removeEventListener('reset', state.reset)
    states.delete(form)
  },
}
