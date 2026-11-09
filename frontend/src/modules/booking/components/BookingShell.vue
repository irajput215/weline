<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink, useRoute } from 'vue-router'
import { CalendarCheck, Lock } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { PageHeader, ErrorState } from '@/components/shared'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { ScrollArea } from '@/components/ui/scroll-area'
import type { LoadState } from '../stores/booking'

const props = defineProps<{ state: LoadState }>()
defineEmits<{ retry: [] }>()

const { t } = useI18n()
const route = useRoute()
const auth = useAuthStore()

const tabs = computed(() =>
  [
    { to: '/x/booking/appointments', label: 'wl.booking.tabs.appointments', perm: 'wl_booking_appointments' },
    { to: '/x/booking/availability', label: 'wl.booking.tabs.availability', perm: 'wl_booking_availability' },
    { to: '/x/booking/online', label: 'wl.booking.tabs.online', perm: 'wl_booking_availability' }
  ].filter(tab => auth.hasPermission(tab.perm, 'read'))
)
const loading = computed(() => props.state === 'loading' || props.state === 'idle')
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader
      :title="t('wl.booking.title')"
      :description="t('wl.booking.subtitle')"
      :icon="CalendarCheck"
      icon-gradient="bg-gradient-to-br from-sky-500 to-indigo-600 shadow-sky-500/20"
    />
    <nav class="flex gap-1 border-b border-border px-6" :aria-label="t('wl.booking.title')">
      <RouterLink
        v-for="tab in tabs"
        :key="tab.to"
        :to="tab.to"
        class="px-3 py-2 text-sm border-b-2 -mb-px"
        :class="route.path === tab.to ? 'border-primary text-foreground font-medium' : 'border-transparent text-muted-foreground hover:text-foreground'"
        :data-testid="`booking-tab-${tab.to.split('/').pop()}`"
      >
        {{ t(tab.label) }}
      </RouterLink>
    </nav>

    <EmptyState
      v-if="state === 'forbidden'"
      data-testid="booking-forbidden"
      :icon="Lock"
      :title="t('wl.booking.forbiddenTitle')"
      :description="t('wl.booking.forbiddenDescription')"
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
          <div v-if="loading" class="space-y-3" data-testid="booking-loading">
            <Skeleton v-for="i in 4" :key="i" class="h-12 w-full" />
          </div>
          <slot v-else />
        </div>
      </div>
    </ScrollArea>
  </div>
</template>
