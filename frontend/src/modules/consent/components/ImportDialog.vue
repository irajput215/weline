<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { consentApi, type ImportPreview } from '../api'
import { messageOf, useConsentStore } from '../stores/consent'
import SaveError from './SaveError.vue'

const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{ done: [] }>()

const { t } = useI18n()
const store = useConsentStore()
const file = ref<File | null>(null)
const preview = ref<ImportPreview | null>(null)
const checking = ref(false)
const evidence = ref('')
const formError = ref<string | null>(null)

watch(open, o => {
  if (!o) return
  file.value = null
  preview.value = null
  evidence.value = ''
  formError.value = null
  store.saveError = null
})

function pick(e: Event) {
  file.value = (e.target as HTMLInputElement).files?.[0] ?? null
  preview.value = null
  formError.value = null
}

async function check() {
  if (!file.value) return
  checking.value = true
  formError.value = null
  try {
    preview.value = await consentApi.previewImport(file.value)
  } catch (err) {
    formError.value = messageOf(err) || t('wl.consent.saveFailed')
  } finally {
    checking.value = false
  }
}

async function apply() {
  if (!preview.value || !file.value) return
  formError.value = null
  if (!evidence.value.trim()) return (formError.value = t('wl.consent.import.needsEvidence'))
  const p = preview.value
  const res = await store.save(() =>
    consentApi.applyImport({ token: p.token, filename: file.value?.name ?? '', evidence: evidence.value.trim(), confirm_count: p.will_apply })
  )
  if (!res) {
    preview.value = null // counts changed or the preview expired: check again
    return
  }
  toast.success(t('wl.consent.import.done', { n: res.import.applied }))
  open.value = false
  emit('done')
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent class="max-w-xl max-h-[90vh] overflow-y-auto">
      <DialogHeader>
        <DialogTitle>{{ t('wl.consent.import.title') }}</DialogTitle>
        <DialogDescription>{{ t('wl.consent.import.description') }}</DialogDescription>
      </DialogHeader>
      <div class="space-y-4">
        <div class="flex flex-wrap items-end gap-2">
          <div class="space-y-1">
            <Label for="wl-consent-file">{{ t('wl.consent.import.file') }}</Label>
            <Input id="wl-consent-file" type="file" accept=".csv,text/csv" data-testid="consent-import-file" @change="pick" />
          </div>
          <Button variant="outline" :disabled="!file || checking" data-testid="consent-import-check" @click="check">{{ t('wl.consent.import.preview') }}</Button>
        </div>

        <div v-if="preview" class="space-y-2 text-sm" data-testid="consent-import-preview">
          <p class="text-muted-foreground">{{ t('wl.consent.import.rows', { n: preview.rows_total }) }}</p>
          <p class="text-lg font-semibold" data-testid="consent-import-will">{{ t('wl.consent.import.willApply', { n: preview.will_apply }) }}</p>
          <ul class="list-disc pl-5 text-muted-foreground">
            <li v-if="preview.already_opted_in">{{ t('wl.consent.import.alreadyOptedIn', { n: preview.already_opted_in }) }}</li>
            <li v-if="preview.unknown_numbers" data-testid="consent-import-unknown">{{ t('wl.consent.import.unknown', { n: preview.unknown_numbers }) }}</li>
            <li v-if="preview.opted_out_later">{{ t('wl.consent.import.optedOutLater', { n: preview.opted_out_later }) }}</li>
            <li v-if="preview.invalid">{{ t('wl.consent.import.invalid', { n: preview.invalid }) }}</li>
          </ul>
          <ul v-if="preview.invalid_rows.length" class="text-xs text-muted-foreground">
            <li v-for="r in preview.invalid_rows" :key="r.row">{{ t('wl.consent.import.row', { row: r.row, reason: r.reason }) }}</li>
          </ul>
          <p v-if="preview.will_apply === 0" class="text-destructive">{{ t('wl.consent.import.nothing') }}</p>
          <div v-else class="space-y-1">
            <Label for="wl-consent-import-evidence">{{ t('wl.consent.import.evidence') }}</Label>
            <Textarea
              id="wl-consent-import-evidence"
              v-model="evidence"
              :rows="3"
              maxlength="1000"
              :placeholder="t('wl.consent.import.evidencePlaceholder')"
              data-testid="consent-import-evidence"
            />
          </div>
        </div>
        <SaveError :message="formError" />
      </div>
      <DialogFooter>
        <Button variant="outline" @click="open = false">{{ t('wl.consent.cancel') }}</Button>
        <Button :disabled="!preview || preview.will_apply === 0 || store.saving" data-testid="consent-import-confirm" @click="apply">
          {{ t('wl.consent.import.confirm', { n: preview?.will_apply ?? 0 }) }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
