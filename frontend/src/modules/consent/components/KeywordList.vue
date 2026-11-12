<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { normaliseKeyword } from '../stores/consent'

// Chips for a keyword list, normalised like the server. `taken` are words that
// may not be added here (they're in the other list).
const props = defineProps<{ label: string; testid: string; taken?: string[]; readonly?: boolean }>()
const words = defineModel<string[]>({ required: true })
const { t } = useI18n()
const draft = ref('')
const error = ref<string | null>(null)

function add() {
  const w = normaliseKeyword(draft.value)
  error.value = null
  if (!w) return
  if (props.taken?.includes(w)) {
    error.value = t('wl.consent.settings.inBoth', { word: w })
    return
  }
  if (!words.value.includes(w)) words.value = [...words.value, w]
  draft.value = ''
}
</script>

<template>
  <div class="space-y-2" :data-testid="testid">
    <p class="text-sm font-medium">{{ label }}</p>
    <div class="flex flex-wrap gap-2">
      <span v-for="w in words" :key="w" class="inline-flex items-center gap-1 rounded-full bg-muted px-2.5 py-1 text-xs font-medium">
        {{ w }}
        <button
          v-if="!readonly"
          type="button"
          class="rounded-full hover:text-destructive"
          :aria-label="t('wl.consent.settings.remove', { word: w })"
          @click="words = words.filter(x => x !== w)"
        >
          <X class="h-3 w-3" />
        </button>
      </span>
    </div>
    <form v-if="!readonly" class="flex gap-2" @submit.prevent="add">
      <Input v-model="draft" class="w-64" maxlength="50" :aria-label="label" :data-testid="`${testid}-input`" />
      <Button type="submit" variant="outline" size="sm" :data-testid="`${testid}-add`">{{ t('wl.consent.settings.add') }}</Button>
    </form>
    <p v-if="error" class="text-xs text-destructive" role="alert">{{ error }}</p>
  </div>
</template>
