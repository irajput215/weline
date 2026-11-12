<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink, useRoute } from 'vue-router'
import { Lock, UsersRound } from 'lucide-vue-next'
import { PageHeader, ErrorState } from '@/components/shared'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { ScrollArea } from '@/components/ui/scroll-area'
import type { LoadState } from '../stores/audiences'

const props = defineProps<{ state: LoadState }>()
defineEmits<{ retry: [] }>()

const { t } = useI18n()
const route = useRoute()

const tabs = [
  { to: '/x/audiences', label: 'wl.audiences.tabs.segments', key: 'segments' },
  { to: '/x/audiences/retarget', label: 'wl.audiences.tabs.retarget', key: 'retarget' },
  { to: '/x/audiences/history', label: 'wl.audiences.tabs.history', key: 'history' }
]
const active = (to: string) => (to === '/x/audiences' ? route.path === to || route.path.startsWith('/x/audiences/segments') : route.path === to)
const loading = computed(() => props.state === 'loading' || props.state === 'idle')
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader
      :title="t('wl.audiences.title')"
      :description="t('wl.audiences.subtitle')"
      :icon="UsersRound"
      icon-gradient="bg-gradient-to-br from-emerald-500 to-teal-600 shadow-emerald-500/20"
    />
    <nav class="flex gap-1 border-b border-border px-6" :aria-label="t('wl.audiences.title')">
      <RouterLink
        v-for="tab in tabs"
        :key="tab.to"
        :to="tab.to"
        class="px-3 py-2 text-sm border-b-2 -mb-px"
        :class="active(tab.to) ? 'border-primary text-foreground font-medium' : 'border-transparent text-muted-foreground hover:text-foreground'"
        :data-testid="`audiences-tab-${tab.key}`"
      >
        {{ t(tab.label) }}
      </RouterLink>
    </nav>

    <EmptyState
      v-if="state === 'forbidden'"
      data-testid="audiences-forbidden"
      :icon="Lock"
      :title="t('wl.audiences.forbiddenTitle')"
      :description="t('wl.audiences.forbiddenDescription')"
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
        <div class="max-w-5xl mx-auto space-y-6">
          <div v-if="loading" class="space-y-3" data-testid="audiences-loading">
            <Skeleton v-for="i in 4" :key="i" class="h-12 w-full" />
          </div>
          <slot v-else />
        </div>
      </div>
    </ScrollArea>
  </div>
</template>
