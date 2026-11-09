<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { ClipboardList, Plus } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import { EmptyState } from '@/components/ui/empty-state'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import PracticeShell from '../components/PracticeShell.vue'
import { centsToDollars, dollarsToCents, durationOptions, usePracticeStore } from '../stores/practice'
import type { Service } from '../api'

const { t } = useI18n()
const store = usePracticeStore()
const auth = useAuthStore()
const canCreate = computed(() => auth.hasPermission('wl_practice_services', 'create'))
const canUpdate = computed(() => auth.hasPermission('wl_practice_services', 'update'))

const slot = computed(() => store.profile?.slot_minutes ?? 15)
const durations = computed(() => durationOptions(slot.value))
const knownRoles = computed(() => [...new Set(store.practitioners.map(p => p.role))].sort())

const open = ref(false)
const editing = ref<string | null>(null)
const form = reactive({ name: '', duration_minutes: 15, price: '', roles: '' })
const formError = ref<string | null>(null)

function edit(s: Service | null) {
  editing.value = s?.id ?? null
  Object.assign(form, {
    name: s?.name ?? '',
    duration_minutes: s?.duration_minutes ?? slot.value,
    price: centsToDollars(s?.price_cents ?? null),
    roles: (s?.allowed_roles ?? []).join(', ')
  })
  formError.value = null
  store.saveError = null
  open.value = true
}

async function submit() {
  const price = dollarsToCents(form.price)
  if (Number.isNaN(price)) {
    formError.value = t('wl.practice.services.badPrice')
    return
  }
  formError.value = null
  const body = {
    name: form.name,
    duration_minutes: Number(form.duration_minutes),
    price_cents: price,
    allowed_roles: form.roles.split(',').map(r => r.trim()).filter(Boolean)
  }
  const warnings = await store.saveService(editing.value, body)
  if (warnings === null) return
  warnings.forEach(w => toast.warning(w))
  open.value = false
}

const toggle = (s: Service, active: boolean) => store.saveService(s.id, { is_active: active })
const price = (cents: number | null) => (cents === null ? '—' : `$${centsToDollars(cents)}`)

async function load() {
  await Promise.all([
    store.loadServices(),
    store.profile ? Promise.resolve() : store.loadProfile(),
    store.practitioners.length ? Promise.resolve() : store.loadPractitioners()
  ])
}
onMounted(load)
</script>

<template>
  <PracticeShell :state="store.state.services" @retry="load">
    <div class="flex justify-end">
      <Button v-if="canCreate" size="sm" :disabled="!store.profile" data-testid="service-add" @click="edit(null)"><Plus class="h-4 w-4 mr-2" />{{ t('wl.practice.services.add') }}</Button>
    </div>
    <p v-if="canCreate && !store.profile" class="text-sm text-muted-foreground">{{ t('wl.practice.needsProfile') }}</p>
    <EmptyState
      v-if="store.services.length === 0"
      data-testid="practice-empty"
      :icon="ClipboardList"
      :title="t('wl.practice.services.emptyTitle')"
      :description="t('wl.practice.services.emptyDescription')"
    />
    <Card v-else>
      <CardContent class="p-0">
        <Table data-testid="services-table">
          <TableHeader>
            <TableRow>
              <TableHead>{{ t('wl.practice.fields.name') }}</TableHead>
              <TableHead>{{ t('wl.practice.fields.duration') }}</TableHead>
              <TableHead>{{ t('wl.practice.fields.price') }}</TableHead>
              <TableHead>{{ t('wl.practice.fields.roles') }}</TableHead>
              <TableHead>{{ t('wl.practice.fields.active') }}</TableHead>
              <TableHead v-if="canUpdate" />
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="s in store.services" :key="s.id">
              <TableCell class="font-medium">{{ s.name }}</TableCell>
              <TableCell>{{ t('wl.practice.minutes', { n: s.duration_minutes }) }}</TableCell>
              <TableCell>{{ price(s.price_cents) }}</TableCell>
              <TableCell>{{ s.allowed_roles.join(', ') }}</TableCell>
              <TableCell>
                <Switch v-if="canUpdate" :checked="s.is_active" @update:checked="v => toggle(s, v)" />
                <Badge v-else :variant="s.is_active ? 'default' : 'secondary'">{{ s.is_active ? t('wl.practice.active') : t('wl.practice.inactive') }}</Badge>
              </TableCell>
              <TableCell v-if="canUpdate" class="text-right">
                <Button variant="ghost" size="sm" @click="edit(s)">{{ t('wl.practice.edit') }}</Button>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>

    <Dialog v-model:open="open">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{{ editing ? t('wl.practice.services.edit') : t('wl.practice.services.add') }}</DialogTitle>
        </DialogHeader>
        <form class="grid gap-4" @submit.prevent="submit">
          <div class="grid gap-1.5">
            <Label for="s-name">{{ t('wl.practice.fields.name') }}</Label>
            <Input id="s-name" v-model="form.name" maxlength="120" data-testid="service-name" />
          </div>
          <div class="grid gap-1.5">
            <Label>{{ t('wl.practice.fields.duration') }}</Label>
            <Select :model-value="String(form.duration_minutes)" @update:model-value="v => (form.duration_minutes = Number(v))">
              <SelectTrigger data-testid="service-duration"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem v-for="d in durations" :key="d" :value="String(d)">{{ t('wl.practice.minutes', { n: d }) }}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div class="grid gap-1.5">
            <Label for="s-price">{{ t('wl.practice.services.priceLabel') }}</Label>
            <Input id="s-price" v-model="form.price" inputmode="decimal" placeholder="85.00" data-testid="service-price" />
          </div>
          <div class="grid gap-1.5">
            <Label for="s-roles">{{ t('wl.practice.fields.roles') }}</Label>
            <Input id="s-roles" v-model="form.roles" :placeholder="knownRoles.join(', ') || 'GP, Nurse'" data-testid="service-roles" />
            <p class="text-xs text-muted-foreground">{{ t('wl.practice.services.rolesHint') }}</p>
          </div>
          <p v-if="formError || store.saveError" class="text-sm text-destructive" role="alert">
            {{ formError || (store.saveError === 'forbidden' ? t('wl.practice.saveForbidden') : store.saveError) }}
          </p>
          <DialogFooter>
            <Button type="submit" :disabled="store.saving" data-testid="service-save">{{ t('wl.practice.save') }}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  </PracticeShell>
</template>
