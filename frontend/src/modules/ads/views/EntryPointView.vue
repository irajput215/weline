<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { Archive, ArrowLeft, Pencil, Trash2, Users } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { ConfirmDialog } from '@/components/shared'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import AdsShell from '../components/AdsShell.vue'
import CopyButton from '../components/CopyButton.vue'
import EntryPointDialog from '../components/EntryPointDialog.vue'
import PeriodPicker from '../components/PeriodPicker.vue'
import SaveError from '../components/SaveError.vue'
import { formatDate, useAdsStore } from '../stores/ads'

const STATS = ['conversations', 'new_contacts', 'opted_in', 'booked'] as const
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const store = useAdsStore()
const id = computed(() => String(route.params.id))
const e = computed(() => store.entry)
const canEdit = computed(() => auth.hasPermission('wl_ads_entrypoints', 'update') && !e.value?.archived_at)
const canDelete = computed(() => auth.hasPermission('wl_ads_entrypoints', 'delete'))
const editOpen = ref(false)
const archiveOpen = ref(false)
const deleteOpen = ref(false)
// Delete is offered only while nobody has arrived (R3); the server checks all time, not just this period.
const noArrivals = computed(() => store.arrivals.length === 0)

const load = () => store.loadEntry(id.value)
async function archive() {
  if (await store.archive(id.value)) {
    archiveOpen.value = false
    toast.success(t('wl.ads.detail.archivedToast'))
  }
}
async function remove() {
  if (await store.remove(id.value)) {
    deleteOpen.value = false
    toast.success(t('wl.ads.detail.deletedToast'))
    void router.push('/x/ads')
  }
}
watch(id, load)
onMounted(load)
</script>

<template>
  <AdsShell :state="store.entryState" :title="e?.name" @retry="load">
    <template #actions>
      <Button variant="ghost" as-child>
        <RouterLink to="/x/ads"><ArrowLeft class="h-4 w-4 mr-2" />{{ t('wl.ads.detail.back') }}</RouterLink>
      </Button>
    </template>

    <template v-if="e">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex items-center gap-2">
          <span class="font-mono text-lg" data-testid="ads-detail-code">{{ e.code }}</span>
          <Badge variant="secondary">{{ t(`wl.ads.channels.${e.channel}`) }}</Badge>
          <Badge v-if="e.campaign" variant="outline">{{ e.campaign }}</Badge>
          <Badge v-if="e.archived_at" variant="destructive" data-testid="ads-detail-archived">{{ t('wl.ads.archived') }}</Badge>
        </div>
        <div class="flex gap-2">
          <Button v-if="canEdit" variant="outline" data-testid="ads-edit" @click="editOpen = true">
            <Pencil class="h-4 w-4 mr-2" />{{ t('wl.ads.detail.edit') }}
          </Button>
          <Button v-if="canDelete && !e.archived_at" variant="outline" data-testid="ads-archive" @click="archiveOpen = true">
            <Archive class="h-4 w-4 mr-2" />{{ t('wl.ads.detail.archive') }}
          </Button>
          <Button v-if="canDelete && noArrivals" variant="outline" data-testid="ads-delete" @click="deleteOpen = true">
            <Trash2 class="h-4 w-4 mr-2" />{{ t('wl.ads.detail.delete') }}
          </Button>
        </div>
      </div>
      <SaveError />

      <Card>
        <CardHeader><CardTitle class="text-base">{{ t('wl.ads.detail.linkTitle') }}</CardTitle></CardHeader>
        <CardContent class="space-y-4">
          <div class="space-y-1">
            <p class="text-sm text-muted-foreground">{{ t('wl.ads.detail.link') }}</p>
            <div class="flex items-start gap-2">
              <code class="flex-1 rounded bg-muted px-2 py-1 text-xs break-all" data-testid="ads-detail-link">{{ e.link }}</code>
              <CopyButton :text="e.link" :label="t('wl.ads.copy')" />
            </div>
          </div>
          <div class="space-y-1">
            <p class="text-sm text-muted-foreground">{{ t('wl.ads.detail.text') }}</p>
            <div class="flex items-start gap-2">
              <p class="flex-1 rounded bg-muted px-2 py-1 text-sm" data-testid="ads-detail-text">{{ e.prefill_text }}</p>
              <CopyButton :text="e.prefill_text" :label="t('wl.ads.copy')" />
            </div>
            <p class="text-xs text-muted-foreground">{{ t('wl.ads.detail.textHelp') }}</p>
          </div>
          <p class="text-sm">
            {{ t('wl.ads.detail.tags') }}
            <Badge variant="secondary" class="font-mono" data-testid="ads-detail-tag">{{ e.tag }}</Badge>
            <Badge v-if="e.campaign_tag" variant="secondary" class="font-mono ml-1">{{ e.campaign_tag }}</Badge>
          </p>
        </CardContent>
      </Card>

      <div class="flex flex-wrap items-center justify-between gap-3">
        <PeriodPicker @change="load" />
      </div>
      <div class="grid gap-3 grid-cols-2 sm:grid-cols-4">
        <Card v-for="k in STATS" :key="k">
          <CardContent class="p-4">
            <p class="text-sm text-muted-foreground">{{ t(`wl.ads.stats.${k}`) }}</p>
            <p class="text-2xl font-semibold tabular-nums" :data-testid="`ads-detail-${k}`">{{ e.stats[k] }}</p>
          </CardContent>
        </Card>
      </div>

      <EmptyState
        v-if="store.arrivals.length === 0"
        data-testid="ads-arrivals-empty"
        :icon="Users"
        :title="t('wl.ads.detail.noArrivals')"
        :description="t('wl.ads.detail.noArrivalsDescription')"
      />
      <Card v-else>
        <CardHeader><CardTitle class="text-base">{{ t('wl.ads.detail.arrivals') }}</CardTitle></CardHeader>
        <CardContent class="p-0">
          <Table data-testid="ads-arrivals">
            <TableHeader>
              <TableRow>
                <TableHead>{{ t('wl.ads.detail.when') }}</TableHead>
                <TableHead>{{ t('wl.ads.detail.contact') }}</TableHead>
                <TableHead>{{ t('wl.ads.detail.newOrReturning') }}</TableHead>
                <TableHead>{{ t('wl.ads.stats.opted_in') }}</TableHead>
                <TableHead>{{ t('wl.ads.stats.booked') }}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="a in store.arrivals" :key="a.id" :data-testid="`ads-arrival-${a.contact.name}`">
                <TableCell class="whitespace-nowrap">{{ formatDate(a.arrived_at, true) }}</TableCell>
                <TableCell>
                  <span class="font-medium">{{ a.contact.name || '—' }}</span>
                  <p class="text-xs text-muted-foreground tabular-nums">{{ a.contact.phone_masked }}</p>
                </TableCell>
                <TableCell>{{ a.is_new ? t('wl.ads.detail.new') : t('wl.ads.detail.returning') }}</TableCell>
                <TableCell>{{ a.opted_in ? t('wl.ads.yes') : '—' }}</TableCell>
                <TableCell>{{ a.booked ? t('wl.ads.yes') : '—' }}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <div v-if="store.nextBefore" class="text-center">
        <Button variant="ghost" :disabled="store.loadingMore" @click="store.loadMore()">{{ t('wl.ads.detail.loadMore') }}</Button>
      </div>

      <EntryPointDialog v-if="canEdit" v-model:open="editOpen" :entry="e" />
      <ConfirmDialog
        v-model:open="archiveOpen"
        :title="t('wl.ads.detail.archiveTitle')"
        :description="t('wl.ads.detail.archiveDescription', { code: e.code })"
        :confirm-label="t('wl.ads.detail.archive')"
        :cancel-label="t('wl.ads.cancel')"
        :is-submitting="store.saving"
        @confirm="archive"
      />
      <ConfirmDialog
        v-model:open="deleteOpen"
        :title="t('wl.ads.detail.deleteTitle')"
        :description="t('wl.ads.detail.deleteDescription')"
        :confirm-label="t('wl.ads.detail.delete')"
        :cancel-label="t('wl.ads.cancel')"
        variant="destructive"
        :is-submitting="store.saving"
        @confirm="remove"
      />
    </template>
  </AdsShell>
</template>
