import { defineComponent, h } from 'vue'

// Component contract for business tests using Vue's in-memory renderer.
// The real popup, keyboard, validation and layout run in playwright.select.config.ts.
export const SelectControlTestComponent = defineComponent({
  props: ['modelValue', 'options', 'label', 'disabled', 'required'],
  emits: ['update:modelValue', 'change'],
  setup(props, { emit }) {
    return () => h('select-control', {
      role: 'combobox', value: props.modelValue,
      'onUpdate:modelValue': (value: unknown) => emit('update:modelValue', value), 'aria-label': props.label, disabled: props.disabled,
      onChange(value: string | number | boolean) {
        emit('update:modelValue', value)
        emit('change', value)
      },
    }, props.options.map((option: {value:string|number|boolean;label:string;disabled?:boolean}) => h('option', {value:option.value,disabled:option.disabled}, option.label)))
  },
})
