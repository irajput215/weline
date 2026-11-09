<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MapPin, Plus } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import { EmptyState } from '@/components/ui/empty-state'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import PracticeShell from '../components/PracticeShell.vue'
import { usePracticeStore } from '../stores/practice'
import type { Location } from '../api'

const { t } = useI18n()
const store = usePracticeStore()
const auth = useAuthStore()
const canCreate = computed(() => auth.hasPermission('wl_practice_locations', 'create'))
const canUpdate = computed(() => auth.hasPermission('wl_practice_locations', 'update'))

const open = ref(false)
const editing = ref<string | null>(null)
const form = reactive({ name: '', address: '', phone: '' })

function edit(l: Location | null) {
  editing.value = l?.id ?? null
  Object.assign(form, { name: l?.name ?? '', address: l?.address ?? '', phone: l?.phone ?? '' })
  store.saveError = null
  open.value = true
}

async function submit() {
  if (await store.saveLocation(editing.value, { ...form })) open.value = false
}

const toggle = (l: Location, active: boolean) => store.saveLocation(l.id, { is_active: active })

onMounted(store.loadLocations)
</script>

<template>
  <PracticeShell :state="store.state.locations" @retry="store.loadLocations">
    <div class="flex justify-end">
      <Button v-if="canCreate" size="sm" data-testid="location-add" @click="edit(null)"><Plus class="h-4 w-4 mr-2" />{{ t('wl.practice.locations.add') }}</Button>
    </div>
    <EmptyState
      v-if="store.locations.length === 0"
      data-testid="practice-empty"
      :icon="MapPin"
      :title="t('wl.practice.locations.emptyTitle')"
      :description="t('wl.practice.locations.emptyDescription')"
    />
    <Card v-else>
      <CardContent class="p-0">
        <Table data-testid="locations-table">
          <TableHeader>
            <TableRow>
              <TableHead>{{ t('wl.practice.fields.name') }}</TableHead>
              <TableHead>{{ t('wl.practice.fields.address') }}</TableHead>
              <TableHead>{{ t('wl.practice.fields.phone') }}</TableHead>
              <TableHead>{{ t('wl.practice.fields.active') }}</TableHead>
              <TableHead v-if="canUpdate" />
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="l in store.locations" :key="l.id">
              <TableCell class="font-medium">{{ l.name }}</TableCell>
              <TableCell>{{ l.address }}</TableCell>
              <TableCell>{{ l.phone }}</TableCell>
              <TableCell>
                <Switch v-if="canUpdate" :checked="l.is_active" @update:checked="v => toggle(l, v)" />
                <Badge v-else :variant="l.is_active ? 'default' : 'secondary'">{{ l.is_active ? t('wl.practice.active') : t('wl.practice.inactive') }}</Badge>
              </TableCell>
              <TableCell v-if="canUpdate" class="text-right">
                <Button variant="ghost" size="sm" @click="edit(l)">{{ t('wl.practice.edit') }}</Button>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>

    <Dialog v-model:open="open">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{{ editing ? t('wl.practice.locations.edit') : t('wl.practice.locations.add') }}</DialogTitle>
        </DialogHeader>
        <form class="grid gap-4" @submit.prevent="submit">
          <div class="grid gap-1.5">
            <Label for="l-name">{{ t('wl.practice.fields.name') }}</Label>
            <Input id="l-name" v-model="form.name" maxlength="120" data-testid="location-name" />
          </div>
          <div class="grid gap-1.5">
            <Label for="l-address">{{ t('wl.practice.fields.address') }}</Label>
            <Input id="l-address" v-model="form.address" maxlength="255" />
          </div>
          <div class="grid gap-1.5">
            <Label for="l-phone">{{ t('wl.practice.fields.phone') }}</Label>
            <Input id="l-phone" v-model="form.phone" maxlength="40" />
          </div>
          <p v-if="store.saveError" class="text-sm text-destructive" role="alert">{{ store.saveError === 'forbidden' ? t('wl.practice.saveForbidden') : store.saveError }}</p>
          <DialogFooter>
            <Button type="submit" :disabled="store.saving" data-testid="location-save">{{ t('wl.practice.save') }}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  </PracticeShell>
</template>
