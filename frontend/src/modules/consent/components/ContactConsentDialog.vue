<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { useAuthStore } from '@/stores/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { consentApi, type Action, type ContactConsent, type FoundContact } from '../api'
import { formatDate, messageOf, today, useConsentStore } from '../stores/consent'
import SaveError from './SaveError.vue'
import StateBadge from './StateBadge.vue'

const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{ changed: [] }>()

const { t } = useI18n()
const auth = useAuthStore()
const store = useConsentStore()
const canRecord = computed(() => auth.hasPermission('wl_consent_records', 'create'))

const q = ref('')
const results = ref<FoundContact[]>([])
const searching = ref(false)
const searchError = ref<string | null>(null)
const chosen = ref<ContactConsent | null>(null)
const form = reactive<{ action: Action | null; given_at: string; evidence: string }>({ action: null, given_at: today(), evidence: '' })
const formError = ref<string | null>(null)
let seq = 0
let timer: ReturnType<typeof setTimeout> | undefined

watch(open, o => {
  if (!o) return
  q.value = ''
  results.value = []
  chosen.value = null
  form.action = null
  store.saveError = null
})

watch(q, v => {
  clearTimeout(timer)
  searchError.value = null
  if (v.trim().length < 2) {
    results.value = []
    return
  }
  timer = setTimeout(async () => {
    const n = ++seq
    searching.value = true
    try {
      const r = await consentApi.search(v)
      if (n === seq) results.value = r.contacts ?? []
    } catch (err) {
      if (n === seq) searchError.value = messageOf(err) || t('wl.consent.saveFailed')
    } finally {
      if (n === seq) searching.value = false
    }
  }, 300)
})

async function choose(c: FoundContact) {
  try {
    chosen.value = await consentApi.contact(c.id)
  } catch (err) {
    searchError.value = messageOf(err) || t('wl.consent.saveFailed')
  }
}

function start(action: Action) {
  form.action = action
  form.given_at = today()
  form.evidence = ''
  formError.value = null
  store.saveError = null
}

async function submit() {
  if (!chosen.value || !form.action) return
  formError.value = null
  if (form.action === 'opt_in' && !form.evidence.trim()) return (formError.value = t('wl.consent.contact.needsEvidence'))
  const id = chosen.value.contact.id
  const res = await store.save(() => consentApi.record(id, { action: form.action as Action, given_at: form.given_at, evidence: form.evidence.trim() }))
  if (!res) return
  chosen.value = res.contact
  toast.success(res.changed || res.recorded ? t('wl.consent.contact.recorded') : t('wl.consent.contact.unchanged', { state: t(`wl.consent.states.${res.marketing}`) }))
  form.action = null
  emit('changed')
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent class="max-w-2xl max-h-[90vh] overflow-y-auto">
      <DialogHeader>
        <DialogTitle>{{ t('wl.consent.contact.title') }}</DialogTitle>
        <DialogDescription v-if="!canRecord">{{ t('wl.consent.contact.readOnly') }}</DialogDescription>
      </DialogHeader>

      <div v-if="!chosen" class="space-y-3">
        <Input v-model="q" :placeholder="t('wl.consent.contact.search')" :aria-label="t('wl.consent.contact.search')" data-testid="consent-search" autofocus />
        <p v-if="q.trim().length < 2" class="text-xs text-muted-foreground">{{ t('wl.consent.contact.searchHint') }}</p>
        <p v-else-if="searchError" class="text-sm text-destructive" role="alert">{{ searchError }}</p>
        <p v-else-if="!searching && results.length === 0" class="text-sm text-muted-foreground">{{ t('wl.consent.contact.noResults') }}</p>
        <ul class="divide-y divide-border rounded-md border border-border" v-if="results.length">
          <li v-for="c in results" :key="c.id">
            <button
              type="button"
              class="flex w-full items-center justify-between gap-3 px-3 py-2 text-left text-sm hover:bg-muted"
              :data-testid="`consent-result-${c.name}`"
              @click="choose(c)"
            >
              <span><span class="font-medium">{{ c.name || '—' }}</span> <span class="text-muted-foreground tabular-nums">{{ c.phone_masked }}</span></span>
              <StateBadge :state="c.marketing" />
            </button>
          </li>
        </ul>
      </div>

      <div v-else class="space-y-4" data-testid="consent-contact">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div>
            <p class="font-medium">{{ chosen.contact.name || '—' }} <span class="text-muted-foreground tabular-nums">{{ chosen.contact.phone_masked }}</span></p>
            <p class="text-sm text-muted-foreground">
              <StateBadge :state="chosen.marketing" />
              <span v-if="chosen.given_at && chosen.source" class="ml-2">
                {{ t('wl.consent.contact.since', { date: formatDate(chosen.given_at), source: t(`wl.consent.sources.${chosen.source}`) }) }}
              </span>
            </p>
          </div>
          <div v-if="canRecord && !form.action" class="flex gap-2">
            <Button size="sm" variant="outline" data-testid="consent-record-in" @click="start('opt_in')">{{ t('wl.consent.contact.recordIn') }}</Button>
            <Button size="sm" variant="outline" data-testid="consent-record-out" @click="start('opt_out')">{{ t('wl.consent.contact.recordOut') }}</Button>
          </div>
        </div>

        <form v-if="form.action" class="space-y-3 rounded-md border border-border p-3" @submit.prevent="submit">
          <p class="text-sm font-medium">{{ form.action === 'opt_in' ? t('wl.consent.contact.recordIn') : t('wl.consent.contact.recordOut') }}</p>
          <div class="space-y-1">
            <Label for="wl-consent-date">{{ t('wl.consent.contact.date') }}</Label>
            <Input id="wl-consent-date" v-model="form.given_at" type="date" :max="today()" class="w-44" data-testid="consent-date" />
          </div>
          <div class="space-y-1">
            <Label for="wl-consent-evidence">{{ form.action === 'opt_in' ? t('wl.consent.contact.evidenceIn') : t('wl.consent.contact.evidenceOut') }}</Label>
            <Textarea
              id="wl-consent-evidence"
              v-model="form.evidence"
              :rows="2"
              maxlength="500"
              :placeholder="form.action === 'opt_in' ? t('wl.consent.contact.evidenceInPlaceholder') : ''"
              data-testid="consent-evidence"
            />
          </div>
          <SaveError :message="formError" />
          <div class="flex gap-2">
            <Button type="submit" size="sm" :disabled="store.saving" data-testid="consent-record-save">{{ t('wl.consent.save') }}</Button>
            <Button type="button" size="sm" variant="ghost" @click="form.action = null">{{ t('wl.consent.cancel') }}</Button>
          </div>
        </form>

        <div>
          <p class="text-sm font-medium mb-2">{{ t('wl.consent.contact.history') }}</p>
          <p v-if="chosen.events.length === 0" class="text-sm text-muted-foreground">{{ t('wl.consent.contact.noHistory') }}</p>
          <ul v-else class="space-y-2 text-sm" data-testid="consent-contact-history">
            <li v-for="e in chosen.events" :key="e.id" class="rounded-md border border-border p-2">
              <span class="font-medium">{{ t(`wl.consent.actions.${e.action}`) }}</span>
              · {{ t(`wl.consent.sources.${e.source}`) }} · {{ formatDate(e.recorded_at, true) }}
              <span v-if="e.recorded_by"> · {{ e.recorded_by.name }}</span>
              <p v-if="e.keyword" class="text-muted-foreground">{{ t('wl.consent.overview.keyword', { keyword: e.keyword }) }}</p>
              <p v-if="e.evidence" class="text-muted-foreground">{{ e.evidence }}</p>
            </li>
          </ul>
        </div>
        <Button variant="ghost" size="sm" @click="chosen = null">{{ t('wl.consent.contact.search') }}</Button>
      </div>
    </DialogContent>
  </Dialog>
</template>
