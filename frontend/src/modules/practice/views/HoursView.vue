<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { RouterLink } from 'vue-router'
import { Trash2 } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import PracticeShell from '../components/PracticeShell.vue'
import { timeOptions, usePracticeStore, type LoadState } from '../stores/practice'
import type { Hours } from '../api'

const ALL = 'all'
const { t } = useI18n()
const store = usePracticeStore()
const auth = useAuthStore()
const canEdit = computed(() => auth.hasPermission('wl_practice_settings', 'update'))

const locationId = ref<string>('')
const times = computed(() => timeOptions(store.profile?.slot_minutes ?? 15))
const week = reactive(
  [1, 2, 3, 4, 5, 6, 7].map(weekday => ({ weekday, open: false, opens_at: '09:00', closes_at: '17:00' }))
)
const closure = reactive({ location_id: ALL, starts_on: '', ends_on: '', reason: '' })

// The page is usable once the locations and profile have loaded.
const pageState = computed<LoadState>(() => {
  for (const s of [store.state.locations, store.state.profile]) {
    if (s === 'forbidden' || s === 'error' || s === 'loading' || s === 'idle') return s
  }
  return 'ready'
})

watch(
  () => store.hours,
  hours => {
    for (const day of week) {
      const h = hours.find(x => x.weekday === day.weekday)
      day.open = !!h
      if (h) Object.assign(day, { opens_at: h.opens_at, closes_at: h.closes_at })
    }
  }
)
watch(locationId, id => {
  if (id) store.loadHours(id)
})

async function saveWeek() {
  const hours: Hours[] = week.filter(d => d.open).map(d => ({ weekday: d.weekday, opens_at: d.opens_at, closes_at: d.closes_at }))
  if (await store.saveHours(locationId.value, hours)) toast.success(t('wl.practice.saved'))
}

async function addClosure() {
  const ok = await store.addClosure({
    location_id: closure.location_id === ALL ? null : closure.location_id,
    starts_on: closure.starts_on,
    ends_on: closure.ends_on || closure.starts_on,
    reason: closure.reason
  })
  if (ok) Object.assign(closure, { starts_on: '', ends_on: '', reason: '' })
}

const closureLocation = (id: string | null) =>
  id === null ? t('wl.practice.hours.allLocations') : (store.locations.find(l => l.id === id)?.name ?? '—')

async function load() {
  await Promise.all([store.loadLocations(), store.loadProfile(), store.loadClosures()])
  const first = store.locations.find(l => l.is_active)
  if (first && !locationId.value) locationId.value = first.id
}
onMounted(load)
</script>

<template>
  <PracticeShell :state="pageState" @retry="load">
    <Card v-if="store.locations.length === 0" data-testid="practice-empty">
      <CardContent class="py-8 text-center text-sm text-muted-foreground">
        {{ t('wl.practice.hours.noLocations') }}
        <RouterLink to="/x/practice/locations" class="underline">{{ t('wl.practice.tabs.locations') }}</RouterLink>
      </CardContent>
    </Card>

    <template v-else>
      <Card>
        <CardHeader>
          <CardTitle>{{ t('wl.practice.hours.title') }}</CardTitle>
          <CardDescription>{{ t('wl.practice.hours.description', { tz: store.profile?.time_zone ?? '' }) }}</CardDescription>
        </CardHeader>
        <CardContent class="space-y-4">
          <div class="max-w-xs">
            <Select v-model="locationId">
              <SelectTrigger data-testid="hours-location"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem v-for="l in store.locations" :key="l.id" :value="l.id">{{ l.name }}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <p v-if="store.state.hours === 'forbidden'" class="text-sm text-muted-foreground" data-testid="practice-forbidden">{{ t('wl.practice.forbiddenDescription') }}</p>
          <p v-else-if="!store.profile" class="text-sm text-muted-foreground">{{ t('wl.practice.needsProfile') }}</p>
          <div v-else class="space-y-2" data-testid="hours-week">
            <div v-for="day in week" :key="day.weekday" class="flex flex-wrap items-center gap-3">
              <span class="w-28 text-sm">{{ t(`wl.practice.weekdays.${day.weekday}`) }}</span>
              <Switch :checked="day.open" :disabled="!canEdit" :data-testid="`hours-open-${day.weekday}`" @update:checked="v => (day.open = v)" />
              <template v-if="day.open">
                <Select v-model="day.opens_at" :disabled="!canEdit">
                  <SelectTrigger class="w-28" :data-testid="`hours-from-${day.weekday}`"><SelectValue /></SelectTrigger>
                  <SelectContent><SelectItem v-for="tm in times" :key="tm" :value="tm">{{ tm }}</SelectItem></SelectContent>
                </Select>
                <span class="text-sm text-muted-foreground">–</span>
                <Select v-model="day.closes_at" :disabled="!canEdit">
                  <SelectTrigger class="w-28" :data-testid="`hours-to-${day.weekday}`"><SelectValue /></SelectTrigger>
                  <SelectContent><SelectItem v-for="tm in times" :key="tm" :value="tm">{{ tm }}</SelectItem></SelectContent>
                </Select>
              </template>
              <span v-else class="text-sm text-muted-foreground">{{ t('wl.practice.hours.closed') }}</span>
            </div>
            <p v-if="store.saveError" class="text-sm text-destructive" role="alert">{{ store.saveError === 'forbidden' ? t('wl.practice.saveForbidden') : store.saveError }}</p>
            <Button v-if="canEdit" :disabled="store.saving || !locationId" data-testid="hours-save" @click="saveWeek">{{ t('wl.practice.save') }}</Button>
            <p v-else class="text-sm text-muted-foreground" data-testid="practice-readonly">{{ t('wl.practice.readOnly') }}</p>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{{ t('wl.practice.hours.closuresTitle') }}</CardTitle>
          <CardDescription>{{ t('wl.practice.hours.closuresDescription') }}</CardDescription>
        </CardHeader>
        <CardContent class="space-y-4">
          <p v-if="store.closures.length === 0" class="text-sm text-muted-foreground">{{ t('wl.practice.hours.noClosures') }}</p>
          <ul v-else class="divide-y divide-border" data-testid="closures-list">
            <li v-for="c in store.closures" :key="c.id" class="flex items-center justify-between py-2 text-sm">
              <span>
                <span class="font-medium">{{ c.reason }}</span>
                · {{ c.starts_on === c.ends_on ? c.starts_on : `${c.starts_on} – ${c.ends_on}` }} · {{ closureLocation(c.location_id) }}
              </span>
              <Button v-if="canEdit" variant="ghost" size="sm" :aria-label="t('wl.practice.hours.remove')" @click="store.removeClosure(c.id)"><Trash2 class="h-4 w-4" /></Button>
            </li>
          </ul>
          <form v-if="canEdit" class="grid gap-3 md:grid-cols-5 items-end" @submit.prevent="addClosure">
            <div class="grid gap-1.5 md:col-span-2">
              <Label for="c-reason">{{ t('wl.practice.hours.reason') }}</Label>
              <Input id="c-reason" v-model="closure.reason" maxlength="120" placeholder="Good Friday" data-testid="closure-reason" />
            </div>
            <div class="grid gap-1.5">
              <Label for="c-from">{{ t('wl.practice.hours.from') }}</Label>
              <Input id="c-from" v-model="closure.starts_on" type="date" data-testid="closure-from" />
            </div>
            <div class="grid gap-1.5">
              <Label for="c-to">{{ t('wl.practice.hours.to') }}</Label>
              <Input id="c-to" v-model="closure.ends_on" type="date" />
            </div>
            <div class="grid gap-1.5">
              <Label>{{ t('wl.practice.hours.where') }}</Label>
              <Select v-model="closure.location_id">
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem :value="ALL">{{ t('wl.practice.hours.allLocations') }}</SelectItem>
                  <SelectItem v-for="l in store.locations" :key="l.id" :value="l.id">{{ l.name }}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <Button type="submit" class="md:col-span-5 md:w-fit" :disabled="store.saving" data-testid="closure-add">{{ t('wl.practice.hours.addClosure') }}</Button>
          </form>
        </CardContent>
      </Card>
    </template>
  </PracticeShell>
</template>
