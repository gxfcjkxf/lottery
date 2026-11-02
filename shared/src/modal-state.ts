import { computed, shallowReactive } from 'vue'

export const modalDialogs = shallowReactive<HTMLDialogElement[]>([])
export const activeModalDialog = computed(() => modalDialogs.at(-1) ?? null)
