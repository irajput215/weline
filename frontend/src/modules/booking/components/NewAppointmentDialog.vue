<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Checkbox } from '@/components/ui/checkbox'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { bookingApi, type Patient } from '../api'
import { gridTimes, instantOf, useBookingStore } from '../stores/booking'
import SaveError from './SaveError.vue'

const props = defineProps<{ day: string }>()
const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{ booked: [] }>()

const ANYONE = 'anyone'
const { t } = useI18n()
const store = useBookingStore()

const form = reactive({
  service: '', location: '', practitioner: ANYONE, day: props.day, startsAt: '', note: '',
  outsideHours: false, outsideTime: '09:00', allowOverlap: false
})
const patient = ref<Patient | null>(null)
const query = ref('')
const results = ref<Patient[]>([])
const newPatient = reactive({ show: false, name: '', mobile: '' })
const patientError = ref<string | null>(null)

const tz = computed(() => store.profile?.time_zone ?? 'Australia/Sydney')
const times = computed(() => gridTimes(store.profile?.slot_minutes ?? 15).slice(0, -1))
const practitionersHere = computed(() => {
  const svc = store.services.find(s => s.id === form.service)
  const roles = (svc?.allowed_roles ?? []).map(r => r.toLowerCase())
  return store.practitioners.filter(p => p.location_ids.includes(form.location) && roles.includes(p.role.toLowerCase()))
})
const nameOf = (id: string) => store.practitioners.find(p => p.id === id)?.display_name ?? ''
const overlapRefused = computed(() => (store.saveError ?? '').startsWith('the patient already has'))

watch(open, o => {
  if (!o) return
  Object.assign(form, { service: store.services[0]?.id ?? '', location: store.locations[0]?.id ?? '', practitioner: ANYONE,
    day: props.day, startsAt: '', note: '', outsideHours: false, allowOverlap: false })
  patient.value = null
  query.value = ''
  results.value = []
  Object.assign(newPatient, { show: false, name: '', mobile: '' })
  store.saveError = null
  loadSlots()
})
watch(() => [form.service, form.location, form.practitioner, form.day], () => {
  if (form.practitioner !== ANYONE && !practitionersHere.value.some(p => p.id === form.practitioner)) form.practitioner = ANYONE
  loadSlots()
})

function loadSlots() {
  form.startsAt = ''
  if (!form.service || !form.location || !form.day) return
  store.searchSlots({ service_id: form.service, location_id: form.location, from: form.day, to: form.day,
    practitioner_id: form.practitioner === ANYONE ? undefined : form.practitioner })
}

let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(query, q => {
  clearTimeout(searchTimer)
  if (q.trim().length < 3) {
    results.value = []
    return
  }
  searchTimer = setTimeout(async () => {
    try {
      results.value = (await bookingApi.searchPatients(q)).patients
    } catch {
      results.value = []
    }
  }, 250)
})

async function addPatient() {
  patientError.value = null
  try {
    const r = await bookingApi.addPatient(newPatient.name, newPatient.mobile)
    patient.value = r.patient
    newPatient.show = false
    if (r.existing) toast.info(t('wl.booking.new.existingPatient', { name: r.patient.name }))
  } catch (err) {
    patientError.value = (err as { response?: { data?: { message?: string } } })?.response?.data?.message ?? t('wl.booking.saveFailed')
  }
}

const chosenSlot = computed(() => store.slots.find(s => s.starts_at === form.startsAt))
const practitionerForBooking = computed(() => {
  if (form.practitioner !== ANYONE) return form.practitioner
  return chosenSlot.value?.practitioner_ids[0] ?? ''
})
const canBook = computed(() =>
  !!patient.value && !!form.service && !!form.location &&
  (form.outsideHours ? form.practitioner !== ANYONE : !!form.startsAt)
)

async function submit() {
  if (!patient.value) return
  const startsAt = form.outsideHours ? instantOf(form.day, form.outsideTime, tz.value) : form.startsAt
  const a = await store.book({
    contact_id: patient.value.id, service_id: form.service, location_id: form.location,
    practitioner_id: practitionerForBooking.value, starts_at: startsAt, note: form.note,
    outside_hours: form.outsideHours, allow_patient_overlap: form.allowOverlap
  })
  if (!a) {
    if (!form.outsideHours) loadSlots()
    return
  }
  toast.success(t('wl.booking.new.booked', { ref: a.reference }))
  open.value = false
  emit('booked')
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent class="max-w-2xl max-h-[90vh] overflow-y-auto">
      <DialogHeader>
        <DialogTitle>{{ t('wl.booking.new.title') }}</DialogTitle>
      </DialogHeader>
      <form class="grid gap-4" @submit.prevent="submit">
        <div class="grid gap-3 md:grid-cols-2">
          <div class="grid gap-1.5">
            <Label>{{ t('wl.booking.fields.service') }}</Label>
            <Select v-model="form.service">
              <SelectTrigger data-testid="new-service"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem v-for="s in store.services" :key="s.id" :value="s.id">{{ s.name }} · {{ t('wl.booking.minutes', { n: s.duration_minutes }) }}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div class="grid gap-1.5">
            <Label>{{ t('wl.booking.fields.location') }}</Label>
            <Select v-model="form.location">
              <SelectTrigger data-testid="new-location"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem v-for="l in store.locations" :key="l.id" :value="l.id">{{ l.name }}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div class="grid gap-1.5">
            <Label>{{ t('wl.booking.fields.practitioner') }}</Label>
            <Select v-model="form.practitioner">
              <SelectTrigger data-testid="new-practitioner"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem :value="ANYONE">{{ t('wl.booking.anyone') }}</SelectItem>
                <SelectItem v-for="p in practitionersHere" :key="p.id" :value="p.id">{{ p.display_name }} ({{ p.role }})</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div class="grid gap-1.5">
            <Label for="new-day">{{ t('wl.booking.fields.day') }}</Label>
            <Input id="new-day" v-model="form.day" type="date" data-testid="new-day" />
          </div>
        </div>

        <div class="grid gap-2">
          <Label>{{ t('wl.booking.fields.time') }}</Label>
          <template v-if="!form.outsideHours">
            <p v-if="store.slotsLoading" class="text-sm text-muted-foreground">{{ t('wl.booking.new.loadingTimes') }}</p>
            <p v-else-if="store.slotsError" class="text-sm text-destructive" role="alert">{{ store.slotsError === 'error' ? t('wl.booking.saveFailed') : store.slotsError }}</p>
            <p v-else-if="store.slots.length === 0" class="text-sm text-muted-foreground" data-testid="new-no-times">{{ t('wl.booking.new.noTimes') }}</p>
            <div v-else class="flex flex-wrap gap-2" role="group" :aria-label="t('wl.booking.fields.time')" data-testid="new-times">
              <Button
                v-for="s in store.slots"
                :key="s.starts_at"
                type="button"
                size="sm"
                :variant="form.startsAt === s.starts_at ? 'default' : 'outline'"
                :aria-pressed="form.startsAt === s.starts_at"
                :title="s.practitioner_ids.map(nameOf).join(', ')"
                @click="form.startsAt = s.starts_at"
              >
                {{ s.local.slice(11) }}
              </Button>
            </div>
          </template>
          <Select v-else v-model="form.outsideTime">
            <SelectTrigger class="w-32" data-testid="new-outside-time"><SelectValue /></SelectTrigger>
            <SelectContent><SelectItem v-for="tm in times" :key="tm" :value="tm">{{ tm }}</SelectItem></SelectContent>
          </Select>
          <label class="flex items-center gap-2 text-sm">
            <Checkbox :checked="form.outsideHours" data-testid="new-outside-hours" @update:checked="v => (form.outsideHours = v === true)" />
            {{ t('wl.booking.new.outsideHours') }}
          </label>
          <p v-if="form.outsideHours && form.practitioner === ANYONE" class="text-xs text-muted-foreground">{{ t('wl.booking.new.outsideHoursNeedsPractitioner') }}</p>
        </div>

        <div class="grid gap-2">
          <Label for="new-patient">{{ t('wl.booking.fields.patient') }}</Label>
          <div v-if="patient" class="flex items-center justify-between rounded-md border border-border px-3 py-2 text-sm" data-testid="new-patient-chosen">
            <span><span class="font-medium">{{ patient.name || t('wl.booking.noName') }}</span> · {{ patient.phone }}</span>
            <Button type="button" variant="ghost" size="sm" @click="patient = null">{{ t('wl.booking.new.change') }}</Button>
          </div>
          <template v-else-if="!newPatient.show">
            <Input id="new-patient" v-model="query" :placeholder="t('wl.booking.new.searchPatient')" autocomplete="off" data-testid="new-patient-search" />
            <ul v-if="results.length" class="divide-y divide-border rounded-md border border-border text-sm">
              <li v-for="p in results" :key="p.id">
                <button type="button" class="w-full px-3 py-2 text-left hover:bg-muted" @click="patient = p">{{ p.name || t('wl.booking.noName') }} · {{ p.phone }}</button>
              </li>
            </ul>
            <Button type="button" variant="link" class="justify-start px-0" data-testid="new-patient-add" @click="newPatient.show = true">{{ t('wl.booking.new.newPatient') }}</Button>
          </template>
          <div v-else class="grid gap-2 rounded-md border border-border p-3 md:grid-cols-2">
            <Input v-model="newPatient.name" maxlength="120" :placeholder="t('wl.booking.new.patientName')" data-testid="new-patient-name" />
            <Input v-model="newPatient.mobile" inputmode="tel" placeholder="0412 345 678" data-testid="new-patient-mobile" />
            <p v-if="patientError" class="text-sm text-destructive md:col-span-2" role="alert">{{ patientError }}</p>
            <div class="flex gap-2 md:col-span-2">
              <Button type="button" size="sm" data-testid="new-patient-save" @click="addPatient">{{ t('wl.booking.new.addPatient') }}</Button>
              <Button type="button" size="sm" variant="ghost" @click="newPatient.show = false">{{ t('wl.booking.cancel') }}</Button>
            </div>
          </div>
        </div>

        <div class="grid gap-1.5">
          <Label for="new-note">{{ t('wl.booking.fields.note') }}</Label>
          <Textarea id="new-note" v-model="form.note" maxlength="280" :rows="2" :placeholder="t('wl.booking.new.notePlaceholder')" />
          <p class="text-xs text-muted-foreground">{{ t('wl.booking.new.noteHint') }}</p>
        </div>

        <SaveError />
        <label v-if="overlapRefused || form.allowOverlap" class="flex items-center gap-2 text-sm">
          <Checkbox :checked="form.allowOverlap" @update:checked="v => (form.allowOverlap = v === true)" />
          {{ t('wl.booking.new.allowOverlap') }}
        </label>
        <DialogFooter>
          <Button type="submit" :disabled="store.saving || !canBook" data-testid="new-book">{{ t('wl.booking.new.book') }}</Button>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
</template>
