<script setup lang="ts">
import { onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { History } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { Card, CardContent } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import AudiencesShell from '../components/AudiencesShell.vue'
import { formatDate, useAudiencesStore } from '../stores/audiences'
import type { Send } from '../api'

const { t } = useI18n()
const auth = useAuthStore()
const store = useAudiencesStore()
const canOpen = auth.hasPermission('campaigns', 'read')

const excludedTotal = (s: Send) => Object.values(s.excluded ?? {}).reduce((a, n) => a + (Number(n) || 0), 0)
const load = () => store.loadSends()
onMounted(load)
</script>

<template>
  <AudiencesShell :state="store.state.sends" @retry="load">
    <EmptyState
      v-if="store.sends.length === 0"
      data-testid="history-empty"
      :icon="History"
      :title="t('wl.audiences.history.emptyTitle')"
      :description="t('wl.audiences.history.emptyDescription')"
    />
    <Card v-else>
      <CardContent class="p-0">
        <Table data-testid="history-table">
          <TableHeader>
            <TableRow>
              <TableHead>{{ t('wl.audiences.history.campaign') }}</TableHead>
              <TableHead>{{ t('wl.audiences.history.from') }}</TableHead>
              <TableHead class="text-right">{{ t('wl.audiences.history.recipients') }}</TableHead>
              <TableHead class="text-right">{{ t('wl.audiences.history.excluded') }}</TableHead>
              <TableHead>{{ t('wl.audiences.history.by') }}</TableHead>
              <TableHead>{{ t('wl.audiences.history.when') }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="s in store.sends" :key="s.id" :data-testid="`history-row-${s.campaign_name}`">
              <TableCell class="font-medium">
                <RouterLink v-if="canOpen" :to="`/campaigns/${s.campaign_id}`" class="hover:underline" :title="t('wl.audiences.history.open')">
                  {{ s.campaign_name }}
                </RouterLink>
                <span v-else>{{ s.campaign_name }}</span>
              </TableCell>
              <TableCell>
                <RouterLink v-if="s.kind === 'segment' && s.segment_id" :to="`/x/audiences/segments/${s.segment_id}`" class="hover:underline">
                  {{ t('wl.audiences.history.segment') }}
                </RouterLink>
                <span v-else-if="s.kind === 'segment'">{{ t('wl.audiences.history.segment') }}</span>
                <span v-else>{{ t('wl.audiences.history.retarget', { filter: t(`wl.audiences.retarget.filters.${s.retarget_filter}`) }) }}</span>
              </TableCell>
              <TableCell class="text-right tabular-nums">{{ s.recipient_count }}</TableCell>
              <TableCell class="text-right tabular-nums">{{ excludedTotal(s) }}</TableCell>
              <TableCell>{{ s.created_by?.name || '—' }}</TableCell>
              <TableCell>{{ formatDate(s.created_at, true) }}</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  </AudiencesShell>
</template>
