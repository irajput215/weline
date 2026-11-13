<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Lock, PlugZap } from 'lucide-vue-next'
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
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { ProviderInfo } from '../api'
import { useConnectorsStore } from '../stores/connectors'

const { t, te } = useI18n()
const auth = useAuthStore()
const store = useConnectorsStore()
const canEdit = computed(() => auth.hasPermission('wl_connectors_settings', 'update'))
const loading = computed(() => store.state === 'loading' || store.state === 'idle')
const chosen = ref<string>('')
const creds = reactive<Record<string, string>>({})

const label = (key: string, fallback: string) => (te(key) ? t(key) : fallback)
const fmt = (iso: string | null) =>
  iso ? new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', hour: 'numeric', minute: '2-digit' }).format(new Date(iso)) : t('wl.connectors.never')
const tone = { connected: 'default', error: 'destructive', not_set_up: 'secondary' } as const
const errorText = computed(() => {
  switch (store.saveError) {
    case null:
      return ''
    case 'forbidden':
      return t('wl.connectors.saveForbidden')
    case 'error':
      return t('wl.connectors.saveFailed')
    default:
      return store.saveError
  }
})

function choose(p: ProviderInfo) {
  chosen.value = p.name
  for (const k of Object.keys(creds)) delete creds[k]
  if (p.fields.length === 0) void submit(p)
}
async function submit(p: ProviderInfo) {
  const body: Record<string, string> = {}
  for (const f of p.fields) if (creds[f.name]?.trim()) body[f.name] = creds[f.name].trim()
  if (await store.save(p.name, Object.keys(body).length ? body : undefined)) {
    chosen.value = ''
    toast.success(t('wl.connectors.saved'))
  }
}
onMounted(store.load)
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader
      :title="t('wl.connectors.title')"
      :description="t('wl.connectors.subtitle')"
      :icon="PlugZap"
      icon-gradient="bg-gradient-to-br from-sky-500 to-indigo-600 shadow-sky-500/20"
    />
    <EmptyState
      v-if="store.state === 'forbidden'"
      data-testid="connectors-forbidden"
      :icon="Lock"
      :title="t('wl.connectors.forbiddenTitle')"
      :description="t('wl.connectors.forbiddenDescription')"
      class="flex-1"
    />
    <ErrorState
      v-else-if="store.state === 'error'"
      :title="t('common.loadErrorTitle')"
      :description="t('common.loadErrorDescription')"
      :retry-label="t('common.retryLoad')"
      class="flex-1"
      @retry="store.load"
    />
    <ScrollArea v-else class="flex-1">
      <div class="p-6">
        <div class="max-w-4xl mx-auto space-y-6">
          <div v-if="loading" class="space-y-3" data-testid="connectors-loading">
            <Skeleton v-for="i in 3" :key="i" class="h-20 w-full" />
          </div>
          <template v-else-if="store.account">
            <p v-if="!canEdit" class="text-sm text-muted-foreground" data-testid="connectors-read-only">{{ t('wl.connectors.readOnly') }}</p>
            <p class="text-sm text-muted-foreground">{{ t('wl.connectors.switchNote') }}</p>

            <div class="grid gap-4 sm:grid-cols-2">
              <Card
                v-for="p in store.providers"
                :key="p.name"
                :class="(store.account.provider === p.name ? 'border-primary ' : '') + (p.available ? '' : 'opacity-60')"
                :data-testid="`connectors-provider-${p.name}`"
              >
                <CardHeader>
                  <div class="flex items-center justify-between gap-2">
                    <CardTitle class="text-base">{{ label(`wl.connectors.providers.${p.name}`, p.name) }}</CardTitle>
                    <Badge v-if="store.account.provider === p.name" :variant="tone[store.account.status]" data-testid="connectors-status">
                      {{ t(`wl.connectors.statuses.${store.account.status}`) }}
                    </Badge>
                    <Badge v-else-if="!p.available" variant="outline">{{ t('wl.connectors.comingSoon') }}</Badge>
                  </div>
                  <CardDescription>{{ label(`wl.connectors.providerDescriptions.${p.name}`, '') }}</CardDescription>
                </CardHeader>
                <CardContent class="space-y-3">
                  <template v-if="store.account.provider === p.name">
                    <p class="text-xs text-muted-foreground">
                      {{ t('wl.connectors.lastTest', { when: fmt(store.account.last_test_at) }) }}
                      · {{ t('wl.connectors.lastSync', { when: fmt(store.account.last_sync_at) }) }}
                    </p>
                    <p v-if="store.account.last_error" class="text-sm text-destructive">
                      {{ t('wl.connectors.lastError', { error: store.account.last_error }) }}
                    </p>
                    <Button v-if="canEdit && p.fields.length" variant="outline" size="sm" :disabled="store.saving" data-testid="connectors-test" @click="store.test()">
                      {{ t('wl.connectors.test') }}
                    </Button>
                  </template>
                  <Button
                    v-if="canEdit && p.available && store.account.provider !== p.name && chosen !== p.name"
                    size="sm"
                    :disabled="store.saving"
                    :data-testid="`connectors-choose-${p.name}`"
                    @click="choose(p)"
                  >
                    {{ t('wl.connectors.choose') }}
                  </Button>
                  <form v-if="canEdit && chosen === p.name && p.fields.length" class="space-y-3" @submit.prevent="submit(p)">
                    <div v-for="fl in p.fields" :key="fl.name" class="space-y-1">
                      <Label :for="`wl-conn-${fl.name}`">{{ label(`wl.connectors.fields.${fl.name}`, fl.name) }}</Label>
                      <Input
                        :id="`wl-conn-${fl.name}`"
                        v-model="creds[fl.name]"
                        :type="fl.secret ? 'password' : 'text'"
                        autocomplete="off"
                        :data-testid="`connectors-field-${fl.name}`"
                      />
                    </div>
                    <Button type="submit" size="sm" :disabled="store.saving" data-testid="connectors-save">{{ t('wl.connectors.saveAndTest') }}</Button>
                  </form>
                </CardContent>
              </Card>
            </div>
            <p v-if="errorText" class="text-sm text-destructive" role="alert" data-testid="connectors-error">{{ errorText }}</p>
            <p v-if="store.testMessage" :class="store.testMessage.ok ? 'text-sm' : 'text-sm text-destructive'" data-testid="connectors-test-result">
              {{ store.testMessage.message }}
            </p>

            <Card>
              <CardHeader><CardTitle class="text-base">{{ t('wl.connectors.log.title') }}</CardTitle></CardHeader>
              <CardContent class="p-0">
                <p v-if="store.syncs.length === 0" class="p-6 text-sm text-muted-foreground" data-testid="connectors-log-empty">{{ t('wl.connectors.log.empty') }}</p>
                <Table v-else data-testid="connectors-log">
                  <TableHeader>
                    <TableRow>
                      <TableHead>{{ t('wl.connectors.log.when') }}</TableHead>
                      <TableHead>{{ t('wl.connectors.log.what') }}</TableHead>
                      <TableHead>{{ t('wl.connectors.log.outcome') }}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    <TableRow v-for="s in store.syncs" :key="s.id">
                      <TableCell class="whitespace-nowrap">{{ fmt(s.started_at) }}</TableCell>
                      <TableCell>{{ t(`wl.connectors.log.kinds.${s.kind}`) }} · {{ label(`wl.connectors.providers.${s.provider}`, s.provider) }}</TableCell>
                      <TableCell>
                        {{ t(`wl.connectors.log.outcomes.${s.outcome}`) }}
                        <span v-if="s.error" class="text-muted-foreground"> · {{ s.error }}</span>
                      </TableCell>
                    </TableRow>
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
          </template>
        </div>
      </div>
    </ScrollArea>
  </div>
</template>
