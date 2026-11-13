<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Copy } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'

// Copies text to the clipboard and says so.
const props = defineProps<{ text: string; label?: string }>()
const { t } = useI18n()

async function copy() {
  try {
    await navigator.clipboard.writeText(props.text)
    toast.success(t('wl.ads.copied'))
  } catch {
    toast.error(t('wl.ads.copyFailed'))
  }
}
</script>

<template>
  <Button variant="outline" size="sm" @click.stop="copy">
    <Copy class="h-4 w-4" :class="{ 'mr-2': label }" />
    <span v-if="label">{{ label }}</span>
    <span v-else class="sr-only">{{ t('wl.ads.copy') }}</span>
  </Button>
</template>
