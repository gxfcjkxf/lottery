<script setup lang="ts">
import {provide,ref} from 'vue'
import Settings from '../../platform-web/src/SecuritySettingsPanel.vue'
import {createPlatformI18n,platformI18nKey} from '../../platform-web/src/i18n'
const props=defineProps<{locale:'en'|'zh-CN'}>()
const i18n=createPlatformI18n();i18n.setLocale(props.locale);provide(platformI18nKey,i18n)
const brand=ref('')
const id='aaaaaaaa-aaaa-4aaa-8aaa-000000000001'
const account={id,username:'security_test',super_admin:true,platform_permissions:['auth_config.view.platform','auth_config.write.platform','compliance_policy.view.platform','compliance_policy.write.platform']}
</script>
<template><main class="management-console console-stack" style="padding:12px"><button data-testid="select-brand" class="button" @click="brand=id">Brand fixture</button><button data-testid="clear-brand" class="button" @click="brand=''">Clear brand fixture</button><Settings :brand-id="brand" :account="account" status="active"/></main></template>
