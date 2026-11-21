<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { AssociationName, associationLabel, withGlobalLoading } from '@lottery/shared'
import ManagementList from '@lottery/shared/ManagementList.vue'
import AgentTreeLevel from './AgentTreeLevel.vue'
import { createAgentsApi, type AgentNode, type AgentTree } from './agents-api'
import { agentRatioToPercentage } from './agent-percentage'
import { useAdminI18n } from './i18n'

const props = defineProps<{ node: AgentNode; brandId: string; canCreate: boolean; disabled: boolean; initialPage?: AgentTree }>()
const emit = defineEmits<{ select: [node: AgentNode]; create: [node: AgentNode]; error: [cause: unknown] }>()
const { t, locale } = useAdminI18n()
const api = createAgentsApi()
const expanded = ref(Boolean(props.initialPage)), page = ref<AgentTree | null>(props.initialPage ?? null)
const busy = ref(false), failed = ref(false)
const childrenId = `agent-children-${props.node.id}`
const name = computed(() => associationLabel(props.node, 'agent', props.node.id, locale.value))
let disposed = false
onBeforeUnmount(() => { disposed = true })
async function load() {
  if (busy.value) return
  busy.value = true; failed.value = false
  try {
    const result = await api.treeLevel(props.brandId, props.node.id)
    if (disposed) return
    page.value = result; expanded.value = true
  } catch (cause) {
    if (!disposed) { failed.value = true; emit('error', cause) }
  } finally { if (!disposed) busy.value = false }
}
function toggle() {
  if (page.value) { expanded.value = !expanded.value; return }
  return withGlobalLoading(load)
}
</script>

<template>
  <li class="agent-tree-branch">
    <article class="agent-tree-row am-node" :aria-busy="busy || undefined" :data-agent-id="node.id">
      <button type="button" class="button button-secondary agent-tree-toggle" :aria-label="t(expanded ? 'admin.agents.collapseNode' : 'admin.agents.expandNode', { account: name })" :aria-expanded="expanded" :aria-controls="childrenId" :disabled="busy || disabled" @click="toggle"><span aria-hidden="true">{{ expanded ? '▾' : '▸' }}</span></button>
      <div class="agent-tree-main">
        <strong><AssociationName :record="node" kind="agent" :id="node.id" :locale="locale" /></strong>
        <div class="agent-tree-meta"><span>{{ t('admin.copy.ac22afab') }} {{ node.depth }}</span><span>{{ t('admin.copy.4f39e60b') }} {{ agentRatioToPercentage(node.config.ratio) }}%</span><span>{{ t(node.config.status === 'active' ? 'admin.copy.5eb01613' : 'admin.copy.5e3326db') }}</span><span>{{ t(node.effective_mode === 'loss' ? 'admin.copy.a13e79ef' : 'admin.copy.89fd862d') }}</span></div>
      </div>
      <div class="agent-tree-actions"><button type="button" class="button button-secondary" :disabled="disabled" @click="emit('select', node)">{{ t('admin.copy.01db40da') }}</button><button v-if="canCreate && node.config.can_create_children && !node.pending_change" type="button" class="button button-secondary" :disabled="disabled" @click="emit('create', node)">{{ t('admin.agents.createChild') }}</button></div>
    </article>
    <ManagementList v-show="expanded || (!page && failed)" :id="childrenId" class="agent-tree-children" embedded :busy="busy">
      <AgentTreeLevel v-if="page?.items.length" class="agent-tree-branches">
        <AgentTreeBranch v-for="child in page.items" :key="child.id" :node="child" :brand-id="brandId" :can-create="canCreate" :disabled="disabled || busy" @select="emit('select', $event)" @create="emit('create', $event)" @error="emit('error', $event)" />
      </AgentTreeLevel>
      <p v-else-if="page && !failed" class="agent-tree-empty empty-state">{{ t('admin.agents.noChildren') }}</p>
      <button v-if="failed" type="button" class="text-button agent-tree-retry" :disabled="busy || disabled" @click="withGlobalLoading(load)">{{ t('admin.agents.retryChildren') }}</button>
    </ManagementList>
  </li>
</template>

<style scoped>
.agent-tree-branch { min-width: 0; position: relative; }
.agent-tree-row.am-node { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 8px; margin: 0; padding: 8px; border: 1px solid var(--line); border-radius: var(--radius-sm); background: var(--surface); }
.agent-tree-toggle { width: 32px; padding: 0; box-shadow: none; border-color: transparent; background: transparent; }
.agent-tree-main { min-width: 0; overflow-wrap: anywhere; }
.agent-tree-main strong { color: var(--ink); }
.agent-tree-meta { display: flex; flex-wrap: wrap; gap: 4px 12px; margin-top: 4px; color: var(--ink-secondary); font-size: var(--body-font-size); }
.agent-tree-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
.agent-tree-branch > .agent-tree-children.management-list--embedded { min-width: 0; margin: 8px 0 0 16px; padding-left: 20px; border-left: 1px solid var(--line); }
.agent-tree-branches { margin-left: -20px; padding-left: 20px; }
.agent-tree-branches > .agent-tree-branch::before { content: ''; position: absolute; top: 24px; left: -20px; width: 20px; border-top: 1px solid var(--line); }
.agent-tree-empty { margin: 0; padding: 8px 0; font-size: var(--body-font-size); }
.agent-tree-retry { min-height: 32px; }
@media (max-width: 640px), (pointer: coarse) {
  .agent-tree-row.am-node { grid-template-columns: 44px minmax(0, 1fr); }
  .agent-tree-toggle { width: 44px; min-height: 44px; }
  .agent-tree-actions { grid-column: 2; }
  .agent-tree-branch > .agent-tree-children.management-list--embedded { margin-left: 8px; padding-left: 8px; }
  .agent-tree-branches { margin-left: -8px; padding-left: 8px; }
  .agent-tree-branches > .agent-tree-branch::before { left: -8px; width: 8px; }
  .agent-tree-retry { min-height: 44px; }
}
</style>
