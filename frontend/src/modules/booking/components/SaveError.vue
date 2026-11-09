<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useBookingStore } from '../stores/booking'

// The last failed save, in words: the server's message, or a translated fallback.
// `message` (e.g. a form's own validation error) takes precedence.
const props = defineProps<{ message?: string | null }>()
const { t } = useI18n()
const store = useBookingStore()

const text = computed(() => {
  if (props.message) return props.message
  switch (store.saveError) {
    case null:
      return ''
    case 'forbidden':
      return t('wl.booking.saveForbidden')
    case 'error':
      return t('wl.booking.saveFailed')
    default:
      return store.saveError
  }
})
</script>

<template>
  <p v-if="text" class="text-sm text-destructive" role="alert" data-testid="booking-save-error">{{ text }}</p>
</template>
