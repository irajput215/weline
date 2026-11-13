<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { CHANNELS, type Channel, type EntryPoint } from '../api'
import { hasCode, lastNumber, suggestCode, useAdsStore, waLink } from '../stores/ads'
import SaveError from './SaveError.vue'

// New or edit entry point (requirements R1, R3). The code is set on create only.
const props = defineProps<{ entry?: EntryPoint | null; campaigns?: string[]; takenCodes?: string[] }>()
const emit = defineEmits<{ saved: [entry: EntryPoint] }>()
const open = defineModel<boolean>('open', { default: false })

const { t } = useI18n()
const store = useAdsStore()
const editing = computed(() => !!props.entry)

const form = reactive({
  name: '',
  code: '',
  channel: 'meta_ad' as Channel,
  campaign: '',
  wa_number: '',
  prefill_text: '',
  tag: '',
  tag_campaign: true
})
const codeTouched = ref(false)
const textTouched = ref(false)
const formError = ref('')

function defaultText(code: string) {
  return t('wl.ads.form.defaultText', { code })
}

watch(open, o => {
  if (!o) return
  store.saveError = null
  formError.value = ''
  const e = props.entry
  codeTouched.value = !!e
  textTouched.value = !!e
  Object.assign(form, e
    ? { name: e.name, code: e.code, channel: e.channel, campaign: e.campaign, wa_number: e.wa_number,
        prefill_text: e.prefill_text, tag: e.tag, tag_campaign: e.tag_campaign }
    : { name: '', code: '', channel: 'meta_ad', campaign: '', wa_number: lastNumber(), prefill_text: '', tag: '', tag_campaign: true })
})

// Suggest a code from the name until the code is typed by hand, and keep the
// default message in step with it until the message is edited.
watch(() => form.name, name => {
  if (editing.value || codeTouched.value) return
  form.code = suggestCode(name, props.takenCodes ?? [])
})
watch(() => form.code, (code, old) => {
  if (editing.value) return
  if (!textTouched.value || form.prefill_text === defaultText(old ?? '')) form.prefill_text = code ? defaultText(code) : ''
})

const code = computed(() => form.code.trim().toUpperCase())
const codeOk = computed(() => /^[A-Z0-9]{3,12}$/.test(code.value) && /[A-Z]/.test(code.value) && /\d/.test(code.value))
const preview = computed(() => (form.wa_number && form.prefill_text ? waLink(form.wa_number, form.prefill_text) : ''))
const codeError = computed(() => (store.saveStatus === 409 ? store.saveError : ''))
const defaultTag = computed(() => (code.value ? `from-${code.value.toLowerCase()}` : ''))

function addCode() {
  form.prefill_text = `${form.prefill_text.trim()} [${code.value}]`.trim()
  textTouched.value = true
}

async function submit() {
  formError.value = ''
  if (!editing.value && !codeOk.value) return (formError.value = t('wl.ads.form.codeRule'))
  if (!hasCode(form.prefill_text, code.value)) return (formError.value = t('wl.ads.form.needsCode', { code: code.value }))
  const body = { ...form, code: editing.value ? undefined : code.value, tag: form.tag.trim() }
  const saved = props.entry ? await store.update(props.entry.id, body) : await store.create(body)
  if (saved) {
    open.value = false
    emit('saved', saved)
  }
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent class="max-w-xl" data-testid="ads-entry-dialog">
      <DialogHeader>
        <DialogTitle>{{ editing ? t('wl.ads.form.editTitle') : t('wl.ads.form.newTitle') }}</DialogTitle>
        <DialogDescription>{{ t('wl.ads.form.description') }}</DialogDescription>
      </DialogHeader>
      <form class="space-y-4" @submit.prevent="submit">
        <div class="space-y-1">
          <Label for="wl-ads-name">{{ t('wl.ads.form.name') }}</Label>
          <Input id="wl-ads-name" v-model="form.name" maxlength="100" :placeholder="t('wl.ads.form.namePlaceholder')" data-testid="ads-name" />
        </div>
        <div class="grid gap-4 sm:grid-cols-2">
          <div class="space-y-1">
            <Label for="wl-ads-code">{{ t('wl.ads.form.code') }}</Label>
            <Input
              id="wl-ads-code"
              v-model="form.code"
              maxlength="12"
              class="uppercase"
              :disabled="editing"
              data-testid="ads-code"
              @input="codeTouched = true"
            />
            <p v-if="codeError" class="text-xs text-destructive" data-testid="ads-code-error">{{ codeError }}</p>
            <p v-else class="text-xs text-muted-foreground">{{ editing ? t('wl.ads.form.codeFixed') : t('wl.ads.form.codeRule') }}</p>
          </div>
          <div class="space-y-1">
            <Label>{{ t('wl.ads.form.channel') }}</Label>
            <Select v-model="form.channel">
              <SelectTrigger :aria-label="t('wl.ads.form.channel')" data-testid="ads-channel"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem v-for="c in CHANNELS" :key="c" :value="c">{{ t(`wl.ads.channels.${c}`) }}</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
        <div class="grid gap-4 sm:grid-cols-2">
          <div class="space-y-1">
            <Label for="wl-ads-campaign">{{ t('wl.ads.form.campaign') }}</Label>
            <Input id="wl-ads-campaign" v-model="form.campaign" maxlength="60" list="wl-ads-campaigns" data-testid="ads-campaign" />
            <datalist id="wl-ads-campaigns">
              <option v-for="c in campaigns ?? []" :key="c" :value="c" />
            </datalist>
          </div>
          <div class="space-y-1">
            <Label for="wl-ads-number">{{ t('wl.ads.form.number') }}</Label>
            <Input id="wl-ads-number" v-model="form.wa_number" inputmode="tel" placeholder="61 400 000 000" data-testid="ads-number" />
            <p class="text-xs text-muted-foreground">{{ t('wl.ads.form.numberHelp') }}</p>
          </div>
        </div>
        <div class="space-y-1">
          <div class="flex items-center justify-between">
            <Label for="wl-ads-text">{{ t('wl.ads.form.text') }}</Label>
            <Button v-if="code && !hasCode(form.prefill_text, code)" type="button" variant="ghost" size="sm" @click="addCode">
              {{ t('wl.ads.form.addCode', { code }) }}
            </Button>
          </div>
          <Textarea id="wl-ads-text" v-model="form.prefill_text" maxlength="500" :rows="2" data-testid="ads-text" @input="textTouched = true" />
          <p v-if="preview" class="text-xs text-muted-foreground break-all" data-testid="ads-link-preview">{{ preview }}</p>
        </div>
        <div class="grid gap-4 sm:grid-cols-2 items-end">
          <div class="space-y-1">
            <Label for="wl-ads-tag">{{ t('wl.ads.form.tag') }}</Label>
            <Input id="wl-ads-tag" v-model="form.tag" maxlength="50" :placeholder="defaultTag" data-testid="ads-tag" />
          </div>
          <div class="flex items-center gap-2 pb-2">
            <Switch id="wl-ads-tag-campaign" v-model:checked="form.tag_campaign" :disabled="!form.campaign" data-testid="ads-tag-campaign" />
            <Label for="wl-ads-tag-campaign">{{ t('wl.ads.form.tagCampaign') }}</Label>
          </div>
        </div>
        <p v-if="formError" class="text-sm text-destructive" role="alert" data-testid="ads-form-error">{{ formError }}</p>
        <SaveError v-if="!codeError" />
        <DialogFooter>
          <Button type="button" variant="outline" @click="open = false">{{ t('wl.ads.cancel') }}</Button>
          <Button type="submit" :disabled="store.saving" data-testid="ads-save">{{ t('wl.ads.save') }}</Button>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
</template>
