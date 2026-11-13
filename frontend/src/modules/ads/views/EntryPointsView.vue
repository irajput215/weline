<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Megaphone, Plus } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { EmptyState } from '@/components/ui/empty-state'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import AdsShell from '../components/AdsShell.vue'
import CopyButton from '../components/CopyButton.vue'
import EntryPointDialog from '../components/EntryPointDialog.vue'
import PeriodPicker from '../components/PeriodPicker.vue'
import type { EntryPoint } from '../api'
import { useAdsStore } from '../stores/ads'

const STATS = ['conversations', 'new_contacts', 'opted_in', 'booked'] as const
const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const store = useAdsStore()
const canCreate = computed(() => auth.hasPermission('wl_ads_entrypoints', 'create'))
const dialogOpen = ref(false)
const campaignNames = computed(() => [...new Set(store.entrypoints.map(e => e.campaign).filter(Boolean))])
const codes = computed(() => store.entrypoints.map(e => e.code))

function open(e: EntryPoint) {
  void router.push(`/x/ads/${e.id}`)
}
function toggleArchived(v: boolean) {
  store.showArchived = v
  void store.refreshList()
}
function saved(e: EntryPoint) {
  void router.push(`/x/ads/${e.id}`)
}
onMounted(store.loadList)
</script>

<template>
  <AdsShell :state="store.listState" @retry="store.loadList">
    <template #actions>
      <Button v-if="canCreate" data-testid="ads-new" @click="dialogOpen = true">
        <Plus class="h-4 w-4 mr-2" />{{ t('wl.ads.list.new') }}
      </Button>
    </template>

    <div class="flex flex-wrap items-center justify-between gap-3">
      <PeriodPicker @change="store.refreshList()" />
      <div class="flex items-center gap-2">
        <Switch id="wl-ads-archived" :checked="store.showArchived" data-testid="ads-show-archived" @update:checked="toggleArchived" />
        <Label for="wl-ads-archived">{{ t('wl.ads.list.showArchived') }}</Label>
      </div>
    </div>

    <EmptyState
      v-if="store.entrypoints.length === 0"
      data-testid="ads-empty"
      :icon="Megaphone"
      :title="t('wl.ads.list.emptyTitle')"
      :description="t('wl.ads.list.emptyDescription')"
    >
      <template v-if="canCreate" #action>
        <Button @click="dialogOpen = true"><Plus class="h-4 w-4 mr-2" />{{ t('wl.ads.list.new') }}</Button>
      </template>
    </EmptyState>

    <template v-else>
      <div v-if="store.campaigns.length" class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3" data-testid="ads-campaigns">
        <Card v-for="c in store.campaigns" :key="c.campaign">
          <CardContent class="p-4">
            <p class="font-medium truncate">{{ c.campaign }}</p>
            <dl class="mt-2 grid grid-cols-4 gap-2 text-center">
              <div v-for="k in STATS" :key="k">
                <dt class="text-xs text-muted-foreground">{{ t(`wl.ads.stats.${k}`) }}</dt>
                <dd class="text-lg font-semibold tabular-nums" :data-testid="`ads-campaign-${k}`">{{ c[k] }}</dd>
              </div>
            </dl>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardContent class="p-0">
          <Table data-testid="ads-table">
            <TableHeader>
              <TableRow>
                <TableHead>{{ t('wl.ads.list.name') }}</TableHead>
                <TableHead>{{ t('wl.ads.list.code') }}</TableHead>
                <TableHead>{{ t('wl.ads.list.channel') }}</TableHead>
                <TableHead>{{ t('wl.ads.list.campaign') }}</TableHead>
                <TableHead v-for="k in STATS" :key="k" class="text-right">{{ t(`wl.ads.stats.${k}`) }}</TableHead>
                <TableHead><span class="sr-only">{{ t('wl.ads.list.link') }}</span></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow
                v-for="e in store.entrypoints"
                :key="e.id"
                class="cursor-pointer"
                tabindex="0"
                :data-testid="`ads-row-${e.code}`"
                @click="open(e)"
                @keydown.enter="open(e)"
              >
                <TableCell class="font-medium">
                  {{ e.name }}
                  <Badge v-if="e.archived_at" variant="secondary" class="ml-2">{{ t('wl.ads.archived') }}</Badge>
                </TableCell>
                <TableCell class="font-mono">{{ e.code }}</TableCell>
                <TableCell>{{ t(`wl.ads.channels.${e.channel}`) }}</TableCell>
                <TableCell>{{ e.campaign || '—' }}</TableCell>
                <TableCell v-for="k in STATS" :key="k" class="text-right tabular-nums" :data-testid="`ads-${e.code}-${k}`">
                  {{ e.stats[k] }}
                </TableCell>
                <TableCell class="text-right"><CopyButton :text="e.link" /></TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <p class="text-xs text-muted-foreground">{{ t('wl.ads.list.creditNote') }}</p>
    </template>

    <EntryPointDialog v-if="canCreate" v-model:open="dialogOpen" :campaigns="campaignNames" :taken-codes="codes" @saved="saved" />
  </AdsShell>
</template>
