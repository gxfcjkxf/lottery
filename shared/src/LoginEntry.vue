<script setup lang="ts">
import LanguageSwitcher from './LanguageSwitcher.vue'
import SiteIcon from './SiteIcon'
withDefaults(defineProps<{
  brandName: string
  logoText?: string
  logoUrl?: string | null
  subtitle?: string
  title: string
  titleId: string
  locale: string
  languageTestId?: string
  languageLabel: string
  languages: readonly {code: string; label: string}[]
  ready?: boolean
  cardClass?: string
}>(), { ready: true })
defineEmits<{ 'update:locale': [value: string]; logoError: [] }>()
</script>
<template>
  <div class="login-entry">
    <header class="login-entry__topbar">
      <a class="login-entry__brand" href="#" :aria-label="brandName">
        <span class="login-entry__mark" aria-hidden="true"><img v-if="logoUrl" :src="logoUrl" alt="" crossorigin="anonymous" referrerpolicy="no-referrer" @error="$emit('logoError')" /><template v-else-if="logoText">{{logoText}}</template><SiteIcon v-else name="ticket" /></span>
        <span><b class="auth-brand-name">{{brandName}}</b><small v-if="subtitle">{{subtitle}}</small></span>
      </a>
      <LanguageSwitcher :data-testid="languageTestId" v-if="languages.length > 1" :model-value="locale" :label="languageLabel" :options="languages" @update:model-value="$emit('update:locale', $event)" />
    </header>
    <main class="login-entry__main">
      <slot name="status" />
      <article v-if="ready !== false" class="login-entry__card" :class="cardClass" :aria-labelledby="titleId">
        <h1 :id="titleId">{{title}}</h1>
        <slot />
      </article>
    </main>
  </div>
</template>
