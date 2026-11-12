<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink, useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { Plus, Trash2, UsersRound } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import AudiencesShell from '../components/AudiencesShell.vue'
import SaveError from '../components/SaveError.vue'
import { formatDate, useAudiencesStore } from '../stores/audiences'
import type { Segment } from '../api'

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const store = useAudiencesStore()

const canCreate = computed(() => auth.hasPermission('wl_audiences_segments', 'create'))
const canDelete = computed(() => auth.hasPermission('wl_audiences_segments', 'delete'))
const toDelete = ref<Segment | null>(null)

async function confirmDelete() {
  if (!toDelete.value) return
  if (await store.deleteSegment(toDelete.value.id)) {
    toast.success(t('wl.audiences.deleted'))
    toDelete.value = null
  }
}

onMounted(store.loadSegments)
</script>

<template>
  <AudiencesShell :state="store.state.segments" @retry="store.loadSegments">
    <div class="flex items-center justify-between gap-4">
      <p v-if="!canCreate" class="text-sm text-muted-foreground">{{ t('wl.audiences.readOnly') }}</p>
      <span v-else />
      <Button v-if="canCreate" data-testid="segment-new" @click="router.push('/x/audiences/segments/new')">
        <Plus class="h-4 w-4 mr-2" />{{ t('wl.audiences.segments.new') }}
      </Button>
    </div>

    <EmptyState
      v-if="store.segments.length === 0"
      data-testid="segments-empty"
      :icon="UsersRound"
      :title="t('wl.audiences.segments.emptyTitle')"
      :description="t('wl.audiences.segments.emptyDescription')"
    />
    <Card v-else>
      <CardContent class="p-0">
        <Table data-testid="segments-table">
          <TableHeader>
            <TableRow>
              <TableHead>{{ t('wl.audiences.segments.name') }}</TableHead>
              <TableHead class="text-right">{{ t('wl.audiences.segments.count') }}</TableHead>
              <TableHead>{{ t('wl.audiences.segments.lastUsed') }}</TableHead>
              <TableHead v-if="canDelete" />
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="s in store.segments" :key="s.id" :data-testid="`segment-row-${s.name}`">
              <TableCell>
                <RouterLink :to="`/x/audiences/segments/${s.id}`" class="font-medium hover:underline">{{ s.name }}</RouterLink>
                <p v-if="s.description" class="text-xs text-muted-foreground">{{ s.description }}</p>
              </TableCell>
              <TableCell class="text-right tabular-nums" data-testid="segment-count">{{ s.count ?? '—' }}</TableCell>
              <TableCell>{{ s.last_used_at ? formatDate(s.last_used_at) : t('wl.audiences.never') }}</TableCell>
              <TableCell v-if="canDelete" class="text-right">
                <Button variant="ghost" size="icon" :aria-label="t('wl.audiences.delete')" @click="toDelete = s">
                  <Trash2 class="h-4 w-4" />
                </Button>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>

    <Dialog :open="!!toDelete" @update:open="o => { if (!o) toDelete = null }">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{{ t('wl.audiences.segments.deleteConfirmTitle', { name: toDelete?.name ?? '' }) }}</DialogTitle>
          <DialogDescription>{{ t('wl.audiences.segments.deleteConfirmDescription') }}</DialogDescription>
        </DialogHeader>
        <SaveError />
        <DialogFooter>
          <Button variant="outline" @click="toDelete = null">{{ t('wl.audiences.cancel') }}</Button>
          <Button variant="destructive" :disabled="store.saving" data-testid="segment-delete-confirm" @click="confirmDelete">
            {{ t('wl.audiences.delete') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </AudiencesShell>
</template>
