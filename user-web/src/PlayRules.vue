<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import { configuredAvailableSelectionGroups, createCatalogI18n, type ConfiguredDisabledOptions, type ConfiguredSnapshot } from '@lottery/shared'
import SiteIcon from '@lottery/shared/SiteIcon'
import DetailModal from '@lottery/shared/DetailModal.vue'
import { describePlayRules, describePlayGroup } from './play-rules'
import { playRulesCatalog } from './play-rules-catalog'

const props = defineProps<{ locale: 'en' | 'zh'; snapshot: ConfiguredSnapshot; disabledOptions?: ConfiguredDisabledOptions; plate: string }>()
defineEmits<{ close: [] }>()
const accordionName = useId()
const openGroupId = ref<string | null>(null)
const localeCode = computed(() => props.locale === 'en' ? 'en' : 'zh-CN')
const i18n = computed(() => createCatalogI18n(playRulesCatalog, localeCode.value))
const t = (key: string) => i18n.value.t(key)
const groups = computed(() => configuredAvailableSelectionGroups(props.snapshot, props.disabledOptions))
const description = computed(() => {
  const group = groups.value.find(group => group.id === openGroupId.value)
  if (!group) return null
  const { snapshot, plate } = props
  try {
    if (group.plays.length > 1) {
      return { kind: 'group' as const, rules: describePlayGroup(localeCode.value, snapshot, group, plate, props.disabledOptions) }
    }
    return { kind: 'play' as const, rules: describePlayRules(localeCode.value, { snapshot, play: group.plays[0]!, plate, disabledOptions: props.disabledOptions }) }
  } catch {
    return null
  }
})

function toggleGroup(id: string, event: Event) {
  if ((event.currentTarget as HTMLDetailsElement).open) openGroupId.value = id
  else if (openGroupId.value === id) openGroupId.value = null
}
</script>
<template>
  <DetailModal class="player-rules-dialog" :title="t('title')" :close-label="t('close')" @close="$emit('close')">
    <div class="player-rules-list">
      <details v-for="group in groups" :key="group.id" :name="accordionName" class="player-rules-section" @toggle="toggleGroup(group.id, $event)">
        <summary>
          <span>{{ group.label[localeCode] }}</span><SiteIcon name="down" aria-hidden="true" />
        </summary>
        <div v-if="openGroupId === group.id" class="player-rule-body">
          <template v-if="description?.kind === 'group'">
            <section>
              <h3>{{ t('how') }}</h3>
              <p v-for="instruction in description.rules.instructions" :key="instruction">{{ instruction }}</p>
              <p v-for="note in description.rules.notes" :key="note">{{ note }}</p>
            </section>
            <section>
              <h3>{{ t('win') }} · {{ t('odds') }}</h3>
              <details v-for="entry in description.rules.entries" :key="entry.id" :name="`${accordionName}-${group.id}`" class="player-group-rule" :data-play-id="entry.id">
                <summary>
                  <span>{{ entry.label }}</span><span class="player-group-price">{{ entry.prices.map(price => `× ${price}`).join(' / ') }}</span><SiteIcon name="down" aria-hidden="true" />
                </summary>
                <div v-for="(prize, index) in entry.rules.prizes" :key="index" class="player-rule-prize">
                  <p>{{ prize.text }}</p><p v-if="prize.detail" class="player-rule-detail">{{ prize.detail }}</p>
                </div>
              </details>
            </section>
          </template>
          <template v-else-if="description?.kind === 'play'">
            <section>
              <h3>{{ t('how') }}</h3>
              <p v-for="instruction in description.rules.instructions" :key="instruction">{{ instruction }}</p>
            </section>
            <section>
              <h3>{{ t('win') }}</h3>
              <div v-for="(prize, index) in description.rules.prizes" :key="index" class="player-rule-prize">
                <p>{{ prize.text }}</p><p v-if="prize.detail" class="player-rule-detail">{{ prize.detail }}</p>
              </div>
            </section>
            <details v-if="description.rules.options.length" class="player-group-rule">
              <summary><span>{{ t('options') }} · {{ t('odds') }}</span><SiteIcon name="down" aria-hidden="true" /></summary>
              <dl>
                <div v-for="(option, index) in description.rules.options" :key="index"><dt>{{ option.text }}</dt><dd>{{ option.detail }}</dd></div>
              </dl>
            </details>
          </template>
          <p v-else class="message error" role="alert">{{ t('unavailable') }}</p>
        </div>
      </details>
    </div>
  </DetailModal>
</template>
<style scoped>
.player-rules-section + .player-rules-section,
.player-group-rule { border-top: 1px solid var(--line); }
.player-rules-section > summary,
.player-group-rule > summary {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: var(--control-height);
  padding: 4px 0;
  box-sizing: border-box;
  cursor: pointer;
  font-size: 14px;
  line-height: 22px;
}
.player-rules-section > summary { font-weight: 650; }
.player-rules-section > summary::-webkit-details-marker,
.player-group-rule > summary::-webkit-details-marker { display: none; }
.player-rules-dialog summary > span:first-child { flex: 1; min-width: 0; overflow-wrap: anywhere; }
.player-rules-dialog summary :deep(.site-icon) { flex-shrink: 0; color: var(--muted); }
.player-rules-dialog details[open] > summary :deep(.site-icon) { transform: rotate(180deg); }
.player-rules-section[open] > summary { color: var(--primary-dark); }
.player-rule-body { padding: 4px 0 8px; }
.player-rule-body h3 { font-size: 15px; margin: 0 0 8px; }
.player-rule-body section + section { margin-top: 12px; }
.player-rule-body p { font-size: 14px; line-height: 1.6; margin: 4px 0; overflow-wrap: anywhere; }
.player-rule-prize + .player-rule-prize { border-top: 1px solid var(--line); padding-top: 8px; margin-top: 8px; }
.player-rule-detail { white-space: pre-line; color: var(--ink-secondary); }
.player-group-price { white-space: nowrap; color: var(--danger-text); font-variant-numeric: tabular-nums; }
.player-rule-body dl { margin: 0; }
.player-rule-body dl > div {
  display: grid;
  grid-template-columns: minmax(64px, 1fr) minmax(0, 3fr);
  gap: 12px;
  padding: 8px 0;
  border-top: 1px solid var(--line);
  font-size: var(--body-font-size);
  line-height: 1.6;
}
.player-rule-body dd { margin: 0; overflow-wrap: anywhere; }
</style>
