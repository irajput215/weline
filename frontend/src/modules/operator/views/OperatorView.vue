<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Building2, Lock, X } from 'lucide-vue-next'
import { PageHeader, ErrorState } from '@/components/shared'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useOperatorStore } from '../stores/operator'

const { t } = useI18n()
const store = useOperatorStore()
const q = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
const loading = computed(() => store.state === 'loading' || store.state === 'idle')
const o = computed(() => store.overview)
const d = computed(() => store.detail)
const day = (iso: string | null) => (iso ? new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', year: 'numeric' }).format(new Date(iso)) : t('wl.operator.never'))
const planLabel = (p: string) => t(`wl.operator.plans.${p || 'none'}`)
function onSearch() {
  clearTimeout(timer)
  timer = setTimeout(() => void store.find(q.value), 300)
}
onMounted(() => void store.load())
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="t('wl.operator.title')" :description="t('wl.operator.subtitle')" :icon="Building2"
      icon-gradient="bg-gradient-to-br from-slate-500 to-zinc-700 shadow-slate-500/20" />
    <EmptyState v-if="store.state === 'forbidden'" data-testid="operator-forbidden" :icon="Lock" :title="t('wl.operator.forbiddenTitle')"
      :description="t('wl.operator.forbiddenDescription')" class="flex-1" />
    <ErrorState v-else-if="store.state === 'error'" :title="t('common.loadErrorTitle')" :description="t('common.loadErrorDescription')"
      :retry-label="t('common.retryLoad')" class="flex-1" @retry="store.load()" />
    <ScrollArea v-else class="flex-1">
      <div class="p-6"><div class="max-w-6xl mx-auto space-y-6">
        <div v-if="loading" class="space-y-3" data-testid="operator-loading"><Skeleton v-for="i in 4" :key="i" class="h-16 w-full" /></div>
        <template v-else-if="o">
          <div class="grid gap-3 grid-cols-2 md:grid-cols-5" data-testid="operator-overview">
            <Card v-for="(v, k) in { organizations: o.organizations, signups7: o.signups_7d, signups30: o.signups_30d, connectorErrors: o.connector_errors, failedSends: o.with_failed_sends_7d }"
              :key="k" :data-testid="`operator-stat-${k}`">
              <CardContent class="p-4"><p class="text-2xl font-semibold">{{ v }}</p><p class="text-xs text-muted-foreground">{{ t(`wl.operator.stats.${k}`) }}</p></CardContent>
            </Card>
          </div>
          <p class="flex flex-wrap gap-2 text-sm">
            <Badge v-for="(n, p) in o.by_plan" :key="p" variant="secondary">{{ planLabel(String(p)) }}: {{ n }}</Badge>
          </p>

          <Card v-if="store.detailState !== 'idle'" data-testid="operator-detail">
            <CardHeader class="flex flex-row items-center justify-between pb-2">
              <CardTitle class="text-base">{{ d?.name ?? t('wl.operator.detail') }}</CardTitle>
              <Button variant="ghost" size="sm" :aria-label="t('wl.operator.close')" @click="store.detailState = 'idle'"><X class="h-4 w-4" /></Button>
            </CardHeader>
            <CardContent>
              <Skeleton v-if="store.detailState === 'loading'" class="h-24 w-full" />
              <p v-else-if="store.detailState !== 'ready'" class="text-sm text-destructive">{{ t('common.loadErrorDescription') }}</p>
              <div v-else-if="d" class="grid gap-6 sm:grid-cols-3 text-sm">
                <dl class="space-y-1">
                  <div v-if="d.owner"><dt class="text-muted-foreground">{{ t('wl.operator.owner') }}</dt><dd data-testid="operator-owner">{{ d.owner.name }} · {{ d.owner.email }}</dd></div>
                  <div><dt class="text-muted-foreground">{{ t('wl.operator.plan') }}</dt><dd>{{ planLabel(d.plan) }} <span v-if="d.plan_status">({{ d.plan_status }})</span></dd></div>
                  <div><dt class="text-muted-foreground">{{ t('wl.operator.setup') }}</dt><dd data-testid="operator-setup">{{ t('wl.operator.setupDone', { n: d.setup_done }) }}</dd></div>
                  <div><dt class="text-muted-foreground">{{ t('wl.operator.stripe') }}</dt><dd>{{ d.has_stripe_customer ? t('wl.operator.yes') : t('wl.operator.no') }}</dd></div>
                </dl>
                <dl class="space-y-1">
                  <dt class="font-medium">{{ t('wl.operator.usage') }}</dt>
                  <div class="flex justify-between"><span class="text-muted-foreground">{{ t('wl.operator.broadcast') }}</span><span>{{ d.usage.broadcast_recipients }}</span></div>
                  <div class="flex justify-between"><span class="text-muted-foreground">{{ t('wl.operator.aiRuns') }}</span><span>{{ d.usage.ai_runs }}</span></div>
                </dl>
                <dl class="space-y-1" data-testid="operator-health">
                  <dt class="font-medium">{{ t('wl.operator.health') }}</dt>
                  <div class="flex justify-between"><span class="text-muted-foreground">{{ t('wl.operator.failedMessages') }}</span><span>{{ d.health.failed_messages }}</span></div>
                  <div class="flex justify-between"><span class="text-muted-foreground">{{ t('wl.operator.failedReminders') }}</span><span>{{ d.health.failed_reminders }}</span></div>
                  <div class="flex justify-between"><span class="text-muted-foreground">{{ t('wl.operator.skippedReminders') }}</span><span>{{ d.health.skipped_reminders }}</span></div>
                  <div class="flex justify-between"><span class="text-muted-foreground">{{ t('wl.operator.aiErrors') }}</span><span>{{ d.health.ai_errors }}</span></div>
                  <div v-if="d.health.connector_error"><span class="text-muted-foreground">{{ t('wl.operator.connectorError') }}:</span> {{ d.health.connector_error }}</div>
                  <div v-if="d.health.connector_last_sync_at"><span class="text-muted-foreground">{{ t('wl.operator.lastSync') }}:</span> {{ day(d.health.connector_last_sync_at) }}</div>
                </dl>
              </div>
              <p class="text-xs text-muted-foreground mt-4">{{ t('wl.operator.audited') }}</p>
            </CardContent>
          </Card>

          <Input v-model="q" :placeholder="t('wl.operator.search')" class="max-w-sm" maxlength="100" data-testid="operator-search" @input="onSearch" />
          <EmptyState v-if="store.tenants.length === 0" data-testid="operator-empty" :icon="Building2" :title="t('wl.operator.empty')" />
          <Card v-else><CardContent class="p-0"><Table data-testid="operator-tenants">
            <TableHeader><TableRow>
              <TableHead>{{ t('wl.operator.business') }}</TableHead><TableHead>{{ t('wl.operator.joined') }}</TableHead><TableHead>{{ t('wl.operator.plan') }}</TableHead>
              <TableHead>{{ t('wl.operator.system') }}</TableHead><TableHead>{{ t('wl.operator.people') }}</TableHead><TableHead>{{ t('wl.operator.lastMessage') }}</TableHead>
            </TableRow></TableHeader>
            <TableBody><TableRow v-for="tn in store.tenants" :key="tn.id" class="cursor-pointer" :data-testid="`operator-tenant-${tn.name}`" @click="store.open(tn.id)">
              <TableCell><span class="font-medium">{{ tn.name }}</span><p class="text-xs text-muted-foreground">{{ t(`wl.operator.sources.${tn.source}`) }}</p></TableCell>
              <TableCell class="whitespace-nowrap">{{ day(tn.created_at) }}</TableCell>
              <TableCell>{{ planLabel(tn.plan) }}</TableCell>
              <TableCell>
                <span v-if="tn.connector">{{ tn.connector }}</span><span v-else class="text-muted-foreground">{{ t('wl.operator.notSetUp') }}</span>
                <Badge v-if="tn.connector_status === 'error'" variant="destructive" class="ml-1">{{ t('wl.operator.connectorStatuses.error') }}</Badge>
              </TableCell>
              <TableCell>{{ tn.members }} / {{ tn.contacts }}</TableCell>
              <TableCell class="whitespace-nowrap">{{ day(tn.last_message_at) }}</TableCell>
            </TableRow></TableBody>
          </Table></CardContent></Card>
          <div v-if="store.nextOffset !== null" class="text-center"><Button variant="ghost" @click="store.loadMore()">{{ t('wl.operator.loadMore') }}</Button></div>
        </template>
      </div></div>
    </ScrollArea>
  </div>
</template>
