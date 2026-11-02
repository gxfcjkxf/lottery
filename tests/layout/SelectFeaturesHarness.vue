<script setup lang="ts">
import { ref, watch } from 'vue'
import SelectControl from '../../shared/src/SelectControl.vue'
import TimeZoneSelect from '../../shared/src/TimeZoneSelect.vue'
import PlatformTimeInput from '../../shared/src/PlatformTimeInput.vue'
import FilterAssociationSelect from '../../shared/src/FilterAssociationSelect.vue'
import { setInlineValidationLocale, vInlineValidation } from '../../shared/src/form-validation'
const props = defineProps<{ locale: 'en' | 'zh-CN' }>()
const copy = {en:{required:'Required choice',choose:'Choose',first:'First',second:'Second',platform:'Platform',brand:'Brand',number:'Number',boolean:'Boolean',yes:'Yes',no:'No',save:'Save',disable:'Disable',time:'Time',member:'Member',outside:'Outside'},'zh-CN':{required:'必选项',choose:'请选择',first:'第一项',second:'第二项',platform:'平台',brand:'品牌',number:'数字',boolean:'布尔值',yes:'是',no:'否',save:'保存',disable:'禁用',time:'时间',member:'会员',outside:'外部按钮'}}[props.locale]
watch(() => props.locale, setInlineValidationLocale, {immediate:true})
const required=ref(''), numeric=ref(0), boolean=ref(false), timezone=ref('Asia/Singapore'), time=ref('13:05:00'), member=ref(''), disabled=ref(false), saves=ref(0), changes=ref(0)
const options=[{value:'',label:copy.choose,disabled:true},{value:'first',label:copy.first,group:copy.platform},{value:'disabled',label:copy.no,group:copy.platform,disabled:true},{value:'second',label:copy.second,group:copy.brand}]
</script>
<template>
 <main class="management-console">
  <form novalidate v-inline-validation @submit.prevent="saves++">
   <fieldset :disabled="disabled">
    <label>{{copy.required}}<SelectControl v-model="required" :label="copy.required" :options="options" required name="choice" @change="changes++" /></label>
    <label>{{copy.number}}<SelectControl v-model="numeric" :label="copy.number" :options="[{value:0,label:'0'},{value:1,label:'1'}]" /></label>
    <label>{{copy.boolean}}<SelectControl v-model="boolean" :label="copy.boolean" :options="[{value:false,label:copy.no},{value:true,label:copy.yes}]" /></label>
    <label>{{copy.time}}<TimeZoneSelect v-model="timezone" :locale="locale" required /></label>
    <PlatformTimeInput v-model="time" :locale="locale" :label="copy.time" :display-settings="{timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'hh:mm:ss A'}" />
    <label>{{copy.member}}<FilterAssociationSelect v-model="member" brand-id="select-test-brand" surface="admin" enabled resource="report_withdrawal" kind="member" :label="copy.member" :locale="locale" /></label>
   </fieldset>
   <button class="button button-primary" type="submit">{{copy.save}}</button>
  </form>
  <button class="button button-secondary" @click="disabled=!disabled">{{copy.disable}}</button>
  <button class="button button-secondary" data-testid="outside">{{copy.outside}}</button>
  <output data-testid="result">{{JSON.stringify({required,numeric,boolean,timezone,time,member,saves,changes})}}</output>
 </main>
</template>
<style scoped>
main{padding:12px;box-sizing:border-box;max-width:100%;display:grid;gap:12px}form,fieldset{display:flex;align-items:end;flex-wrap:wrap;gap:12px;min-width:0}fieldset{margin:0;padding:0;border:0;flex-basis:100%}label{display:grid;gap:6px;min-width:0;max-width:100%}output{overflow-wrap:anywhere}
</style>
