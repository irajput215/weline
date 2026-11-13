<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { BellRing, Lock, Pencil, Trash2 } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { PageHeader, ErrorState } from '@/components/shared'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { Kind, Rule, Variable } from '../api'
import { useRemindersStore, type LoadState } from '../stores/reminders'

type Tab = 'rules' | 'sends' | 'recalls'
const { t } = useI18n()
const auth = useAuthStore()
const store = useRemindersStore()
const can = (res: string, act: string) => auth.hasPermission(res, act)
const tabs = computed(() =>
  ([
    { key: 'rules', perm: 'wl_reminders_rules' },
    { key: 'sends', perm: 'wl_reminders_rules' },
    { key: 'recalls', perm: 'wl_reminders_recalls' }
  ] as const).filter(x => can(x.perm, 'read'))
)
const tab = ref<Tab>('rules')
const recallRules = computed(() => store.rules.filter(r => r.kind === 'recall'))
const recallId = ref('')
const state = computed<LoadState>(() => {
  if (tab.value === 'recalls') return store.state.rules === 'ready' && recallId.value ? store.state.due : store.state.rules
  return store.state[tab.value]
})
const loading = computed(() => state.value === 'loading' || (state.value === 'idle' && !(tab.value === 'recalls' && !recallId.value)))
const errorText = computed(() =>
  store.saveError === 'forbidden' ? t('wl.reminders.saveForbidden') : store.saveError === 'error' ? t('wl.reminders.saveFailed') : store.saveError ?? ''
)
const allVars: Variable[] = ['name', 'service', 'when', 'location', 'practitioner']
const recallVars: Variable[] = ['name', 'service']
const date = (iso: string, time = true) =>
  new Intl.DateTimeFormat(undefined, time ? { day: 'numeric', month: 'short', hour: 'numeric', minute: '2-digit' } : { day: 'numeric', month: 'short', year: 'numeric' }).format(new Date(iso))

// ---- rule form ----
const editing = ref(false)
const form = reactive({ id: '' as string | undefined, kind: 'reminder' as Kind, name: '', hours: '24', days: '180', template: '', language: 'en', variables: [] as Variable[], enabled: true })
function openForm(r?: Rule) {
  store.saveError = null
  Object.assign(form, r
    ? { id: r.id, kind: r.kind, name: r.name, hours: String(r.offset_minutes / 60), days: String(r.recall_days || 180), template: r.template_name, language: r.language, variables: [...r.variables], enabled: r.enabled }
    : { id: undefined, kind: 'reminder', name: '', hours: '24', days: '180', template: '', language: 'en', variables: ['name', 'when'], enabled: true })
  editing.value = true
}
watch(() => form.kind, k => { if (k === 'recall') form.variables = form.variables.filter(v => recallVars.includes(v)) })
watch(() => form.template, name => {
  const tpl = store.templates.find(x => x.name === name)
  if (tpl) form.language = tpl.language
})
function toggleVar(v: Variable) {
  form.variables = form.variables.includes(v) ? form.variables.filter(x => x !== v) : [...form.variables, v]
}
async function saveRule() {
  const ok = await store.saveRule({ id: form.id, kind: form.kind, name: form.name.trim(), offset_minutes: form.kind === 'reminder' ? Math.round(Number(form.hours) * 60) : 0,
    recall_days: form.kind === 'recall' ? Number(form.days) : 0, template_name: form.template, language: form.language, variables: form.variables, service_ids: [], enabled: form.enabled })
  if (ok) {
    editing.value = false
    toast.success(t('wl.reminders.saved'))
  }
}

// ---- recalls ----
const picked = ref<string[]>([])
watch(recallId, id => {
  picked.value = []
  if (id) void store.loadDue(id)
})
const allPicked = computed(() => store.due.length > 0 && picked.value.length === store.due.length)
function pickAll() {
  picked.value = allPicked.value ? [] : store.due.map(d => d.customer_id)
}
async function send() {
  const c = await store.sendRecalls(recallId.value, picked.value)
  if (c) {
    picked.value = []
    toast.success(t('wl.reminders.sentCounts', c))
  }
}

function load(k: Tab) {
  if (k === 'rules' || k === 'recalls') void store.loadRules()
  if (k === 'sends') void store.loadSends()
}
watch(tab, load)
watch(recallRules, rs => { if (!recallId.value && rs[0]?.id) recallId.value = rs[0].id })
onMounted(() => {
  tab.value = tabs.value[0]?.key ?? 'rules'
  load(tab.value)
  if (can('wl_reminders_rules', 'create') || can('wl_reminders_rules', 'update')) void store.loadTemplates()
})
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="t('wl.reminders.title')" :description="t('wl.reminders.subtitle')" :icon="BellRing"
      icon-gradient="bg-gradient-to-br from-amber-500 to-orange-600 shadow-amber-500/20" />
    <nav class="flex gap-1 border-b border-border px-6">
      <button v-for="x in tabs" :key="x.key" type="button" class="px-3 py-2 text-sm border-b-2 -mb-px"
        :class="tab === x.key ? 'border-primary text-foreground font-medium' : 'border-transparent text-muted-foreground'"
        :data-testid="`reminders-tab-${x.key}`" @click="tab = x.key">{{ t(`wl.reminders.tabs.${x.key}`) }}</button>
    </nav>
    <EmptyState v-if="state === 'forbidden'" data-testid="reminders-forbidden" :icon="Lock" :title="t('wl.reminders.forbiddenTitle')"
      :description="t('wl.reminders.forbiddenDescription')" class="flex-1" />
    <ErrorState v-else-if="state === 'error'" :title="t('common.loadErrorTitle')" :description="t('common.loadErrorDescription')"
      :retry-label="t('common.retryLoad')" class="flex-1" @retry="tab === 'recalls' && recallId ? store.loadDue(recallId) : load(tab)" />
    <ScrollArea v-else class="flex-1">
      <div class="p-6"><div class="max-w-4xl mx-auto space-y-6">
        <div v-if="loading" class="space-y-3" data-testid="reminders-loading"><Skeleton v-for="i in 3" :key="i" class="h-16 w-full" /></div>

        <template v-else-if="tab === 'rules'">
          <p class="text-sm text-muted-foreground">{{ t('wl.reminders.replyNote') }}</p>
          <Card v-if="editing" data-testid="reminders-form">
            <CardHeader><CardTitle class="text-base">{{ form.id ? t('wl.reminders.editRule') : t('wl.reminders.addRule') }}</CardTitle></CardHeader>
            <CardContent class="space-y-4">
              <div class="grid gap-4 sm:grid-cols-2">
                <div class="space-y-1">
                  <Label>{{ t('wl.reminders.kind') }}</Label>
                  <Select v-model="form.kind" :disabled="!!form.id">
                    <SelectTrigger :aria-label="t('wl.reminders.kind')" data-testid="reminders-kind"><SelectValue /></SelectTrigger>
                    <SelectContent><SelectItem v-for="k in ['reminder', 'recall']" :key="k" :value="k">{{ t(`wl.reminders.kinds.${k}`) }}</SelectItem></SelectContent>
                  </Select>
                </div>
                <div class="space-y-1">
                  <Label for="wl-rem-name">{{ t('wl.reminders.name') }}</Label>
                  <Input id="wl-rem-name" v-model="form.name" maxlength="100" data-testid="reminders-name" />
                </div>
                <div v-if="form.kind === 'reminder'" class="space-y-1">
                  <Label for="wl-rem-hours">{{ t('wl.reminders.offsetHours') }}</Label>
                  <Input id="wl-rem-hours" v-model="form.hours" type="number" min="0.5" max="336" step="0.5" data-testid="reminders-hours" />
                  <p class="text-xs text-muted-foreground">{{ t('wl.reminders.offsetHelp') }}</p>
                </div>
                <div v-else class="space-y-1">
                  <Label for="wl-rem-days">{{ t('wl.reminders.recallDays') }}</Label>
                  <Input id="wl-rem-days" v-model="form.days" type="number" min="7" max="1095" data-testid="reminders-days" />
                  <p class="text-xs text-muted-foreground">{{ t('wl.reminders.recallHelp') }}</p>
                </div>
                <div class="space-y-1">
                  <Label for="wl-rem-template">{{ t('wl.reminders.template') }}</Label>
                  <Select v-if="store.templates.length" v-model="form.template">
                    <SelectTrigger id="wl-rem-template" data-testid="reminders-template"><SelectValue /></SelectTrigger>
                    <SelectContent><SelectItem v-for="x in store.templates" :key="`${x.name}-${x.language}`" :value="x.name">{{ x.name }} ({{ x.language }})</SelectItem></SelectContent>
                  </Select>
                  <Input v-else id="wl-rem-template" v-model="form.template" data-testid="reminders-template-name" />
                  <p class="text-xs text-muted-foreground">{{ t('wl.reminders.templateHelp') }}</p>
                </div>
              </div>
              <fieldset class="space-y-2">
                <legend class="text-sm font-medium">{{ t('wl.reminders.variables') }}</legend>
                <div class="flex flex-wrap gap-2">
                  <button v-for="v in (form.kind === 'recall' ? recallVars : allVars)" :key="v" type="button" :aria-pressed="form.variables.includes(v)"
                    class="rounded-full border px-3 py-1 text-xs" :class="form.variables.includes(v) ? 'border-primary bg-primary/10' : 'border-border text-muted-foreground'"
                    :data-testid="`reminders-var-${v}`" @click="toggleVar(v)">
                    <span v-if="form.variables.includes(v)">{{ form.variables.indexOf(v) + 1 }}. </span>{{ t(`wl.reminders.variableNames.${v}`) }}
                  </button>
                </div>
                <p class="text-xs text-muted-foreground">{{ t('wl.reminders.variablesHelp') }}</p>
              </fieldset>
              <div class="flex items-center gap-2">
                <Switch id="wl-rem-enabled" v-model:checked="form.enabled" />
                <Label for="wl-rem-enabled">{{ t('wl.reminders.enabled') }}</Label>
              </div>
              <div class="flex gap-2">
                <Button :disabled="store.saving || !form.name.trim() || !form.template" data-testid="reminders-save" @click="saveRule">{{ t('wl.reminders.save') }}</Button>
                <Button variant="ghost" @click="editing = false">{{ t('wl.reminders.cancel') }}</Button>
              </div>
            </CardContent>
          </Card>
          <Button v-else-if="can('wl_reminders_rules', 'create')" data-testid="reminders-add" @click="openForm()">{{ t('wl.reminders.addRule') }}</Button>

          <EmptyState v-if="store.rules.length === 0" data-testid="reminders-rules-empty" :icon="BellRing" :title="t('wl.reminders.rulesEmpty')"
            :description="t('wl.reminders.rulesEmptyHelp')" />
          <Card v-else><CardContent class="p-0"><Table data-testid="reminders-rules">
            <TableHeader><TableRow><TableHead>{{ t('wl.reminders.name') }}</TableHead><TableHead>{{ t('wl.reminders.timing') }}</TableHead>
              <TableHead>{{ t('wl.reminders.template') }}</TableHead><TableHead /></TableRow></TableHeader>
            <TableBody><TableRow v-for="r in store.rules" :key="r.id" :data-testid="`reminders-rule-${r.name}`">
              <TableCell>{{ r.name }} <Badge variant="secondary">{{ t(`wl.reminders.kinds.${r.kind}`) }}</Badge>
                <Badge v-if="!r.enabled" variant="outline" class="ml-1">{{ t('wl.reminders.off') }}</Badge></TableCell>
              <TableCell>{{ r.kind === 'reminder' ? t('wl.reminders.beforeHours', { n: r.offset_minutes / 60 }) : t('wl.reminders.afterDays', { n: r.recall_days }) }}</TableCell>
              <TableCell>{{ r.template_name }}</TableCell>
              <TableCell class="text-right whitespace-nowrap">
                <Button v-if="can('wl_reminders_rules', 'update')" variant="ghost" size="sm" :aria-label="t('wl.reminders.edit')" @click="openForm(r)"><Pencil class="h-4 w-4" /></Button>
                <Button v-if="can('wl_reminders_rules', 'delete')" variant="ghost" size="sm" :aria-label="t('wl.reminders.remove')" @click="store.removeRule(r.id!)"><Trash2 class="h-4 w-4" /></Button>
              </TableCell>
            </TableRow></TableBody>
          </Table></CardContent></Card>
        </template>

        <template v-else-if="tab === 'sends'">
          <EmptyState v-if="store.sends.length === 0" data-testid="reminders-sends-empty" :icon="BellRing" :title="t('wl.reminders.sendsEmpty')" />
          <Card v-else><CardContent class="p-0"><Table data-testid="reminders-sends">
            <TableHeader><TableRow><TableHead>{{ t('wl.reminders.when') }}</TableHead><TableHead>{{ t('wl.reminders.rule') }}</TableHead>
              <TableHead>{{ t('wl.reminders.appointment') }}</TableHead><TableHead>{{ t('wl.reminders.status') }}</TableHead><TableHead>{{ t('wl.reminders.reply') }}</TableHead></TableRow></TableHeader>
            <TableBody><TableRow v-for="s in store.sends" :key="s.id">
              <TableCell class="whitespace-nowrap">{{ date(s.created_at) }}</TableCell>
              <TableCell>{{ s.rule_name }}</TableCell>
              <TableCell class="whitespace-nowrap">{{ s.kind === 'reminder' ? date(s.start_at) : date(s.start_at, false) }}</TableCell>
              <TableCell :data-testid="`reminders-send-${s.status}`">{{ t(`wl.reminders.statuses.${s.status}`) }}
                <span v-if="s.reason" class="text-xs text-muted-foreground">({{ $te(`wl.reminders.reasons.${s.reason}`) ? t(`wl.reminders.reasons.${s.reason}`) : s.reason }})</span></TableCell>
              <TableCell>{{ t(`wl.reminders.replies.${s.reply}`) }}</TableCell>
            </TableRow></TableBody>
          </Table></CardContent></Card>
          <div v-if="store.nextBefore" class="text-center"><Button variant="ghost" @click="store.loadMore()">{{ t('wl.reminders.loadMore') }}</Button></div>
        </template>

        <template v-else-if="tab === 'recalls'">
          <EmptyState v-if="recallRules.length === 0" data-testid="reminders-no-recalls" :icon="BellRing" :title="t('wl.reminders.noRecallRules')" />
          <template v-else>
            <div class="max-w-xs space-y-1">
              <Label>{{ t('wl.reminders.recallRule') }}</Label>
              <Select v-model="recallId">
                <SelectTrigger :aria-label="t('wl.reminders.recallRule')" data-testid="reminders-recall-rule"><SelectValue /></SelectTrigger>
                <SelectContent><SelectItem v-for="r in recallRules" :key="r.id" :value="r.id!">{{ r.name }}</SelectItem></SelectContent>
              </Select>
            </div>
            <EmptyState v-if="store.due.length === 0" data-testid="reminders-due-empty" :icon="BellRing" :title="t('wl.reminders.dueEmpty')" />
            <template v-else>
              <Card><CardContent class="p-0"><Table data-testid="reminders-due">
                <TableHeader><TableRow>
                  <TableHead class="w-8"><input type="checkbox" :checked="allPicked" :aria-label="t('wl.reminders.selectAll')" @change="pickAll" /></TableHead>
                  <TableHead>{{ t('wl.reminders.patient') }}</TableHead><TableHead>{{ t('wl.reminders.service') }}</TableHead><TableHead>{{ t('wl.reminders.lastVisit') }}</TableHead>
                </TableRow></TableHeader>
                <TableBody><TableRow v-for="d in store.due" :key="d.customer_id">
                  <TableCell><input v-model="picked" type="checkbox" :value="d.customer_id" :aria-label="d.name" /></TableCell>
                  <TableCell>{{ d.name }}</TableCell><TableCell>{{ d.service }}</TableCell><TableCell>{{ date(d.last_visit, false) }}</TableCell>
                </TableRow></TableBody>
              </Table></CardContent></Card>
              <Button v-if="can('wl_reminders_recalls', 'create')" :disabled="store.saving || picked.length === 0" data-testid="reminders-send" @click="send">
                {{ t('wl.reminders.sendSelected', { n: picked.length }) }}</Button>
            </template>
          </template>
        </template>
        <p v-if="errorText" class="text-sm text-destructive" role="alert" data-testid="reminders-error">{{ errorText }}</p>
      </div></div>
    </ScrollArea>
  </div>
</template>
