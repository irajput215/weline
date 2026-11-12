<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ShieldCheck, Upload, UserSearch } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { EmptyState } from '@/components/ui/empty-state'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import ConsentShell from '../components/ConsentShell.vue'
import ContactConsentDialog from '../components/ContactConsentDialog.vue'
import ImportDialog from '../components/ImportDialog.vue'
import { SOURCES, formatDate, useConsentStore } from '../stores/consent'

const ALL = 'all'
const { t } = useI18n()
const auth = useAuthStore()
const store = useConsentStore()
const canCreate = computed(() => auth.hasPermission('wl_consent_records', 'create'))
const contactOpen = ref(false)
const importOpen = ref(false)
const filtered = computed(() => !!(store.filters.action || store.filters.source || store.filters.from || store.filters.to))

function setFilter(key: 'action' | 'source', v: unknown) {
  ;(store.filters as Record<string, string>)[key] = v === ALL ? '' : String(v)
  void store.applyFilters()
}
onMounted(store.loadOverview)
</script>

<template>
  <ConsentShell :state="store.state.overview" @retry="store.loadOverview">
    <div v-if="store.summary" class="grid gap-3 sm:grid-cols-3">
      <Card v-for="k in (['opted_in', 'opted_out', 'not_recorded'] as const)" :key="k">
        <CardContent class="p-4">
          <p class="text-sm text-muted-foreground">{{ t(`wl.consent.states.${k}`) }}</p>
          <p class="text-2xl font-semibold tabular-nums" :data-testid="`consent-count-${k}`">{{ store.summary[k] }}</p>
        </CardContent>
      </Card>
    </div>
    <div class="flex flex-wrap items-center justify-between gap-3">
      <p class="text-sm text-muted-foreground" data-testid="consent-require">
        {{ store.summary?.require_opt_in ? t('wl.consent.overview.requireOn') : t('wl.consent.overview.requireOff') }}
      </p>
      <div class="flex gap-2">
        <Button variant="outline" data-testid="consent-find" @click="contactOpen = true">
          <UserSearch class="h-4 w-4 mr-2" />{{ t('wl.consent.overview.findContact') }}
        </Button>
        <Button v-if="canCreate" variant="outline" data-testid="consent-import" @click="importOpen = true">
          <Upload class="h-4 w-4 mr-2" />{{ t('wl.consent.overview.import') }}
        </Button>
      </div>
    </div>

    <div class="flex flex-wrap items-end gap-2">
      <Select :model-value="store.filters.action || ALL" @update:model-value="v => setFilter('action', v)">
        <SelectTrigger class="w-40" :aria-label="t('wl.consent.overview.change')" data-testid="consent-filter-action"><SelectValue /></SelectTrigger>
        <SelectContent>
          <SelectItem :value="ALL">{{ t('wl.consent.overview.allActions') }}</SelectItem>
          <SelectItem value="opt_in">{{ t('wl.consent.actions.opt_in') }}</SelectItem>
          <SelectItem value="opt_out">{{ t('wl.consent.actions.opt_out') }}</SelectItem>
        </SelectContent>
      </Select>
      <Select :model-value="store.filters.source || ALL" @update:model-value="v => setFilter('source', v)">
        <SelectTrigger class="w-48" :aria-label="t('wl.consent.overview.source')" data-testid="consent-filter-source"><SelectValue /></SelectTrigger>
        <SelectContent>
          <SelectItem :value="ALL">{{ t('wl.consent.overview.allSources') }}</SelectItem>
          <SelectItem v-for="s in SOURCES" :key="s" :value="s">{{ t(`wl.consent.sources.${s}`) }}</SelectItem>
        </SelectContent>
      </Select>
      <Input v-model="store.filters.from" type="date" class="w-40" :aria-label="t('wl.consent.overview.from')" @change="store.applyFilters()" />
      <Input v-model="store.filters.to" type="date" class="w-40" :aria-label="t('wl.consent.overview.to')" @change="store.applyFilters()" />
    </div>

    <EmptyState
      v-if="store.events.length === 0"
      data-testid="consent-empty"
      :icon="ShieldCheck"
      :title="filtered ? t('wl.consent.overview.noMatches') : t('wl.consent.overview.emptyTitle')"
      :description="filtered ? '' : t('wl.consent.overview.emptyDescription')"
    />
    <Card v-else>
      <CardContent class="p-0">
        <Table data-testid="consent-history">
          <TableHeader>
            <TableRow>
              <TableHead>{{ t('wl.consent.overview.when') }}</TableHead>
              <TableHead>{{ t('wl.consent.overview.contact') }}</TableHead>
              <TableHead>{{ t('wl.consent.overview.change') }}</TableHead>
              <TableHead>{{ t('wl.consent.overview.source') }}</TableHead>
              <TableHead>{{ t('wl.consent.overview.evidence') }}</TableHead>
              <TableHead>{{ t('wl.consent.overview.by') }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="e in store.events" :key="e.id" :data-testid="`consent-row-${e.contact.name}`">
              <TableCell class="whitespace-nowrap">
                {{ formatDate(e.recorded_at, true) }}
                <p v-if="e.given_at.slice(0, 10) !== e.recorded_at.slice(0, 10)" class="text-xs text-muted-foreground">
                  {{ t('wl.consent.overview.given', { date: formatDate(e.given_at) }) }}
                </p>
              </TableCell>
              <TableCell>
                <span class="font-medium">{{ e.contact.name || '—' }}</span>
                <p class="text-xs text-muted-foreground tabular-nums">{{ e.contact.phone_masked }}</p>
              </TableCell>
              <TableCell>{{ t(`wl.consent.actions.${e.action}`) }}</TableCell>
              <TableCell>{{ t(`wl.consent.sources.${e.source}`) }}</TableCell>
              <TableCell class="max-w-xs">
                <span v-if="e.keyword">{{ t('wl.consent.overview.keyword', { keyword: e.keyword }) }}</span>
                <span v-else class="line-clamp-2">{{ e.evidence }}</span>
              </TableCell>
              <TableCell>{{ e.recorded_by?.name || '—' }}</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>
    <div v-if="store.nextBefore" class="text-center">
      <Button variant="ghost" :disabled="store.loadingMore" @click="store.loadMore()">{{ t('wl.consent.overview.loadMore') }}</Button>
    </div>

    <ContactConsentDialog v-model:open="contactOpen" @changed="store.refresh" />
    <ImportDialog v-if="canCreate" v-model:open="importOpen" @done="store.refresh" />
  </ConsentShell>
</template>
