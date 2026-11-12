<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAudiencesStore } from '../stores/audiences'

// The last failed save, in words: the server's message, or a translated fallback.
const props = defineProps<{ message?: string | null }>()
const { t } = useI18n()
const store = useAudiencesStore()

const text = computed(() => {
  if (props.message) return props.message
  switch (store.saveError) {
    case null:
      return ''
    case 'forbidden':
      return t('wl.audiences.saveForbidden')
    case 'error':
      return t('wl.audiences.saveFailed')
    default:
      return store.saveError
  }
})
</script>

<template>
  <p v-if="text" class="text-sm text-destructive" role="alert" data-testid="audiences-save-error">{{ text }}</p>
</template>
