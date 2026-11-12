import { computed, onMounted, onUnmounted, ref } from 'vue'
export const themeIds = ['jade', 'vermilion', 'ocean', 'obsidian'] as const
export type ThemeId = typeof themeIds[number]
export type ThemeMode = 'light' | 'dark' | 'auto'
export const themeCatalog = {
  en: { theme: 'Theme', jade: 'Porcelain jade', vermilion: 'Warm vermilion', ocean: 'Nordic ocean', obsidian: 'Obsidian jade', light: 'Day mode', dark: 'Night mode', auto: 'Automatic mode' },
  'zh-CN': { theme: '主题', jade: '青瓷白玉', vermilion: '珊瑚朱砂', ocean: '海盐冰蓝', obsidian: '曜石翡翠', light: '日间模式', dark: '夜间模式', auto: '自动模式' },
}
const mode = ref<ThemeMode>('light')
const dark = ref(false)
export const resolvedTheme = computed(() => mode.value === 'auto' ? (dark.value ? 'dark' : 'light') : mode.value)
export function applyTheme(id: ThemeId) {
  if (!themeIds.includes(id)) throw new Error('Invalid platform theme')
  document.documentElement.dataset.theme = id
  const saved = localStorage.getItem('lottery.theme.mode')
  if (saved !== null) {
    if (!['light', 'dark', 'auto'].includes(saved)) throw new Error('Invalid theme mode')
    mode.value = saved as ThemeMode
  }
  dark.value = matchMedia('(prefers-color-scheme: dark)').matches
  applyMode()
}
function applyMode() {
  document.documentElement.dataset.userTheme = resolvedTheme.value
  document.documentElement.style.colorScheme = resolvedTheme.value
}
export function useTheme() {
  const query = matchMedia('(prefers-color-scheme: dark)')
  const changed = (event: MediaQueryListEvent) => { dark.value = event.matches; applyMode() }
  onMounted(() => { query.addEventListener('change', changed); applyMode() })
  onUnmounted(() => query.removeEventListener('change', changed))
  function cycleTheme() {
    const modes: ThemeMode[] = ['light', 'dark', 'auto']
    mode.value = modes[(modes.indexOf(mode.value) + 1) % modes.length]!
    localStorage.setItem('lottery.theme.mode', mode.value)
    applyMode()
  }
  return { mode, resolvedTheme, cycleTheme, themeIcon: computed(() => mode.value === 'light' ? 'sun' as const : mode.value === 'dark' ? 'moon' as const : 'themeAuto' as const) }
}
