<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { ArrowLeft, Plus, Send } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import AudiencesShell from '../components/AudiencesShell.vue'
import ConditionRow from '../components/ConditionRow.vue'
import CreateCampaignDialog from '../components/CreateCampaignDialog.vue'
import PreviewPanel from '../components/PreviewPanel.vue'
import SaveError from '../components/SaveError.vue'
import { MAX_CONDITIONS, cleanRules, isComplete, newCondition, useAudiencesStore, type LoadState } from '../stores/audiences'
import type { Rules } from '../api'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const store = useAudiencesStore()

const id = computed(() => (route.params.id as string | undefined) ?? null)
const canEdit = computed(() => auth.hasPermission('wl_audiences_segments', id.value ? 'update' : 'create'))
const canSend = computed(() => auth.hasPermission('wl_audiences_sends', 'create') && auth.hasPermission('campaigns', 'write'))

const name = ref('')
const description = ref('')
const rules = ref<Rules>({ match: 'all', conditions: [newCondition('tag')] })
const savedSnapshot = ref('')
const formError = ref<string | null>(null)
const sendOpen = ref(false)

const snapshot = () => JSON.stringify({ n: name.value.trim(), d: description.value.trim(), r: cleanRules(rules.value) })
const dirty = computed(() => snapshot() !== savedSnapshot.value)
const complete = computed(() => rules.value.conditions.length > 0 && rules.value.conditions.every(isComplete))

const pageState = computed<LoadState>(() => {
  const states: LoadState[] = id.value ? [store.state.fields, store.state.segment] : [store.state.fields]
  for (const s of ['forbidden', 'error', 'loading', 'idle'] as LoadState[]) if (states.includes(s)) return s
  return 'ready'
})

async function load() {
  store.resetPreview()
  store.saveError = null
  const jobs: Promise<void>[] = [store.loadFields()]
  if (id.value) jobs.push(store.loadSegment(id.value))
  await Promise.all(jobs)
  if (id.value && store.segment) {
    name.value = store.segment.name
    description.value = store.segment.description
    rules.value = JSON.parse(JSON.stringify(store.segment.rules))
  } else if (!id.value) {
    name.value = ''
    description.value = ''
    rules.value = { match: 'all', conditions: [newCondition('tag')] }
  }
  savedSnapshot.value = id.value ? snapshot() : ''
}

let timer: ReturnType<typeof setTimeout> | undefined
watch(
  rules,
  r => {
    clearTimeout(timer)
    timer = setTimeout(() => store.runPreview({ rules: r }), 400)
  },
  { deep: true }
)
onBeforeUnmount(() => clearTimeout(timer))

function addCondition() {
  if (rules.value.conditions.length < MAX_CONDITIONS) rules.value.conditions.push(newCondition('tag'))
}

async function save() {
  formError.value = null
  if (!name.value.trim()) return (formError.value = t('wl.audiences.builder.needsName'))
  const seg = await store.saveSegment(id.value, { name: name.value.trim(), description: description.value.trim(), rules: rules.value })
  if (!seg) return
  toast.success(t('wl.audiences.saved'))
  savedSnapshot.value = snapshot()
  if (!id.value) await router.replace(`/x/audiences/segments/${seg.id}`)
}

watch(id, load)
onMounted(load)
</script>

<template>
  <AudiencesShell :state="pageState" @retry="load">
    <RouterLink to="/x/audiences" class="inline-flex items-center text-sm text-muted-foreground hover:text-foreground">
      <ArrowLeft class="h-4 w-4 mr-1" />{{ t('wl.audiences.back') }}
    </RouterLink>
    <p v-if="!canEdit" class="text-sm text-muted-foreground" data-testid="builder-readonly">{{ t('wl.audiences.readOnly') }}</p>

    <div class="grid gap-6 lg:grid-cols-[1fr_18rem]">
      <Card>
        <CardHeader>
          <CardTitle>{{ id ? t('wl.audiences.builder.editTitle') : t('wl.audiences.builder.newTitle') }}</CardTitle>
        </CardHeader>
        <CardContent class="space-y-5">
          <div class="grid gap-3 sm:grid-cols-2">
            <div class="space-y-1">
              <Label for="wl-aud-name">{{ t('wl.audiences.builder.name') }}</Label>
              <Input id="wl-aud-name" v-model="name" maxlength="120" :disabled="!canEdit" data-testid="segment-name" />
            </div>
            <div class="space-y-1 sm:col-span-2">
              <Label for="wl-aud-desc">{{ t('wl.audiences.builder.description') }}</Label>
              <Textarea id="wl-aud-desc" v-model="description" :rows="2" maxlength="500" :disabled="!canEdit" />
            </div>
          </div>

          <div class="flex items-center gap-3 text-sm">
            <span>{{ t('wl.audiences.builder.match') }}</span>
            <ToggleGroup
              type="single"
              :model-value="rules.match"
              :disabled="!canEdit"
              @update:model-value="v => { if (v) rules.match = v as 'all' | 'any' }"
            >
              <ToggleGroupItem value="all" data-testid="match-all">{{ t('wl.audiences.builder.matchAll') }}</ToggleGroupItem>
              <ToggleGroupItem value="any" data-testid="match-any">{{ t('wl.audiences.builder.matchAny') }}</ToggleGroupItem>
            </ToggleGroup>
          </div>

          <div class="space-y-3">
            <ConditionRow
              v-for="(_, i) in rules.conditions"
              :key="i"
              v-model="rules.conditions[i]"
              :index="i"
              :fields="store.fields"
              :readonly="!canEdit"
              @remove="rules.conditions.splice(i, 1)"
            />
          </div>
          <div v-if="canEdit" class="flex items-center gap-3">
            <Button variant="outline" size="sm" :disabled="rules.conditions.length >= MAX_CONDITIONS" data-testid="condition-add" @click="addCondition">
              <Plus class="h-4 w-4 mr-1" />{{ t('wl.audiences.builder.addCondition') }}
            </Button>
            <span v-if="rules.conditions.length >= MAX_CONDITIONS" class="text-xs text-muted-foreground">
              {{ t('wl.audiences.builder.maxConditions', { n: MAX_CONDITIONS }) }}
            </span>
          </div>

          <SaveError :message="formError" />
          <div class="flex flex-wrap gap-2">
            <Button v-if="canEdit" :disabled="store.saving || !complete || !dirty" data-testid="segment-save" @click="save">
              {{ t('wl.audiences.save') }}
            </Button>
            <Button
              v-if="canSend && id"
              variant="secondary"
              :disabled="dirty || !complete"
              :title="dirty ? t('wl.audiences.builder.saveFirst') : ''"
              data-testid="segment-create-campaign"
              @click="sendOpen = true"
            >
              <Send class="h-4 w-4 mr-2" />{{ t('wl.audiences.builder.createCampaign') }}
            </Button>
          </div>
          <p v-if="canSend && id && dirty" class="text-xs text-muted-foreground">{{ t('wl.audiences.builder.saveFirst') }}</p>
        </CardContent>
      </Card>

      <PreviewPanel :preview="store.preview" :loading="store.previewLoading" :error="store.previewError" :incomplete="!complete" />
    </div>

    <CreateCampaignDialog
      v-if="id"
      v-model:open="sendOpen"
      :segment-id="id"
      :rules="cleanRules(rules)"
      :default-name="name"
      @created="store.loadSegment(id)"
    />
  </AudiencesShell>
</template>
