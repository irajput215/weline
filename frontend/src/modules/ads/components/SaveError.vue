<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAdsStore } from '../stores/ads'

// The last failed save, in words: the server's message, or a translated fallback.
const { t } = useI18n()
const store = useAdsStore()

const text = computed(() => {
  switch (store.saveError) {
    case null:
      return ''
    case 'forbidden':
      return t('wl.ads.saveForbidden')
    case 'error':
      return t('wl.ads.saveFailed')
    default:
      return store.saveError
  }
})
</script>

<template>
  <p v-if="text" class="text-sm text-destructive" role="alert" data-testid="ads-save-error">{{ text }}</p>
</template>
