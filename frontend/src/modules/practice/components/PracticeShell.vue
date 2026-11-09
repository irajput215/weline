<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink, useRoute } from 'vue-router'
import { Building2, Lock } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { PageHeader, ErrorState } from '@/components/shared'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { ScrollArea } from '@/components/ui/scroll-area'
import type { LoadState } from '../stores/practice'

const props = defineProps<{ state: LoadState }>()
defineEmits<{ retry: [] }>()

const { t } = useI18n()
const route = useRoute()
const auth = useAuthStore()

const tabs = computed(() =>
  [
    { to: '/x/practice/profile', label: 'wl.practice.tabs.profile', perm: 'wl_practice_settings' },
    { to: '/x/practice/locations', label: 'wl.practice.tabs.locations', perm: 'wl_practice_locations' },
    { to: '/x/practice/practitioners', label: 'wl.practice.tabs.practitioners', perm: 'wl_practice_practitioners' },
    { to: '/x/practice/services', label: 'wl.practice.tabs.services', perm: 'wl_practice_services' },
    { to: '/x/practice/hours', label: 'wl.practice.tabs.hours', perm: 'wl_practice_settings' }
  ].filter(tab => auth.hasPermission(tab.perm, 'read'))
)
const loading = computed(() => props.state === 'loading' || props.state === 'idle')
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader
      :title="t('wl.practice.title')"
      :description="t('wl.practice.subtitle')"
      :icon="Building2"
      icon-gradient="bg-gradient-to-br from-teal-500 to-emerald-600 shadow-teal-500/20"
    />
    <nav class="flex gap-1 border-b border-border px-6" :aria-label="t('wl.practice.title')">
      <RouterLink
        v-for="tab in tabs"
        :key="tab.to"
        :to="tab.to"
        class="px-3 py-2 text-sm border-b-2 -mb-px"
        :class="route.path === tab.to ? 'border-primary text-foreground font-medium' : 'border-transparent text-muted-foreground hover:text-foreground'"
        :data-testid="`practice-tab-${tab.to.split('/').pop()}`"
      >
        {{ t(tab.label) }}
      </RouterLink>
    </nav>

    <EmptyState
      v-if="state === 'forbidden'"
      data-testid="practice-forbidden"
      :icon="Lock"
      :title="t('wl.practice.forbiddenTitle')"
      :description="t('wl.practice.forbiddenDescription')"
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
          <div v-if="loading" class="space-y-3" data-testid="practice-loading">
            <Skeleton v-for="i in 4" :key="i" class="h-12 w-full" />
          </div>
          <slot v-else />
        </div>
      </div>
    </ScrollArea>
  </div>
</template>
