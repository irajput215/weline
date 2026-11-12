<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useConsentStore } from '../stores/consent'

// The last failed save, in words: the server's message, or a translated fallback.
const props = defineProps<{ message?: string | null }>()
const { t } = useI18n()
const store = useConsentStore()

const text = computed(() => {
  if (props.message) return props.message
  switch (store.saveError) {
    case null:
      return ''
    case 'forbidden':
      return t('wl.consent.saveForbidden')
    case 'error':
      return t('wl.consent.saveFailed')
    default:
      return store.saveError
  }
})
</script>

<template>
  <p v-if="text" class="text-sm text-destructive" role="alert" data-testid="consent-save-error">{{ text }}</p>
</template>
