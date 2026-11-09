<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { CalendarX, ChevronLeft, ChevronRight, History, Plus } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { EmptyState } from '@/components/ui/empty-state'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import BookingShell from '../components/BookingShell.vue'
import NewAppointmentDialog from '../components/NewAppointmentDialog.vue'
import SaveError from '../components/SaveError.vue'
import { addDays, statusTone, todayIn, useBookingStore, type LoadState } from '../stores/booking'
import type { Appointment, HistoryEntry, Status } from '../api'

const ALL = 'all'
const { t } = useI18n()
const store = useBookingStore()
const auth = useAuthStore()
const canCreate = computed(() => auth.hasPermission('wl_booking_appointments', 'create'))
const canUpdate = computed(() => auth.hasPermission('wl_booking_appointments', 'update'))

const tz = computed(() => store.profile?.time_zone ?? 'Australia/Sydney')
const day = ref('')
const practitioner = ref(ALL)
const creating = ref(false)

// The page is usable once the clinic has loaded; without a practice profile it
// shows a pointer to Practice instead of the list.
const pageState = computed<LoadState>(() => {
  if (store.state.clinic !== 'ready') return store.state.clinic
  return store.profile ? store.state.appointments : 'ready'
})

function loadDay() {
  if (!day.value) return
  store.loadAppointments({ from: day.value, practitioner_id: practitioner.value === ALL ? undefined : practitioner.value })
}
watch([day, practitioner], loadDay)

async function load() {
  await store.loadClinic()
  if (!day.value) day.value = todayIn(tz.value)
  else loadDay()
}
onMounted(load)

// ----- status actions -----
const nextActions: Status[] = ['CONFIRMED', 'ARRIVED', 'COMPLETED', 'NO_SHOW']
const cancelling = reactive({ open: false, appt: null as Appointment | null, reason: '' })

function startCancel(a: Appointment) {
  store.saveError = null
  Object.assign(cancelling, { open: true, appt: a, reason: '' })
}
async function confirmCancel() {
  if (cancelling.appt && (await store.setStatus(cancelling.appt.id, 'CANCELLED', cancelling.reason))) cancelling.open = false
}

// ----- move -----
const moving = reactive({ open: false, appt: null as Appointment | null, day: '', startsAt: '' })

function startMove(a: Appointment) {
  store.saveError = null
  Object.assign(moving, { open: true, appt: a, day: a.local.slice(0, 10), startsAt: '' })
  loadMoveSlots()
}
function loadMoveSlots() {
  if (!moving.appt) return
  moving.startsAt = ''
  store.searchSlots({ service_id: moving.appt.service.id, location_id: moving.appt.location.id,
    practitioner_id: moving.appt.practitioner.id, from: moving.day, to: moving.day })
}
async function confirmMove() {
  if (moving.appt && moving.startsAt && (await store.move(moving.appt.id, { starts_at: moving.startsAt }))) {
    moving.open = false
    loadDay()
  }
}

// ----- history -----
const historyOf = reactive({ open: false, appt: null as Appointment | null, entries: [] as HistoryEntry[] })
async function showHistory(a: Appointment) {
  Object.assign(historyOf, { open: true, appt: a, entries: [] })
  historyOf.entries = await store.history(a.id)
}
function describe(e: HistoryEntry): string {
  if (!e.from_status) return t('wl.booking.history.created', { source: t(`wl.booking.sources.${e.source}`) })
  if (e.from_status !== e.to_status) return t('wl.booking.history.status', { from: t(`wl.booking.statuses.${e.from_status}`), to: t(`wl.booking.statuses.${e.to_status}`) })
  return t('wl.booking.history.changed', { what: Object.keys(e.changes).map(k => t(`wl.booking.history.fields.${k}`)).join(', ') })
}
const when = (iso: string) => new Intl.DateTimeFormat(undefined, { timeZone: tz.value, dateStyle: 'medium', timeStyle: 'short' }).format(new Date(iso))
</script>

<template>
  <BookingShell :state="pageState" @retry="load">
    <p v-if="!store.profile" class="text-sm text-muted-foreground" data-testid="booking-needs-practice">{{ t('wl.booking.needsPractice') }}</p>
    <template v-else>
      <div class="flex flex-wrap items-end gap-3">
        <div class="flex items-end gap-1">
          <Button variant="outline" size="icon" :aria-label="t('wl.booking.previousDay')" @click="day = addDays(day, -1)"><ChevronLeft class="h-4 w-4" /></Button>
          <div class="grid gap-1.5">
            <Label for="appt-day" class="sr-only">{{ t('wl.booking.fields.day') }}</Label>
            <Input id="appt-day" v-model="day" type="date" class="w-40" data-testid="appointments-day" />
          </div>
          <Button variant="outline" size="icon" :aria-label="t('wl.booking.nextDay')" @click="day = addDays(day, 1)"><ChevronRight class="h-4 w-4" /></Button>
          <Button variant="ghost" size="sm" @click="day = todayIn(tz)">{{ t('wl.booking.today') }}</Button>
        </div>
        <Select v-model="practitioner">
          <SelectTrigger class="w-56" data-testid="appointments-practitioner"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem :value="ALL">{{ t('wl.booking.allPractitioners') }}</SelectItem>
            <SelectItem v-for="p in store.practitioners" :key="p.id" :value="p.id">{{ p.display_name }}</SelectItem>
          </SelectContent>
        </Select>
        <div class="flex-1" />
        <Button v-if="canCreate" size="sm" data-testid="appointment-new" @click="creating = true"><Plus class="h-4 w-4 mr-2" />{{ t('wl.booking.new.title') }}</Button>
      </div>

      <EmptyState
        v-if="store.appointments.length === 0"
        data-testid="booking-empty"
        :icon="CalendarX"
        :title="t('wl.booking.appointments.emptyTitle')"
        :description="t('wl.booking.appointments.emptyDescription')"
      />
      <Card v-else>
        <CardContent class="p-0">
          <Table data-testid="appointments-table">
            <TableHeader>
              <TableRow>
                <TableHead>{{ t('wl.booking.fields.time') }}</TableHead>
                <TableHead>{{ t('wl.booking.fields.patient') }}</TableHead>
                <TableHead>{{ t('wl.booking.fields.service') }}</TableHead>
                <TableHead>{{ t('wl.booking.fields.practitioner') }}</TableHead>
                <TableHead>{{ t('wl.booking.fields.status') }}</TableHead>
                <TableHead class="text-right" />
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="a in store.appointments" :key="a.id" :data-testid="`appointment-${a.reference}`">
                <TableCell class="whitespace-nowrap font-medium">
                  {{ a.local.slice(11) }}
                  <div class="text-xs text-muted-foreground">{{ t('wl.booking.minutes', { n: a.duration_minutes }) }} · {{ a.location.name }}</div>
                </TableCell>
                <TableCell>
                  {{ a.contact.name || t('wl.booking.noName') }}
                  <div class="text-xs text-muted-foreground">{{ a.contact.phone }}</div>
                </TableCell>
                <TableCell>
                  {{ a.service.name }}
                  <div v-if="a.note" class="text-xs text-muted-foreground">{{ a.note }}</div>
                </TableCell>
                <TableCell>{{ a.practitioner.name }}</TableCell>
                <TableCell>
                  <Badge :variant="statusTone[a.status]" data-testid="appointment-status">{{ t(`wl.booking.statuses.${a.status}`) }}</Badge>
                  <div class="text-xs text-muted-foreground mt-1">{{ t(`wl.booking.sources.${a.source}`) }} · {{ a.reference }}<span v-if="a.outside_hours"> · {{ t('wl.booking.outsideHoursShort') }}</span></div>
                </TableCell>
                <TableCell class="text-right">
                  <div class="flex flex-wrap justify-end gap-1">
                    <template v-if="canUpdate">
                      <Button
                        v-for="s in nextActions.filter(n => a.allowed_next.includes(n))"
                        :key="s"
                        variant="outline"
                        size="sm"
                        :disabled="store.saving"
                        :data-testid="`action-${s}`"
                        @click="store.setStatus(a.id, s)"
                      >
                        {{ t(`wl.booking.actions.${s}`) }}
                      </Button>
                      <Button v-if="a.status === 'BOOKED' || a.status === 'CONFIRMED'" variant="outline" size="sm" data-testid="action-move" @click="startMove(a)">{{ t('wl.booking.actions.move') }}</Button>
                      <Button v-if="a.allowed_next.includes('CANCELLED')" variant="ghost" size="sm" class="text-destructive" data-testid="action-CANCELLED" @click="startCancel(a)">{{ t('wl.booking.actions.CANCELLED') }}</Button>
                    </template>
                    <Button variant="ghost" size="icon" :aria-label="t('wl.booking.history.title')" data-testid="action-history" @click="showHistory(a)"><History class="h-4 w-4" /></Button>
                  </div>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <SaveError v-if="!cancelling.open && !moving.open && !creating" />
      <p v-if="!canUpdate" class="text-sm text-muted-foreground" data-testid="booking-readonly">{{ t('wl.booking.readOnly') }}</p>
    </template>

    <NewAppointmentDialog v-model:open="creating" :day="day" @booked="loadDay" />

    <Dialog v-model:open="cancelling.open">
      <DialogContent>
        <DialogHeader><DialogTitle>{{ t('wl.booking.cancelTitle', { ref: cancelling.appt?.reference ?? '' }) }}</DialogTitle></DialogHeader>
        <form class="grid gap-3" @submit.prevent="confirmCancel">
          <Label for="cancel-reason">{{ t('wl.booking.fields.reason') }}</Label>
          <Input id="cancel-reason" v-model="cancelling.reason" maxlength="200" data-testid="cancel-reason" />
          <SaveError />
          <DialogFooter>
            <Button type="submit" variant="destructive" :disabled="store.saving || !cancelling.reason.trim()" data-testid="cancel-confirm">{{ t('wl.booking.actions.CANCELLED') }}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>

    <Dialog v-model:open="moving.open">
      <DialogContent>
        <DialogHeader><DialogTitle>{{ t('wl.booking.moveTitle', { ref: moving.appt?.reference ?? '' }) }}</DialogTitle></DialogHeader>
        <div class="grid gap-3">
          <Input v-model="moving.day" type="date" data-testid="move-day" @change="loadMoveSlots" />
          <p v-if="store.slotsLoading" class="text-sm text-muted-foreground">{{ t('wl.booking.new.loadingTimes') }}</p>
          <p v-else-if="store.slots.length === 0" class="text-sm text-muted-foreground">{{ t('wl.booking.new.noTimes') }}</p>
          <div v-else class="flex flex-wrap gap-2" data-testid="move-times">
            <Button
              v-for="s in store.slots"
              :key="s.starts_at"
              size="sm"
              :variant="moving.startsAt === s.starts_at ? 'default' : 'outline'"
              :aria-pressed="moving.startsAt === s.starts_at"
              @click="moving.startsAt = s.starts_at"
            >
              {{ s.local.slice(11) }}
            </Button>
          </div>
          <SaveError />
        </div>
        <DialogFooter>
          <Button :disabled="store.saving || !moving.startsAt" data-testid="move-confirm" @click="confirmMove">{{ t('wl.booking.actions.move') }}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <Dialog v-model:open="historyOf.open">
      <DialogContent>
        <DialogHeader><DialogTitle>{{ t('wl.booking.history.title') }} · {{ historyOf.appt?.reference }}</DialogTitle></DialogHeader>
        <ol class="space-y-2 text-sm" data-testid="history-list">
          <li v-for="(e, i) in historyOf.entries" :key="i">
            <span class="font-medium">{{ describe(e) }}</span>
            <span v-if="e.reason"> — {{ e.reason }}</span>
            <div class="text-xs text-muted-foreground">{{ when(e.created_at) }} · {{ e.changed_by?.name || t(`wl.booking.sources.${e.source}`) }}</div>
          </li>
        </ol>
      </DialogContent>
    </Dialog>
  </BookingShell>
</template>
