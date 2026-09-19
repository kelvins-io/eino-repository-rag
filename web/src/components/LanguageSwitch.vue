<template>
  <el-dropdown trigger="click" @command="setAppLocale">
    <el-button size="small">
      {{ currentLabel }}
      <el-icon class="el-icon--right"><ArrowDown /></el-icon>
    </el-button>
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item
          v-for="item in LOCALES"
          :key="item.value"
          :command="item.value"
          :disabled="locale === item.value"
        >
          {{ item.label }}
        </el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</template>

<script setup>
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowDown } from '@element-plus/icons-vue'
import { LOCALES, setAppLocale } from '@/i18n'

const { locale } = useI18n()
const currentLabel = computed(
  () => LOCALES.find((item) => item.value === locale.value)?.label || '中文',
)
</script>
