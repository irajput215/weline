<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Loader2 } from 'lucide-vue-next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { Excluded, Preview } from '../api'

const props = defineProps<{ preview: Preview | null; loading: boolean; error: string | null; incomplete?: boolean }>()
const { t } = useI18n()
const MAX = 10000

const excluded = computed(() =>
  props.preview
    ? (Object.entries(props.preview.excluded) as [keyof Excluded, number][]).filter(([, n]) => n > 0)
    : []
)
</script>

<template>
  <Card data-testid="audience-preview">
    <CardHeader class="pb-2">
      <CardTitle class="text-base flex items-center gap-2">
        {{ t('wl.audiences.preview.title') }}
        <Loader2 v-if="loading" class="h-4 w-4 animate-spin" :aria-label="t('wl.audiences.preview.loading')" />
      </CardTitle>
    </CardHeader>
    <CardContent class="space-y-3 text-sm">
      <p v-if="error" class="text-destructive" role="alert">
        {{ t('wl.audiences.preview.failed', { msg: error === 'error' ? t('wl.audiences.saveFailed') : error }) }}
      </p>
      <p v-else-if="incomplete || !preview" class="text-muted-foreground">{{ t('wl.audiences.preview.incomplete') }}</p>
      <template v-else>
        <p class="text-2xl font-semibold tabular-nums" data-testid="preview-recipients">
          {{ t('wl.audiences.preview.recipients', { n: preview.recipients }) }}
        </p>
        <p class="text-muted-foreground" data-testid="preview-matched">{{ t('wl.audiences.preview.matched', { n: preview.matched }) }}</p>
        <div v-if="excluded.length">
          <p class="font-medium">{{ t('wl.audiences.preview.excludedTitle') }}</p>
          <ul class="list-disc pl-5 text-muted-foreground">
            <li v-for="[k, n] in excluded" :key="k" :data-testid="`excluded-${k}`">{{ t(`wl.audiences.preview.${k}`, { n, max: MAX }) }}</li>
          </ul>
        </div>
        <p v-if="preview.excluded.opted_out === 0 && preview.marketing_opt_out > 0" class="text-muted-foreground" data-testid="preview-marketing-opt-out">
          {{ t('wl.audiences.preview.marketingOptOut', { n: preview.marketing_opt_out }) }}
        </p>
        <div v-if="preview.sample.length">
          <p class="font-medium">{{ t('wl.audiences.preview.sample') }}</p>
          <ul class="text-muted-foreground" data-testid="preview-sample">
            <li v-for="(s, i) in preview.sample" :key="i" class="flex justify-between gap-4">
              <span class="truncate">{{ s.name || '—' }}</span><span class="tabular-nums">{{ s.phone_masked }}</span>
            </li>
          </ul>
        </div>
        <p v-else class="text-muted-foreground">{{ t('wl.audiences.preview.noOne') }}</p>
      </template>
    </CardContent>
  </Card>
</template>
