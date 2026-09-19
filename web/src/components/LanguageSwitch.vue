<template>
  <a-dropdown trigger="click" @select="setAppLocale">
    <a-button size="small">
      {{ currentLabel }}
      <icon-down />
    </a-button>
    <template #content>
      <a-doption
        v-for="item in LOCALES"
        :key="item.value"
        :value="item.value"
        :disabled="locale === item.value"
      >
        {{ item.label }}
      </a-doption>
    </template>
  </a-dropdown>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { LOCALES, setAppLocale } from '@/i18n'

const { locale } = useI18n()
const currentLabel = computed(
  () => LOCALES.find((item) => item.value === locale.value)?.label || '中文',
)
</script>
