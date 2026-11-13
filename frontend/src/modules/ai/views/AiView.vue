<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Bot, Lock, Trash2 } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { PageHeader, ErrorState } from '@/components/shared'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useAiStore, type LoadState } from '../stores/ai'

type Tab = 'settings' | 'knowledge' | 'runs'
const { t } = useI18n()
const auth = useAuthStore()
const store = useAiStore()
const can = (res: string, act: string) => auth.hasPermission(res, act)
const tabs = computed(() =>
  ([
    { key: 'settings', perm: 'wl_ai_settings' },
    { key: 'knowledge', perm: 'wl_ai_knowledge' },
    { key: 'runs', perm: 'wl_ai_runs' }
  ] as const).filter(x => can(x.perm, 'read'))
)
const tab = ref<Tab>('runs')
const state = computed<LoadState>(() => store.state[tab.value])
const loading = computed(() => state.value === 'loading' || state.value === 'idle')

const form = reactive({ enabled: false, tone: 'friendly' as 'friendly' | 'formal' | 'brief', instructions: '', threshold: '0.7', words: '' })
const faq = reactive({ question: '', answer: '' })
const doc = reactive({ title: '', text: '' })
const errorText = computed(() =>
  store.saveError === 'forbidden' ? t('wl.ai.saveForbidden') : store.saveError === 'error' ? t('wl.ai.saveFailed') : store.saveError ?? ''
)
const when = (iso: string) => new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', hour: 'numeric', minute: '2-digit' }).format(new Date(iso))

function load(k: Tab) {
  if (k === 'settings') void store.loadSettings()
  if (k === 'knowledge') void store.loadKnowledge()
  if (k === 'runs') void store.loadRuns()
}
watch(tab, load)
watch(() => store.settings, s => {
  if (!s) return
  Object.assign(form, { enabled: s.enabled, tone: s.tone, instructions: s.instructions, threshold: String(s.confidence_threshold), words: s.emergency_words.join(', ') })
})
async function saveSettings() {
  const ok = await store.saveSettings({ enabled: form.enabled, handover_team_id: store.settings?.handover_team_id ?? null, tone: form.tone,
    instructions: form.instructions, confidence_threshold: Number(form.threshold), emergency_words: form.words.split(',').map(w => w.trim()).filter(Boolean) })
  if (ok) toast.success(t('wl.ai.saved'))
}
async function addFaq() {
  if (await store.addFaq(faq.question, faq.answer)) Object.assign(faq, { question: '', answer: '' })
}
async function addDoc() {
  if (await store.addDocument(doc.title, doc.text)) Object.assign(doc, { title: '', text: '' })
}
async function readFile(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (!f) return
  doc.title = doc.title || f.name.replace(/\.(txt|md)$/i, '')
  doc.text = await f.text()
}
onMounted(() => {
  tab.value = tabs.value[0]?.key ?? 'runs'
  load(tab.value)
})
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="t('wl.ai.title')" :description="t('wl.ai.subtitle')" :icon="Bot"
      icon-gradient="bg-gradient-to-br from-emerald-500 to-teal-600 shadow-emerald-500/20" />
    <nav class="flex gap-1 border-b border-border px-6">
      <button v-for="x in tabs" :key="x.key" type="button" class="px-3 py-2 text-sm border-b-2 -mb-px"
        :class="tab === x.key ? 'border-primary text-foreground font-medium' : 'border-transparent text-muted-foreground'"
        :data-testid="`ai-tab-${x.key}`" @click="tab = x.key">{{ t(`wl.ai.tabs.${x.key}`) }}</button>
    </nav>
    <EmptyState v-if="state === 'forbidden'" data-testid="ai-forbidden" :icon="Lock" :title="t('wl.ai.forbiddenTitle')"
      :description="t('wl.ai.forbiddenDescription')" class="flex-1" />
    <ErrorState v-else-if="state === 'error'" :title="t('common.loadErrorTitle')" :description="t('common.loadErrorDescription')"
      :retry-label="t('common.retryLoad')" class="flex-1" @retry="load(tab)" />
    <ScrollArea v-else class="flex-1">
      <div class="p-6"><div class="max-w-4xl mx-auto space-y-6">
        <div v-if="loading" class="space-y-3" data-testid="ai-loading"><Skeleton v-for="i in 3" :key="i" class="h-16 w-full" /></div>

        <template v-else-if="tab === 'settings' && store.settings">
          <Card v-if="!store.provider?.key_set" data-testid="ai-no-provider">
            <CardContent class="p-4 text-sm">{{ t('wl.ai.noProvider') }}</CardContent>
          </Card>
          <p v-else class="text-sm text-muted-foreground">{{ t('wl.ai.provider', { name: store.provider?.name, model: store.provider?.model }) }}</p>
          <p class="text-sm text-muted-foreground">{{ t('wl.ai.chatbotNote') }}</p>
          <Card><CardContent class="p-4 space-y-4">
            <div class="flex items-center gap-2">
              <Switch id="wl-ai-enabled" v-model:checked="form.enabled" :disabled="!can('wl_ai_settings', 'update')" data-testid="ai-enabled" />
              <Label for="wl-ai-enabled">{{ t('wl.ai.enabled') }}</Label>
            </div>
            <div class="grid gap-4 sm:grid-cols-2">
              <div class="space-y-1">
                <Label>{{ t('wl.ai.tone') }}</Label>
                <Select v-model="form.tone" :disabled="!can('wl_ai_settings', 'update')">
                  <SelectTrigger :aria-label="t('wl.ai.tone')"><SelectValue /></SelectTrigger>
                  <SelectContent><SelectItem v-for="x in ['friendly', 'formal', 'brief']" :key="x" :value="x">{{ t(`wl.ai.tones.${x}`) }}</SelectItem></SelectContent>
                </Select>
              </div>
              <div class="space-y-1">
                <Label for="wl-ai-threshold">{{ t('wl.ai.threshold') }}</Label>
                <Input id="wl-ai-threshold" v-model="form.threshold" type="number" min="0.3" max="0.95" step="0.05" :disabled="!can('wl_ai_settings', 'update')" />
              </div>
            </div>
            <div class="space-y-1">
              <Label for="wl-ai-instructions">{{ t('wl.ai.instructions') }}</Label>
              <Textarea id="wl-ai-instructions" v-model="form.instructions" :rows="3" maxlength="4000" :disabled="!can('wl_ai_settings', 'update')" />
            </div>
            <div class="space-y-1">
              <Label for="wl-ai-words">{{ t('wl.ai.emergencyWords') }}</Label>
              <Textarea id="wl-ai-words" v-model="form.words" :rows="2" :disabled="!can('wl_ai_settings', 'update')" />
              <p class="text-xs text-muted-foreground">{{ t('wl.ai.emergencyHelp') }}</p>
            </div>
            <Button v-if="can('wl_ai_settings', 'update')" :disabled="store.saving" data-testid="ai-save" @click="saveSettings">{{ t('wl.ai.save') }}</Button>
          </CardContent></Card>
        </template>

        <template v-else-if="tab === 'knowledge'">
          <div v-if="can('wl_ai_knowledge', 'create')" class="grid gap-4 sm:grid-cols-2">
            <Card><CardHeader><CardTitle class="text-base">{{ t('wl.ai.addFaq') }}</CardTitle></CardHeader>
              <CardContent class="space-y-2">
                <Input v-model="faq.question" maxlength="200" :placeholder="t('wl.ai.question')" data-testid="ai-faq-question" />
                <Textarea v-model="faq.answer" :rows="3" maxlength="4000" :placeholder="t('wl.ai.answer')" data-testid="ai-faq-answer" />
                <Button size="sm" :disabled="store.saving || !faq.question || !faq.answer" data-testid="ai-faq-add" @click="addFaq">{{ t('wl.ai.add') }}</Button>
              </CardContent></Card>
            <Card><CardHeader><CardTitle class="text-base">{{ t('wl.ai.addDocument') }}</CardTitle>
              <CardDescription>{{ t('wl.ai.documentHelp') }}</CardDescription></CardHeader>
              <CardContent class="space-y-2">
                <Input v-model="doc.title" maxlength="200" :placeholder="t('wl.ai.docTitle')" />
                <input type="file" accept=".txt,.md,text/plain,text/markdown" class="text-sm" @change="readFile" />
                <Textarea v-model="doc.text" :rows="3" :placeholder="t('wl.ai.docText')" />
                <Button size="sm" :disabled="store.saving || !doc.title || !doc.text" @click="addDoc">{{ t('wl.ai.add') }}</Button>
              </CardContent></Card>
          </div>
          <EmptyState v-if="store.items.length === 0" data-testid="ai-knowledge-empty" :icon="Bot" :title="t('wl.ai.knowledgeEmpty')" :description="t('wl.ai.knowledgeEmptyHelp')" />
          <Card v-else><CardContent class="p-0"><Table data-testid="ai-knowledge">
            <TableHeader><TableRow><TableHead>{{ t('wl.ai.titleCol') }}</TableHead><TableHead>{{ t('wl.ai.kindCol') }}</TableHead><TableHead /></TableRow></TableHeader>
            <TableBody><TableRow v-for="it in store.items" :key="it.source_id" :data-testid="`ai-knowledge-${it.title}`">
              <TableCell>{{ it.title }}</TableCell>
              <TableCell><Badge variant="secondary">{{ t(`wl.ai.kinds.${it.kind}`) }}</Badge></TableCell>
              <TableCell class="text-right">
                <Button v-if="can('wl_ai_knowledge', 'delete')" variant="ghost" size="sm" :aria-label="t('wl.ai.remove')" @click="store.remove(it.source_id)"><Trash2 class="h-4 w-4" /></Button>
              </TableCell>
            </TableRow></TableBody>
          </Table></CardContent></Card>
        </template>

        <template v-else-if="tab === 'runs'">
          <EmptyState v-if="store.runs.length === 0" data-testid="ai-runs-empty" :icon="Bot" :title="t('wl.ai.runsEmpty')" />
          <Card v-else><CardContent class="p-0"><Table data-testid="ai-runs">
            <TableHeader><TableRow><TableHead>{{ t('wl.ai.when') }}</TableHead><TableHead>{{ t('wl.ai.contact') }}</TableHead>
              <TableHead>{{ t('wl.ai.intent') }}</TableHead><TableHead>{{ t('wl.ai.outcome') }}</TableHead></TableRow></TableHeader>
            <TableBody><TableRow v-for="r in store.runs" :key="r.id">
              <TableCell class="whitespace-nowrap">{{ when(r.created_at) }}</TableCell>
              <TableCell>{{ r.contact.name || '—' }}<p class="text-xs text-muted-foreground">{{ r.contact.phone_masked }}</p></TableCell>
              <TableCell>{{ t(`wl.ai.intents.${r.intent}`) }}</TableCell>
              <TableCell :data-testid="`ai-run-outcome-${r.outcome}`">{{ t(`wl.ai.outcomes.${r.outcome}`) }}</TableCell>
            </TableRow></TableBody>
          </Table></CardContent></Card>
          <div v-if="store.nextBefore" class="text-center"><Button variant="ghost" @click="store.loadMore()">{{ t('wl.ai.loadMore') }}</Button></div>
        </template>
        <p v-if="errorText" class="text-sm text-destructive" role="alert" data-testid="ai-error">{{ errorText }}</p>
      </div></div>
    </ScrollArea>
  </div>
</template>
