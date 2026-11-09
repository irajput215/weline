<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plus, Stethoscope } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Checkbox } from '@/components/ui/checkbox'
import { Badge } from '@/components/ui/badge'
import { EmptyState } from '@/components/ui/empty-state'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import PracticeShell from '../components/PracticeShell.vue'
import { usePracticeStore } from '../stores/practice'
import type { Practitioner } from '../api'

const NO_USER = 'none'
const { t } = useI18n()
const store = usePracticeStore()
const auth = useAuthStore()
const canCreate = computed(() => auth.hasPermission('wl_practice_practitioners', 'create'))
const canUpdate = computed(() => auth.hasPermission('wl_practice_practitioners', 'update'))

const open = ref(false)
const editing = ref<string | null>(null)
const form = reactive({ display_name: '', role: '', ahpra_number: '', user_id: NO_USER, location_ids: [] as string[] })

const locationName = (id: string) => store.locations.find(l => l.id === id)?.name ?? '—'
const activeLocations = computed(() => store.locations.filter(l => l.is_active))

function edit(p: Practitioner | null) {
  editing.value = p?.id ?? null
  Object.assign(form, {
    display_name: p?.display_name ?? '',
    role: p?.role ?? '',
    ahpra_number: p?.ahpra_number ?? '',
    user_id: p?.user_id ?? NO_USER,
    location_ids: [...(p?.location_ids ?? [])]
  })
  store.saveError = null
  open.value = true
}

function toggleLocation(id: string, on: boolean) {
  form.location_ids = on ? [...form.location_ids, id] : form.location_ids.filter(x => x !== id)
}

async function submit() {
  const body = { ...form, user_id: form.user_id === NO_USER ? null : form.user_id }
  if (await store.savePractitioner(editing.value, body)) open.value = false
}

const toggle = (p: Practitioner, active: boolean) => store.savePractitioner(p.id, { is_active: active })

async function load() {
  await Promise.all([store.loadPractitioners(), store.locations.length ? Promise.resolve() : store.loadLocations()])
}
onMounted(load)
</script>

<template>
  <PracticeShell :state="store.state.practitioners" @retry="load">
    <div class="flex justify-end">
      <Button v-if="canCreate" size="sm" data-testid="practitioner-add" @click="edit(null)"><Plus class="h-4 w-4 mr-2" />{{ t('wl.practice.practitioners.add') }}</Button>
    </div>
    <EmptyState
      v-if="store.practitioners.length === 0"
      data-testid="practice-empty"
      :icon="Stethoscope"
      :title="t('wl.practice.practitioners.emptyTitle')"
      :description="t('wl.practice.practitioners.emptyDescription')"
    />
    <Card v-else>
      <CardContent class="p-0">
        <Table data-testid="practitioners-table">
          <TableHeader>
            <TableRow>
              <TableHead>{{ t('wl.practice.fields.name') }}</TableHead>
              <TableHead>{{ t('wl.practice.fields.role') }}</TableHead>
              <TableHead>{{ t('wl.practice.fields.locations') }}</TableHead>
              <TableHead>{{ t('wl.practice.fields.login') }}</TableHead>
              <TableHead>{{ t('wl.practice.fields.active') }}</TableHead>
              <TableHead v-if="canUpdate" />
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="p in store.practitioners" :key="p.id">
              <TableCell class="font-medium">{{ p.display_name }}</TableCell>
              <TableCell>{{ p.role }}</TableCell>
              <TableCell>{{ p.location_ids.map(locationName).join(', ') }}</TableCell>
              <TableCell>{{ p.user_name || t('wl.practice.practitioners.noLogin') }}</TableCell>
              <TableCell>
                <Switch v-if="canUpdate" :checked="p.is_active" @update:checked="v => toggle(p, v)" />
                <Badge v-else :variant="p.is_active ? 'default' : 'secondary'">{{ p.is_active ? t('wl.practice.active') : t('wl.practice.inactive') }}</Badge>
              </TableCell>
              <TableCell v-if="canUpdate" class="text-right">
                <Button variant="ghost" size="sm" @click="edit(p)">{{ t('wl.practice.edit') }}</Button>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>

    <Dialog v-model:open="open">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{{ editing ? t('wl.practice.practitioners.edit') : t('wl.practice.practitioners.add') }}</DialogTitle>
        </DialogHeader>
        <form class="grid gap-4" @submit.prevent="submit">
          <div class="grid gap-1.5">
            <Label for="pr-name">{{ t('wl.practice.fields.name') }}</Label>
            <Input id="pr-name" v-model="form.display_name" maxlength="120" data-testid="practitioner-name" />
          </div>
          <div class="grid gap-1.5">
            <Label for="pr-role">{{ t('wl.practice.fields.role') }}</Label>
            <Input id="pr-role" v-model="form.role" maxlength="60" :placeholder="t('wl.practice.practitioners.rolePlaceholder')" data-testid="practitioner-role" />
          </div>
          <div class="grid gap-1.5">
            <Label for="pr-ahpra">{{ t('wl.practice.fields.ahpra') }}</Label>
            <Input id="pr-ahpra" v-model="form.ahpra_number" maxlength="16" placeholder="MED0001234567" />
          </div>
          <div class="grid gap-1.5">
            <Label>{{ t('wl.practice.fields.login') }}</Label>
            <Select v-model="form.user_id">
              <SelectTrigger data-testid="practitioner-user"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem :value="NO_USER">{{ t('wl.practice.practitioners.noLogin') }}</SelectItem>
                <SelectItem v-for="m in store.members" :key="m.id" :value="m.id">{{ m.full_name }} ({{ m.email }})</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <fieldset class="grid gap-2">
            <legend class="text-sm font-medium mb-1">{{ t('wl.practice.fields.locations') }}</legend>
            <p v-if="activeLocations.length === 0" class="text-sm text-muted-foreground">{{ t('wl.practice.practitioners.noLocations') }}</p>
            <label v-for="l in activeLocations" :key="l.id" class="flex items-center gap-2 text-sm">
              <Checkbox :checked="form.location_ids.includes(l.id)" :data-testid="`practitioner-loc-${l.name}`" @update:checked="v => toggleLocation(l.id, v === true)" />
              {{ l.name }}
            </label>
          </fieldset>
          <p v-if="store.saveError" class="text-sm text-destructive" role="alert">{{ store.saveError === 'forbidden' ? t('wl.practice.saveForbidden') : store.saveError }}</p>
          <DialogFooter>
            <Button type="submit" :disabled="store.saving" data-testid="practitioner-save">{{ t('wl.practice.save') }}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  </PracticeShell>
</template>
