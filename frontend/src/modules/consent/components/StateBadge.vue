<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Badge } from '@/components/ui/badge'
import type { Marketing } from '../api'

defineProps<{ state: Marketing }>()
const { t } = useI18n()
const tone: Record<Marketing, string> = {
  opted_in: 'bg-emerald-500/15 text-emerald-500 light:text-emerald-700',
  opted_out: 'bg-red-500/15 text-red-400 light:text-red-700',
  not_recorded: 'bg-muted text-muted-foreground'
}
</script>

<template>
  <Badge variant="secondary" :class="tone[state]" :data-testid="`state-${state}`">{{ t(`wl.consent.states.${state}`) }}</Badge>
</template>
