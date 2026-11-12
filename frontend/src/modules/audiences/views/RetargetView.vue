<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { BookmarkPlus, Repeat, Send } from 'lucide-vue-next'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EmptyState } from '@/components/ui/empty-state'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import AudiencesShell from '../components/AudiencesShell.vue'
import CreateCampaignDialog from '../components/CreateCampaignDialog.vue'
import PreviewPanel from '../components/PreviewPanel.vue'
import { RETARGET_FILTERS, useAudiencesStore } from '../stores/audiences'
import type { Retarget, RetargetFilter } from '../api'

const { t } = useI18n()
const auth = useAuthStore()
const store = useAudiencesStore()
const router = useRouter()
const canSaveSegment = computed(() => auth.hasPermission('wl_audiences_segments', 'create'))

/** Opens the builder with this retarget as a rule (R9). */
function saveAsSegment() {
  if (!retarget.value) return
  const query: Record<string, string> = { campaign: retarget.value.campaign_id, op: retarget.value.filter }
  if (retarget.value.days) query.days = String(retarget.value.days)
  void router.push({ path: '/x/audiences/segments/new', query })
}

const canSend = computed(() => auth.hasPermission('wl_audiences_sends', 'create') && auth.hasPermission('campaigns', 'write'))
const campaignId = ref('')
const filter = ref<RetargetFilter>('not_read')
const days = ref<number | undefined>(undefined)
const sendOpen = ref(false)

const campaigns = computed(() => store.fields?.campaigns ?? [])
const chosen = computed(() => campaigns.value.find(c => c.id === campaignId.value) ?? null)
const needsDays = computed(() => filter.value === 'read_no_reply' || filter.value === 'replied')
const retarget = computed<Retarget | null>(() =>
  campaignId.value ? { campaign_id: campaignId.value, filter: filter.value, ...(needsDays.value && days.value ? { days: days.value } : {}) } : null
)

watch(retarget, r => {
  if (r) void store.runPreview({ retarget: r })
  else store.resetPreview()
}, { deep: true })

async function load() {
  store.resetPreview()
  await store.loadFields()
}
onMounted(load)
</script>

<template>
  <AudiencesShell :state="store.state.fields" @retry="load">
    <EmptyState
      v-if="campaigns.length === 0"
      data-testid="retarget-empty"
      :icon="Repeat"
      :title="t('wl.audiences.retarget.emptyTitle')"
      :description="t('wl.audiences.retarget.emptyDescription')"
    />
    <template v-else>
      <Card>
        <CardHeader>
          <CardTitle>{{ t('wl.audiences.retarget.title') }}</CardTitle>
          <CardDescription>{{ t('wl.audiences.retarget.description') }}</CardDescription>
        </CardHeader>
        <CardContent class="p-0">
          <Table data-testid="retarget-campaigns">
            <TableHeader>
              <TableRow>
                <TableHead>{{ t('wl.audiences.retarget.campaign') }}</TableHead>
                <TableHead class="text-right">{{ t('wl.audiences.retarget.total') }}</TableHead>
                <TableHead class="text-right">{{ t('wl.audiences.retarget.sent') }}</TableHead>
                <TableHead class="text-right">{{ t('wl.audiences.retarget.delivered') }}</TableHead>
                <TableHead class="text-right">{{ t('wl.audiences.retarget.read') }}</TableHead>
                <TableHead class="text-right">{{ t('wl.audiences.retarget.failed') }}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow
                v-for="c in campaigns"
                :key="c.id"
                class="cursor-pointer"
                :class="c.id === campaignId ? 'bg-muted' : ''"
                :data-testid="`retarget-row-${c.name}`"
                :aria-selected="c.id === campaignId"
                tabindex="0"
                @click="campaignId = c.id"
                @keydown.enter="campaignId = c.id"
              >
                <TableCell class="font-medium">{{ c.name }}</TableCell>
                <TableCell class="text-right tabular-nums">{{ c.total }}</TableCell>
                <TableCell class="text-right tabular-nums">{{ c.sent }}</TableCell>
                <TableCell class="text-right tabular-nums">{{ c.delivered }}</TableCell>
                <TableCell class="text-right tabular-nums">{{ c.read }}</TableCell>
                <TableCell class="text-right tabular-nums">{{ c.failed }}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <div v-if="chosen" class="grid gap-6 lg:grid-cols-[1fr_18rem]">
        <Card>
          <CardHeader><CardTitle class="text-base">{{ chosen.name }}</CardTitle></CardHeader>
          <CardContent class="space-y-4">
            <div class="flex flex-wrap items-end gap-3">
              <div class="space-y-1">
                <Label>{{ t('wl.audiences.retarget.filter') }}</Label>
                <Select v-model="filter">
                  <SelectTrigger class="w-64" :aria-label="t('wl.audiences.retarget.filter')" data-testid="retarget-filter"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem v-for="f in RETARGET_FILTERS" :key="f" :value="f">{{ t(`wl.audiences.retarget.filters.${f}`) }}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div v-if="needsDays" class="space-y-1">
                <Label for="wl-aud-rt-days">{{ t('wl.audiences.retarget.days') }}</Label>
                <Input id="wl-aud-rt-days" v-model.number="days" type="number" min="1" max="365" placeholder="7" class="w-24" />
              </div>
            </div>
            <div class="flex flex-wrap gap-2">
            <Button
              v-if="canSend"
              :disabled="!store.preview || store.preview.recipients === 0 || store.previewLoading"
              data-testid="retarget-create-campaign"
              @click="sendOpen = true"
            >
              <Send class="h-4 w-4 mr-2" />{{ t('wl.audiences.builder.createCampaign') }}
            </Button>
            <Button v-if="canSaveSegment" variant="outline" data-testid="retarget-save-segment" @click="saveAsSegment">
              <BookmarkPlus class="h-4 w-4 mr-2" />{{ t('wl.audiences.retarget.saveAsSegment') }}
            </Button>
            </div>
          </CardContent>
        </Card>
        <PreviewPanel :preview="store.preview" :loading="store.previewLoading" :error="store.previewError" />
      </div>
    </template>

    <CreateCampaignDialog
      v-if="retarget"
      v-model:open="sendOpen"
      :retarget="retarget"
      :default-name="chosen ? `${chosen.name} – ${t(`wl.audiences.retarget.filters.${filter}`)}` : ''"
      @created="store.runPreview({ retarget: retarget! })"
    />
  </AudiencesShell>
</template>
