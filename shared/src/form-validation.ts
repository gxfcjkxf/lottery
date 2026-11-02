import type { ObjectDirective } from 'vue'
import type { LocaleCode } from './brand'
import { createCatalogI18n, type TranslationCatalog, translateMessage } from './localization'

export const formValidationCatalog = {
  en: {
    identityRequired: 'Enter at least an account or a phone number.',
    countryFormat: 'Enter two-letter country codes separated by commas, for example SG, MY.',
    accountFormat: 'Account must be 3–32 characters, start with a lowercase letter, and contain only lowercase letters, digits or underscores.',
    codeFormat: 'Code must be {min}–{max} characters, start with a lowercase letter, and contain only lowercase letters, digits or underscores.',
    positiveInteger: 'Enter a whole number greater than 0, without decimal places or separators.',
    nonnegativeInteger: 'Enter 0 or a positive whole number, without decimal places or separators.',
    numberList: 'Enter whole numbers separated by commas, for example 1, 2, 3.',
    oddsFormat: 'Enter 0 or a positive decimal with up to 10 integer digits and 6 decimal places, for example 2.5.',
    periodFormat: 'Period number must contain 1–40 digits.',
    credentialFormat: 'Use DRAW_FEED_ followed by 1–109 uppercase letters, digits or underscores.',
    percentageFormat: 'Enter a percentage from 0 to 100, with up to 2 decimal places; for example, 20 means 20%.',
    emailFormat: 'Enter a complete email address, for example name@example.com.',
    urlFormat: 'Enter a complete address including its protocol, for example https://example.com.',
    dateTimeFormat: 'Select a valid date and time using the date and time controls.',
    passwordHelp: 'Password must be 6–30 characters.',
    passwordMismatch: 'The two new passwords do not match. Enter the same password in both fields.',
    characterLength: '{field} must be {min}–{max} characters.',
    field: 'This field',
    required: '{field} is required.',
    format: 'Enter {field} in the required format.',
    number: '{field} must be a valid number.',
    short: '{field} must contain at least {minLength} characters.',
    long: '{field} must contain no more than {maxLength} characters.',
    minimum: '{field} must be at least {min}.',
    maximum: '{field} must not exceed {max}.',
    step: '{field} must use increments of {step}.',
    invalid: 'Check {field} and try again.',
  },
  'zh-CN': {
    identityRequired: '账号或手机号至少填写一项。',
    countryFormat: '请输入两位英文字母的国家代码，以英文逗号分隔，例如 SG, MY。',
    accountFormat: '账号须为 3–32 位，以小写英文字母开头，仅可包含小写英文字母、数字或下划线。',
    codeFormat: '代码须为 {min}–{max} 位，以小写英文字母开头，仅可包含小写英文字母、数字或下划线。',
    positiveInteger: '请输入大于 0 的整数，不含小数或分隔符。',
    nonnegativeInteger: '请输入 0 或正整数，不含小数或分隔符。',
    numberList: '请输入以英文逗号分隔的整数，例如 1, 2, 3。',
    oddsFormat: '请输入 0 或正小数，整数部分最多 10 位、小数部分最多 6 位，例如 2.5。',
    periodFormat: '期号须为 1–40 位数字。',
    credentialFormat: '凭据名称须以 DRAW_FEED_ 开头，后接 1–109 位大写英文字母、数字或下划线。',
    percentageFormat: '请输入 0–100 的百分比，最多保留 2 位小数；例如 20 表示 20%。',
    emailFormat: '请输入完整的电子邮箱地址，例如 name@example.com。',
    urlFormat: '请输入包含协议的完整地址，例如 https://example.com。',
    dateTimeFormat: '请通过日期和时间控件选择有效的日期与时间。',
    passwordHelp: '密码须为 6–30 个字符。',
    passwordMismatch: '两次输入的新密码不一致，请重新确认。',
    characterLength: '{field}长度须为 {min}–{max} 个字符。',
    field: '此项内容',
    required: '{field}不能为空。',
    format: '{field}格式不正确，请按要求填写。',
    number: '{field}必须为有效数字。',
    short: '{field}长度不得少于 {minLength} 个字符。',
    long: '{field}长度不得超过 {maxLength} 个字符。',
    minimum: '{field}不得小于 {min}。',
    maximum: '{field}不得大于 {max}。',
    step: '{field}须按 {step} 的间隔填写。',
    invalid: '请检查{field}后重试。',
  },
} satisfies TranslationCatalog

const i18n = createCatalogI18n(formValidationCatalog)
let localeInitialized = false
type Control = HTMLInputElement | HTMLTextAreaElement
type State = {
  form: HTMLFormElement
  control?: Control
  message?: HTMLParagraphElement
  ariaInvalid: string | null
  ariaDescribedby: string | null
  submit: (event: Event) => void
  edit: (event: Event) => void
  blur: (event: FocusEvent) => void
  reset: () => void
}
const states = new Map<HTMLFormElement, State>()
let sequence = 0

export function formConstraintMessage(validity: ValidityState, locale: LocaleCode, constraints: FormConstraints): string {
  if (validity.patternMismatch && constraints.format) return constraints.format
  if (validity.typeMismatch && constraints.type === 'email') return formValidationCatalog[locale].emailFormat
  if (validity.typeMismatch && constraints.type === 'url') return formValidationCatalog[locale].urlFormat
  const key = validity.badInput ? 'number'
    : validity.valueMissing ? 'required'
    : validity.typeMismatch || validity.patternMismatch ? 'format'
    : validity.tooShort ? 'short' : validity.tooLong ? 'long'
    : validity.rangeUnderflow ? 'minimum' : validity.rangeOverflow ? 'maximum'
    : validity.stepMismatch ? 'step' : 'invalid'
  return translateMessage(locale, { zh: formValidationCatalog['zh-CN'][key], en: formValidationCatalog.en[key], parameters: constraints })
}

type FormConstraints = { field: string; minLength: string; maxLength: string; min: string; max: string; format?: string; type?: string; step?: string }

export function formValidationText(key: keyof typeof formValidationCatalog.en, parameters?: import('./localization').MessageParameters) {
  return i18n.t(key, parameters)
}

function constraintsFor(control: Control): FormConstraints {
  const label = control.labels?.[0]?.cloneNode(true) as HTMLElement | undefined
  label?.querySelectorAll('input, textarea, small, .select-control, .inline-validation-error').forEach(node => node.remove())
  return {
    field: errorTarget(control).getAttribute('aria-label') || label?.textContent?.trim() || i18n.t('field'),
    minLength: control.dataset.minCharacters ?? '',
    maxLength: control.dataset.maxCharacters ?? '',
    min: control.getAttribute('min') ?? '',
    max: control.getAttribute('max') ?? '',
    format: control.dataset.validationFormat,
    type: control.getAttribute('type') ?? 'text',
    step: control.getAttribute('step') ?? (control.getAttribute('type') === 'time' ? '60' : '1'),
  }
}

function controlMessage(control: Control): string | null {
  const constraints = constraintsFor(control)
  if (!control.validity.valid) return formConstraintMessage(control.validity, i18n.locale.value, constraints)
  if (control.value) {
    const length = Array.from(control.value).length
    const min = control.dataset.minCharacters, max = control.dataset.maxCharacters
    if (min && length < Number(min) || max && length > Number(max)) {
      if (min && max) return formValidationText('characterLength', { field: constraints.field, min, max })
      return formValidationText(min ? 'short' : 'long', constraints)
    }
  }
  return null
}

// Native HTML length limits count UTF-16 units and truncate emoji early.
// Keep text length validation in this directive so it agrees with Go and SQL.
function useCharacterLimits(form: HTMLFormElement) {
  for (const control of form.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>('input[minlength], input[maxlength], textarea[minlength], textarea[maxlength]')) {
    for (const [attribute, key] of [['minlength', 'minCharacters'], ['maxlength', 'maxCharacters']] as const) {
      const value = control.getAttribute(attribute)
      if (value !== null) {
        if (!control.dataset[key]) control.dataset[key] = value
        control.removeAttribute(attribute)
      }
    }
  }
}

function errorTarget(control: Control): HTMLElement {
  if (control.hasAttribute('data-select-validation')) return control.parentElement!.querySelector<HTMLButtonElement>('.select-control-trigger')!
  return control
}

function clear(state: State) {
  const control = state.control
  if (control) {
    const target = errorTarget(control)
    for (const [name, value] of [['aria-invalid', state.ariaInvalid], ['aria-describedby', state.ariaDescribedby]]) {
      if (value === null) target.removeAttribute(name!)
      else target.setAttribute(name!, value!)
    }
    target.classList.remove('inline-validation-invalid')
  }
  state.message?.remove()
  state.control = undefined
  state.message = undefined
}

function refresh(state: State) {
  if (!state.control) return
  if (!state.form.contains(state.control) || !state.control.willValidate || !controlMessage(state.control)) {
    clear(state)
    return
  }
  if (!localeInitialized) throw new Error('Form validation locale has not been initialized')
  state.message!.textContent = controlMessage(state.control)
}

export function setInlineValidationLocale(locale: LocaleCode) {
  i18n.setLocale(locale)
  localeInitialized = true
  for (const state of states.values()) refresh(state)
}

function showError(state: State, control: Control, focus: boolean, requestedSubmitter?: HTMLElement | null) {
  clear(state)
  const form = state.form
  state.control = control
  const target = errorTarget(control)
  state.ariaInvalid = target.getAttribute('aria-invalid')
  state.ariaDescribedby = target.getAttribute('aria-describedby')
  const message = form.ownerDocument.createElement('p')
  state.message = message
  message.id = `inline-validation-${++sequence}`
  message.className = 'inline-validation-error'
  message.setAttribute('role', 'alert')
  message.tabIndex = -1
  target.setAttribute('aria-invalid', 'true')
  target.setAttribute('aria-describedby', [state.ariaDescribedby, message.id].filter(Boolean).join(' '))
  target.classList.add('inline-validation-invalid')
  const submitter = requestedSubmitter
    ?? Array.from(form.elements).find(element =>
      (element.tagName === 'BUTTON' || element.tagName === 'INPUT')
      && (element as HTMLButtonElement).type === 'submit' && !(element as HTMLButtonElement).disabled)
  const group = submitter?.closest('.form-actions, .action-row, .actions, .withdrawal-actions, .auth-actions')
  if (group && form.contains(group)) group.before(message)
  else if (submitter && form.contains(submitter)) submitter.before(message)
  else form.append(message)
  refresh(state)
  if (focus) message.focus()
}

/** Keep browser constraints, but display errors in the page instead of native bubbles. */
export const vInlineValidation: ObjectDirective<HTMLFormElement> = {
  getSSRProps: () => ({ novalidate: true }),
  mounted(form) {
    form.noValidate = true
    useCharacterLimits(form)
    const state: State = {
      form, ariaInvalid: null, ariaDescribedby: null,
      submit(event) {
        useCharacterLimits(form)
        clear(state)
        const control = Array.from(form.elements).find((element): element is Control =>
          ['INPUT', 'TEXTAREA'].includes(element.tagName)
          && (element as Control).willValidate && Boolean(controlMessage(element as Control)))
        if (!control) return
        event.preventDefault()
        event.stopImmediatePropagation()
        showError(state, control, true, (event as SubmitEvent).submitter)
      },
      blur(event) {
        useCharacterLimits(form)
        const control = event.target as Control
        if (!['INPUT', 'TEXTAREA'].includes(control.tagName) || !control.willValidate || !control.value || !controlMessage(control)) return
        if (state.control === control) { refresh(state); return }
        showError(state, control, false)
      },
      edit(event) { if (event.target === state.control) refresh(state) },
      reset() { clear(state) },
    }
    states.set(form, state)
    form.addEventListener('submit', state.submit, true)
    form.addEventListener('input', state.edit)
    form.addEventListener('change', state.edit)
    form.addEventListener('reset', state.reset)
    form.addEventListener('focusout', state.blur)
  },
  updated(form) { useCharacterLimits(form); refresh(states.get(form)!) },
  beforeUnmount(form) {
    const state = states.get(form)!
    clear(state)
    form.removeEventListener('submit', state.submit, true)
    form.removeEventListener('input', state.edit)
    form.removeEventListener('change', state.edit)
    form.removeEventListener('reset', state.reset)
    form.removeEventListener('focusout', state.blur)
    states.delete(form)
  },
}
