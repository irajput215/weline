<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { audiencesApi, type Preview, type Retarget, type Rules, type VarSource, type VarSourceKind } from '../api'
import { useAudiencesStore } from '../stores/audiences'
import PreviewPanel from './PreviewPanel.vue'
import SaveError from './SaveError.vue'

// Either a saved segment (with its rules, for the count) or a retarget.
const props = defineProps<{ segmentId?: string; rules?: Rules; retarget?: Retarget; defaultName?: string }>()
const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{ created: [campaignId: string] }>()

const { t } = useI18n()
const store = useAudiencesStore()
const router = useRouter()
const SOURCES: VarSourceKind[] = ['name', 'text', 'metadata', 'phone']
const varLabel = (name: string) => '{' + '{' + name + '}' + '}'

const form = reactive({ name: '', account: '', template: '' })
const body = ref<Record<string, VarSource>>({})
const header = ref<Record<string, VarSource>>({})
const formError = ref<string | null>(null)

const preview = ref<Preview | null>(null)
const previewLoading = ref(false)
const previewError = ref<string | null>(null)
let seq = 0

const templates = computed(() => (store.fields?.templates ?? []).filter(tp => tp.whatsapp_account === form.account))
const template = computed(() => templates.value.find(tp => tp.id === form.template) ?? null)
const marketing = computed(() => template.value?.category.toUpperCase() === 'MARKETING')

const blank = (): VarSource => ({ source: 'name', fallback: '' })
function mapFor(names: string[], old: Record<string, VarSource>) {
  return Object.fromEntries(names.map(n => [n, old[n] ?? blank()]))
}

watch(open, async o => {
  if (!o) return
  formError.value = null
  store.saveError = null
  if (!store.fields) await store.loadFields()
  Object.assign(form, { name: props.defaultName ?? '', account: store.fields?.accounts[0] ?? '', template: '' })
  form.template = templates.value[0]?.id ?? ''
})

watch(template, tp => {
  body.value = mapFor(tp?.body_vars ?? [], body.value)
  header.value = mapFor(tp?.header_vars ?? [], header.value)
  void count()
})
watch(() => form.account, () => {
  if (!templates.value.some(tp => tp.id === form.template)) form.template = templates.value[0]?.id ?? ''
})

async function count() {
  const n = ++seq
  if (!open.value) return
  previewLoading.value = true
  previewError.value = null
  try {
    const p = await audiencesApi.preview({
      ...(props.retarget ? { retarget: props.retarget } : { rules: props.rules }),
      template_id: form.template || undefined
    })
    if (n === seq) preview.value = p
  } catch (err) {
    if (n === seq) {
      preview.value = null
      previewError.value = (err as { response?: { data?: { message?: string } } })?.response?.data?.message || 'error'
    }
  } finally {
    if (n === seq) previewLoading.value = false
  }
}

/** The message as the first sample contact would see it. */
const rendered = computed(() => {
  const tp = template.value
  if (!tp) return ''
  const first = preview.value?.sample[0]
  const fill = (text: string, vars: Record<string, VarSource>) =>
    text.replace(/\{\{\s*([^}\s]+)\s*\}\}/g, (_, name: string) => {
      const v = vars[name]
      if (!v) return `{{${name}}}`
      let out = ''
      if (v.source === 'text') out = v.value ?? ''
      else if (v.source === 'name') out = first?.name ?? ''
      else if (v.source === 'phone') out = first?.phone_masked ?? ''
      else out = v.key ? `[${v.key}]` : ''
      return out || v.fallback || `{{${name}}}`
    })
  const head = tp.header_type?.toUpperCase() === 'TEXT' && tp.header_content ? fill(tp.header_content, header.value) + '\n\n' : ''
  return head + fill(tp.body_content, body.value)
})

function varsComplete(vars: Record<string, VarSource>) {
  return Object.values(vars).every(v =>
    v.source === 'metadata' ? /^[a-zA-Z0-9_]{1,50}$/.test(v.key ?? '') : v.source !== 'text' || !!(v.value ?? '').trim() || !!v.fallback
  )
}

const canCreate = computed(
  () => !!template.value && !previewLoading.value && (preview.value?.recipients ?? 0) > 0 && !store.saving
)

async function create() {
  formError.value = null
  if (!form.name.trim()) return (formError.value = t('wl.audiences.send.needsName'))
  if (!varsComplete(body.value) || !varsComplete(header.value)) return (formError.value = t('wl.audiences.send.needsVariables'))
  const res = await store.createSend({
    name: form.name.trim(),
    whatsapp_account: form.account,
    template_id: form.template,
    ...(props.retarget ? { retarget: props.retarget } : { segment_id: props.segmentId }),
    variables: { body: body.value, header: header.value },
    confirm_count: preview.value?.recipients ?? 0
  })
  if (!res) {
    void count() // the audience may have changed (409): show the new count
    return
  }
  toast.success(t('wl.audiences.send.created', { n: res.recipients }), {
    action: { label: t('wl.audiences.send.openCampaign'), onClick: () => router.push(`/campaigns/${res.campaign_id}`) }
  })
  open.value = false
  emit('created', res.campaign_id)
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent class="max-w-3xl max-h-[90vh] overflow-y-auto" data-testid="create-campaign-dialog">
      <DialogHeader>
        <DialogTitle>{{ t('wl.audiences.send.title') }}</DialogTitle>
        <DialogDescription>{{ t('wl.audiences.send.description') }}</DialogDescription>
      </DialogHeader>

      <div class="grid gap-6 md:grid-cols-[1fr_16rem]">
        <div class="space-y-4">
          <div class="space-y-1">
            <Label for="wl-aud-send-name">{{ t('wl.audiences.send.name') }}</Label>
            <Input id="wl-aud-send-name" v-model="form.name" maxlength="255" data-testid="send-name" />
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div class="space-y-1">
              <Label>{{ t('wl.audiences.send.account') }}</Label>
              <Select v-model="form.account">
                <SelectTrigger :aria-label="t('wl.audiences.send.account')" data-testid="send-account"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="a in store.fields?.accounts ?? []" :key="a" :value="a">{{ a }}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div class="space-y-1">
              <Label>{{ t('wl.audiences.send.template') }}</Label>
              <Select v-model="form.template" :disabled="templates.length === 0">
                <SelectTrigger :aria-label="t('wl.audiences.send.template')" data-testid="send-template"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="tp in templates" :key="tp.id" :value="tp.id">{{ tp.name }} ({{ tp.language }})</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <p v-if="templates.length === 0" class="text-sm text-muted-foreground" data-testid="send-no-templates">
            {{ t('wl.audiences.send.noTemplates') }}
          </p>
          <p v-if="marketing" class="text-xs text-muted-foreground">{{ t('wl.audiences.send.marketingNote') }}</p>

          <div v-if="Object.keys(header).length || Object.keys(body).length" class="space-y-3">
            <p class="text-sm font-medium">{{ t('wl.audiences.send.variables') }}</p>
            <template v-for="[part, vars] in ([['header', header], ['body', body]] as const)" :key="part">
              <div v-for="(v, name) in vars" :key="`${part}-${name}`" class="flex flex-wrap items-center gap-2" :data-testid="`var-${part}-${name}`">
                <span class="w-28 text-sm font-mono truncate" :title="t(`wl.audiences.send.${part}`)">{{ varLabel(String(name)) }}</span>
                <Select v-model="v.source">
                  <SelectTrigger class="w-36" :aria-label="t('wl.audiences.send.source')"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem v-for="s in SOURCES" :key="s" :value="s">{{ t(`wl.audiences.send.sources.${s}`) }}</SelectItem>
                  </SelectContent>
                </Select>
                <Input v-if="v.source === 'text'" v-model="v.value" class="w-36" :aria-label="t('wl.audiences.send.text')" />
                <template v-if="v.source === 'metadata'">
                  <Input v-model="v.key" list="wl-aud-send-keys" class="w-36" :aria-label="t('wl.audiences.builder.key')" />
                </template>
                <Input v-model="v.fallback" class="w-32" :placeholder="t('wl.audiences.send.fallback')" :aria-label="t('wl.audiences.send.fallback')" />
              </div>
            </template>
            <datalist id="wl-aud-send-keys">
              <option v-for="k in store.fields?.metadata_keys ?? []" :key="k" :value="k" />
            </datalist>
          </div>

          <div v-if="template" class="space-y-1">
            <p class="text-sm font-medium">{{ t('wl.audiences.send.messagePreview') }}</p>
            <p class="whitespace-pre-wrap rounded-md border border-border bg-muted/40 p-3 text-sm" data-testid="send-message-preview">{{ rendered }}</p>
          </div>
        </div>

        <PreviewPanel :preview="preview" :loading="previewLoading" :error="previewError" />
      </div>

      <p v-if="preview && preview.recipients === 0" class="text-sm text-destructive">{{ t('wl.audiences.send.noRecipients') }}</p>
      <SaveError :message="formError" />
      <DialogFooter>
        <Button variant="outline" @click="open = false">{{ t('wl.audiences.cancel') }}</Button>
        <Button :disabled="!canCreate" data-testid="send-confirm" @click="create">
          {{ t('wl.audiences.send.confirm', { n: preview?.recipients ?? 0 }) }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
