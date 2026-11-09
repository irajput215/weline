<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { RouterLink } from 'vue-router'
import { Plus, Trash2 } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import BookingShell from '../components/BookingShell.vue'
import SaveError from '../components/SaveError.vue'
import { gridTimes, instantOf, overlaps, todayIn, useBookingStore, type LoadState } from '../stores/booking'
import type { Availability } from '../api'

const { t } = useI18n()
const store = useBookingStore()
const auth = useAuthStore()
const canEdit = computed(() => auth.hasPermission('wl_booking_availability', 'update'))

const practitionerId = ref('')
const week = ref<Availability[]>([])
const lastSave = ref<'week' | 'block' | 'settings'>('week')
const tz = computed(() => store.profile?.time_zone ?? 'Australia/Sydney')
const times = computed(() => gridTimes(store.profile?.slot_minutes ?? 15))
const practitioner = computed(() => store.practitioners.find(p => p.id === practitionerId.value))
const myLocations = computed(() => store.locations.filter(l => practitioner.value?.location_ids.includes(l.id)))
const clashes = computed(() => new Set(overlaps(week.value).flat()))

const pageState = computed<LoadState>(() => (store.state.clinic === 'ready' ? (practitionerId.value ? store.state.availability : 'ready') : store.state.clinic))

watch(practitionerId, id => {
  if (!id) return
  store.loadAvailability(id)
  store.loadBlocks(id)
})
watch(() => store.availability, a => (week.value = a.map(x => ({ ...x }))))

function addRow() {
  week.value.push({ location_id: myLocations.value[0]?.id ?? '', weekday: 1, starts_at: '09:00', ends_at: '17:00' })
}
async function saveWeek() {
  lastSave.value = 'week'
  const warnings = await store.saveAvailability(practitionerId.value, week.value)
  if (warnings === null) return
  warnings.forEach(w => toast.warning(w))
  toast.success(t('wl.booking.saved'))
}

// ----- blocks -----
const block = reactive({ day: '', from: '09:00', to: '17:00', reason: '' })
async function addBlock() {
  lastSave.value = 'block'
  const clash = await store.addBlock({
    practitioner_id: practitionerId.value, reason: block.reason,
    starts_at: instantOf(block.day, block.from, tz.value), ends_at: instantOf(block.day, block.to, tz.value)
  })
  if (clash === null) return
  if (clash.length) toast.warning(t('wl.booking.availability.blockClashes', { n: clash.length }))
  Object.assign(block, { reason: '' })
}
const when = (iso: string) => new Intl.DateTimeFormat(undefined, { timeZone: tz.value, dateStyle: 'medium', timeStyle: 'short' }).format(new Date(iso))

// ----- settings -----
const settings = reactive({ notice_hours: 2, window_days: 60 })
watch(() => store.settings, s => {
  if (s) Object.assign(settings, { notice_hours: s.min_notice_minutes / 60, window_days: s.window_days })
}, { immediate: true })
async function saveSettings() {
  lastSave.value = 'settings'
  if (await store.saveSettings({ min_notice_minutes: Math.round(Number(settings.notice_hours) * 60), window_days: Number(settings.window_days) })) {
    toast.success(t('wl.booking.saved'))
  }
}

async function load() {
  await Promise.all([store.loadClinic(), store.loadSettings()])
  block.day = todayIn(tz.value)
  if (!practitionerId.value && store.practitioners.length) practitionerId.value = store.practitioners[0].id
}
onMounted(load)
</script>

<template>
  <BookingShell :state="pageState" @retry="load">
    <Card v-if="store.practitioners.length === 0" data-testid="booking-empty">
      <CardContent class="py-8 text-center text-sm text-muted-foreground">
        {{ t('wl.booking.availability.noPractitioners') }}
        <RouterLink to="/x/practice/practitioners" class="underline">{{ t('wl.booking.availability.addPractitioners') }}</RouterLink>
      </CardContent>
    </Card>

    <template v-else>
      <div class="max-w-xs">
        <Select v-model="practitionerId">
          <SelectTrigger data-testid="availability-practitioner"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem v-for="p in store.practitioners" :key="p.id" :value="p.id">{{ p.display_name }} ({{ p.role }})</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{{ t('wl.booking.availability.title') }}</CardTitle>
          <CardDescription>{{ t('wl.booking.availability.description', { tz }) }}</CardDescription>
        </CardHeader>
        <CardContent class="space-y-3" data-testid="availability-week">
          <p v-if="week.length === 0" class="text-sm text-muted-foreground">{{ t('wl.booking.availability.none') }}</p>
          <div v-for="(row, i) in week" :key="i" class="flex flex-wrap items-center gap-2" :class="{ 'rounded-md ring-1 ring-destructive p-1': clashes.has(i) }">
            <Select :model-value="String(row.weekday)" :disabled="!canEdit" @update:model-value="v => (row.weekday = Number(v))">
              <SelectTrigger class="w-36" :data-testid="`avail-day-${i}`"><SelectValue /></SelectTrigger>
              <SelectContent><SelectItem v-for="d in 7" :key="d" :value="String(d)">{{ t(`wl.booking.weekdays.${d}`) }}</SelectItem></SelectContent>
            </Select>
            <Select v-model="row.starts_at" :disabled="!canEdit">
              <SelectTrigger class="w-24" :data-testid="`avail-from-${i}`"><SelectValue /></SelectTrigger>
              <SelectContent><SelectItem v-for="tm in times" :key="tm" :value="tm">{{ tm }}</SelectItem></SelectContent>
            </Select>
            <span class="text-sm text-muted-foreground">–</span>
            <Select v-model="row.ends_at" :disabled="!canEdit">
              <SelectTrigger class="w-24" :data-testid="`avail-to-${i}`"><SelectValue /></SelectTrigger>
              <SelectContent><SelectItem v-for="tm in times" :key="tm" :value="tm">{{ tm }}</SelectItem></SelectContent>
            </Select>
            <Select v-model="row.location_id" :disabled="!canEdit">
              <SelectTrigger class="w-40" :data-testid="`avail-loc-${i}`"><SelectValue /></SelectTrigger>
              <SelectContent><SelectItem v-for="l in myLocations" :key="l.id" :value="l.id">{{ l.name }}</SelectItem></SelectContent>
            </Select>
            <Button v-if="canEdit" variant="ghost" size="icon" :aria-label="t('wl.booking.remove')" @click="week.splice(i, 1)"><Trash2 class="h-4 w-4" /></Button>
          </div>
          <p v-if="clashes.size" class="text-sm text-destructive" role="alert">{{ t('wl.booking.availability.overlap') }}</p>
          <SaveError v-if="lastSave === 'week'" />
          <div v-if="canEdit" class="flex gap-2">
            <Button variant="outline" size="sm" :disabled="myLocations.length === 0" data-testid="avail-add" @click="addRow"><Plus class="h-4 w-4 mr-2" />{{ t('wl.booking.availability.addBlock') }}</Button>
            <Button size="sm" :disabled="store.saving || clashes.size > 0" data-testid="avail-save" @click="saveWeek">{{ t('wl.booking.save') }}</Button>
          </div>
          <p v-else class="text-sm text-muted-foreground" data-testid="booking-readonly">{{ t('wl.booking.readOnly') }}</p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{{ t('wl.booking.availability.blocksTitle') }}</CardTitle>
          <CardDescription>{{ t('wl.booking.availability.blocksDescription') }}</CardDescription>
        </CardHeader>
        <CardContent class="space-y-3">
          <p v-if="store.blocks.length === 0" class="text-sm text-muted-foreground">{{ t('wl.booking.availability.noBlocks') }}</p>
          <ul v-else class="divide-y divide-border text-sm" data-testid="blocks-list">
            <li v-for="b in store.blocks" :key="b.id" class="flex items-center justify-between py-2">
              <span><span class="font-medium">{{ b.reason }}</span> · {{ when(b.starts_at) }} – {{ when(b.ends_at) }}</span>
              <Button v-if="canEdit" variant="ghost" size="sm" :aria-label="t('wl.booking.remove')" @click="store.removeBlock(b.id)"><Trash2 class="h-4 w-4" /></Button>
            </li>
          </ul>
          <form v-if="canEdit" class="grid gap-3 md:grid-cols-5 items-end" @submit.prevent="addBlock">
            <div class="grid gap-1.5 md:col-span-2">
              <Label for="b-reason">{{ t('wl.booking.fields.reason') }}</Label>
              <Input id="b-reason" v-model="block.reason" maxlength="120" :placeholder="t('wl.booking.availability.blockPlaceholder')" data-testid="block-reason" />
            </div>
            <div class="grid gap-1.5">
              <Label for="b-day">{{ t('wl.booking.fields.day') }}</Label>
              <Input id="b-day" v-model="block.day" type="date" data-testid="block-day" />
            </div>
            <div class="grid gap-1.5">
              <Label>{{ t('wl.booking.availability.from') }}</Label>
              <Select v-model="block.from">
                <SelectTrigger data-testid="block-from"><SelectValue /></SelectTrigger>
                <SelectContent><SelectItem v-for="tm in times" :key="tm" :value="tm">{{ tm }}</SelectItem></SelectContent>
              </Select>
            </div>
            <div class="grid gap-1.5">
              <Label>{{ t('wl.booking.availability.to') }}</Label>
              <Select v-model="block.to">
                <SelectTrigger data-testid="block-to"><SelectValue /></SelectTrigger>
                <SelectContent><SelectItem v-for="tm in times" :key="tm" :value="tm">{{ tm }}</SelectItem></SelectContent>
              </Select>
            </div>
            <SaveError v-if="lastSave === 'block'" class="md:col-span-5" />
            <Button type="submit" class="md:col-span-5 md:w-fit" :disabled="store.saving || !block.reason.trim()" data-testid="block-add">{{ t('wl.booking.availability.addTimeOff') }}</Button>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{{ t('wl.booking.settings.title') }}</CardTitle>
          <CardDescription>{{ t('wl.booking.settings.description') }}</CardDescription>
        </CardHeader>
        <CardContent>
          <form class="grid gap-3 md:grid-cols-3 items-end" @submit.prevent="saveSettings">
            <div class="grid gap-1.5">
              <Label for="s-notice">{{ t('wl.booking.settings.notice') }}</Label>
              <Input id="s-notice" v-model="settings.notice_hours" type="number" min="0" max="168" step="0.25" :disabled="!canEdit" data-testid="settings-notice" />
            </div>
            <div class="grid gap-1.5">
              <Label for="s-window">{{ t('wl.booking.settings.window') }}</Label>
              <Input id="s-window" v-model="settings.window_days" type="number" min="1" max="365" :disabled="!canEdit" data-testid="settings-window" />
            </div>
            <Button v-if="canEdit" type="submit" class="md:w-fit" :disabled="store.saving" data-testid="settings-save">{{ t('wl.booking.save') }}</Button>
            <SaveError v-if="lastSave === 'settings'" class="md:col-span-3" />
          </form>
        </CardContent>
      </Card>
    </template>
  </BookingShell>
</template>
