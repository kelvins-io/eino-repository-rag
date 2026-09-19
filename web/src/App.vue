<template>
  <a-config-provider :locale="arcoLocale">
    <router-view />
  </a-config-provider>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import zhCN from '@arco-design/web-vue/es/locale/lang/zh-cn'
import enUS from '@arco-design/web-vue/es/locale/lang/en-us'

const { locale } = useI18n()
const arcoLocale = computed(() => (locale.value === 'en' ? enUS : zhCN))
</script>
