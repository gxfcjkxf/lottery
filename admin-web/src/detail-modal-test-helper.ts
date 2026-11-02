import { defineComponent, h } from 'vue'

// Host-renderer business tests retain dialog identity and content. Native focus,
// dismissal, top-layer progress and responsive behavior run in Playwright.
export const DetailModalTestComponent = defineComponent({
  inheritAttrs: false,
  props: { title: String, titleId: String, closeLabel: String, closeDisabled: Boolean, open: { type: Boolean, default: true } },
  emits: ['close'],
  setup(props, { attrs, slots, emit }) {
    return () => props.open ? h('dialog', { ...attrs, role: 'dialog', 'aria-label': props.title, open: true }, [
      h('header', [h('h2', { id: props.titleId }, props.title), h('button', { type: 'button', disabled: props.closeDisabled, 'aria-label': props.closeLabel, onClick: () => emit('close') }, props.closeLabel)]),
      slots.default?.(), slots.footer?.(),
    ]) : null
  },
})
