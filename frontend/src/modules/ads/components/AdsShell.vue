<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Lock, Megaphone } from 'lucide-vue-next'
import { PageHeader, ErrorState } from '@/components/shared'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { ScrollArea } from '@/components/ui/scroll-area'
import type { LoadState } from '../stores/ads'

// The frame of every ads page: header, and the loading, error and forbidden states.
const props = defineProps<{ state: LoadState; title?: string }>()
defineEmits<{ retry: [] }>()

const { t } = useI18n()
const loading = computed(() => props.state === 'loading' || props.state === 'idle')
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader
      :title="title || t('wl.ads.title')"
      :description="t('wl.ads.subtitle')"
      :icon="Megaphone"
      icon-gradient="bg-gradient-to-br from-amber-500 to-orange-600 shadow-amber-500/20"
    >
      <template #actions><slot name="actions" /></template>
    </PageHeader>

    <EmptyState
      v-if="state === 'forbidden'"
      data-testid="ads-forbidden"
      :icon="Lock"
      :title="t('wl.ads.forbiddenTitle')"
      :description="t('wl.ads.forbiddenDescription')"
      class="flex-1"
    />
    <ErrorState
      v-else-if="state === 'error'"
      :title="t('common.loadErrorTitle')"
      :description="t('common.loadErrorDescription')"
      :retry-label="t('common.retryLoad')"
      class="flex-1"
      @retry="$emit('retry')"
    />
    <ScrollArea v-else class="flex-1">
      <div class="p-6">
        <div class="max-w-6xl mx-auto space-y-6">
          <div v-if="loading" class="space-y-3" data-testid="ads-loading">
            <Skeleton v-for="i in 4" :key="i" class="h-12 w-full" />
          </div>
          <slot v-else />
        </div>
      </div>
    </ScrollArea>
  </div>
</template>
