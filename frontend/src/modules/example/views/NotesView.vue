<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Lock, StickyNote } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { PageHeader, ErrorState } from '@/components/shared'
import { EmptyState } from '@/components/ui/empty-state'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Skeleton } from '@/components/ui/skeleton'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useExampleNotesStore } from '../stores/notes'

const { t, locale } = useI18n()
const store = useExampleNotesStore()
const auth = useAuthStore()

const draft = ref('')
const canCreate = computed(() => auth.hasPermission('wl_example_notes', 'create'))

function formatDate(iso: string) {
  return new Date(iso).toLocaleString(locale.value, { dateStyle: 'medium', timeStyle: 'short' })
}

async function submit() {
  if (!draft.value.trim()) return
  if (await store.addNote(draft.value)) draft.value = ''
}

onMounted(store.fetchNotes)
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader
      :title="t('wl.example.title')"
      :description="t('wl.example.subtitle')"
      :icon="StickyNote"
      icon-gradient="bg-gradient-to-br from-amber-500 to-orange-600 shadow-amber-500/20"
    />

    <!-- Forbidden: the server refused to show notes for this organization. -->
    <EmptyState
      v-if="store.state === 'forbidden'"
      data-testid="example-forbidden"
      :icon="Lock"
      :title="t('wl.example.forbiddenTitle')"
      :description="t('wl.example.forbiddenDescription')"
      class="flex-1"
    />

    <ErrorState
      v-else-if="store.state === 'error'"
      :title="t('common.loadErrorTitle')"
      :description="t('common.loadErrorDescription')"
      :retry-label="t('common.retryLoad')"
      class="flex-1"
      @retry="store.fetchNotes"
    />

    <ScrollArea v-else class="flex-1">
      <div class="p-6">
        <div class="max-w-3xl mx-auto space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>{{ t('wl.example.addTitle') }}</CardTitle>
            </CardHeader>
            <CardContent>
              <form v-if="canCreate" class="space-y-3" @submit.prevent="submit">
                <Textarea
                  v-model="draft"
                  data-testid="example-note-input"
                  :placeholder="t('wl.example.placeholder')"
                  :maxlength="1000"
                  :rows="3"
                />
                <p v-if="store.saveForbidden" class="text-sm text-destructive" data-testid="example-save-forbidden">
                  {{ t('wl.example.saveForbidden') }}
                </p>
                <p v-else-if="store.saveError" class="text-sm text-destructive">{{ store.saveError }}</p>
                <Button type="submit" size="sm" :disabled="store.saving || !draft.trim()" data-testid="example-note-submit">
                  {{ store.saving ? t('wl.example.saving') : t('wl.example.add') }}
                </Button>
              </form>
              <p v-else class="flex items-center gap-2 text-sm text-muted-foreground" data-testid="example-create-forbidden">
                <Lock class="h-4 w-4" />{{ t('wl.example.createForbidden') }}
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{{ t('wl.example.listTitle') }}</CardTitle>
            </CardHeader>
            <CardContent>
              <div v-if="store.state === 'loading' || store.state === 'idle'" class="space-y-3">
                <Skeleton v-for="i in 3" :key="i" class="h-12 w-full" />
              </div>
              <EmptyState
                v-else-if="store.notes.length === 0"
                data-testid="example-empty"
                :icon="StickyNote"
                :title="t('wl.example.emptyTitle')"
                :description="t('wl.example.emptyDescription')"
              />
              <ul v-else class="divide-y divide-border" data-testid="example-notes">
                <li v-for="note in store.notes" :key="note.id" class="py-3">
                  <p class="whitespace-pre-wrap text-sm">{{ note.text }}</p>
                  <p class="mt-1 text-xs text-muted-foreground">{{ formatDate(note.created_at) }}</p>
                </li>
              </ul>
            </CardContent>
          </Card>
        </div>
      </div>
    </ScrollArea>
  </div>
</template>
