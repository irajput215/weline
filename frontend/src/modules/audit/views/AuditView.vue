<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { History, Lock } from 'lucide-vue-next'
import { PageHeader, ErrorState } from '@/components/shared'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useAuditStore } from '../stores/audit'

const ALL = 'all'
const MODULES = ['connectors', 'practice', 'consent', 'ads'] as const
const { t, te } = useI18n()
const store = useAuditStore()
const loading = computed(() => store.state === 'loading' || store.state === 'idle')
const label = (key: string, fallback: string) => (te(key) ? t(key) : fallback)
const when = (iso: string) =>
  new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', year: 'numeric', hour: 'numeric', minute: '2-digit' }).format(new Date(iso))
const details = (d: Record<string, unknown>) =>
  Object.entries(d ?? {})
    .map(([k, v]) => `${k}: ${String(v)}`)
    .join(', ')

function setModule(v: unknown) {
  store.module = v === ALL ? '' : String(v)
  void store.load()
}
onMounted(store.load)
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader
      :title="t('wl.audit.title')"
      :description="t('wl.audit.subtitle')"
      :icon="History"
      icon-gradient="bg-gradient-to-br from-slate-500 to-slate-700 shadow-slate-500/20"
    />
    <EmptyState
      v-if="store.state === 'forbidden'"
      data-testid="audit-forbidden"
      :icon="Lock"
      :title="t('wl.audit.forbiddenTitle')"
      :description="t('wl.audit.forbiddenDescription')"
      class="flex-1"
    />
    <ErrorState
      v-else-if="store.state === 'error'"
      :title="t('common.loadErrorTitle')"
      :description="t('common.loadErrorDescription')"
      :retry-label="t('common.retryLoad')"
      class="flex-1"
      @retry="store.load"
    />
    <ScrollArea v-else class="flex-1">
      <div class="p-6">
        <div class="max-w-5xl mx-auto space-y-4">
          <Select :model-value="store.module || ALL" @update:model-value="setModule">
            <SelectTrigger class="w-56" :aria-label="t('wl.audit.module')" data-testid="audit-module"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem :value="ALL">{{ t('wl.audit.allModules') }}</SelectItem>
              <SelectItem v-for="m in MODULES" :key="m" :value="m">{{ t(`wl.audit.modules.${m}`) }}</SelectItem>
            </SelectContent>
          </Select>
          <div v-if="loading" class="space-y-3" data-testid="audit-loading">
            <Skeleton v-for="i in 4" :key="i" class="h-10 w-full" />
          </div>
          <EmptyState v-else-if="store.events.length === 0" data-testid="audit-empty" :icon="History" :title="t('wl.audit.empty')" />
          <Card v-else>
            <CardContent class="p-0">
              <Table data-testid="audit-table">
                <TableHeader>
                  <TableRow>
                    <TableHead>{{ t('wl.audit.when') }}</TableHead>
                    <TableHead>{{ t('wl.audit.who') }}</TableHead>
                    <TableHead>{{ t('wl.audit.what') }}</TableHead>
                    <TableHead>{{ t('wl.audit.details') }}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow v-for="e in store.events" :key="e.id" :data-testid="`audit-row-${e.action}`">
                    <TableCell class="whitespace-nowrap">{{ when(e.at) }}</TableCell>
                    <TableCell>{{ e.actor?.name || t('wl.audit.system') }}</TableCell>
                    <TableCell>
                      {{ label(`wl.audit.modules.${e.module}`, e.module) }} · {{ label(`wl.audit.actions.${e.action}`, e.action) }}
                    </TableCell>
                    <TableCell class="text-muted-foreground text-xs">{{ details(e.details) }}</TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </CardContent>
          </Card>
          <div v-if="store.nextBefore" class="text-center">
            <Button variant="ghost" :disabled="store.loadingMore" @click="store.loadMore()">{{ t('wl.audit.loadMore') }}</Button>
          </div>
        </div>
      </div>
    </ScrollArea>
  </div>
</template>
