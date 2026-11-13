<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Lock, Package, X } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { PageHeader, ErrorState } from '@/components/shared'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { usePacksStore } from '../stores/packs'

const { t } = useI18n()
const auth = useAuthStore()
const store = usePacksStore()
const canInstall = computed(() => auth.hasPermission('wl_packs_install', 'create'))
const loading = computed(() => store.state === 'loading' || store.state === 'idle')
const p = computed(() => store.preview)
const day = (iso: string) => new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', year: 'numeric' }).format(new Date(iso))
const errorText = computed(() =>
  store.installError === 'forbidden' ? t('wl.packs.installForbidden') : store.installError === 'error' ? t('wl.packs.installFailed') : store.installError ?? ''
)
async function install(id: string) {
  if (await store.install(id)) toast.success(t('wl.packs.result'))
}
onMounted(() => void store.load())
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="t('wl.packs.title')" :description="t('wl.packs.subtitle')" :icon="Package"
      icon-gradient="bg-gradient-to-br from-teal-500 to-cyan-600 shadow-teal-500/20" />
    <EmptyState v-if="store.state === 'forbidden'" data-testid="packs-forbidden" :icon="Lock" :title="t('wl.packs.forbiddenTitle')"
      :description="t('wl.packs.forbiddenDescription')" class="flex-1" />
    <ErrorState v-else-if="store.state === 'error'" :title="t('common.loadErrorTitle')" :description="t('common.loadErrorDescription')"
      :retry-label="t('common.retryLoad')" class="flex-1" @retry="store.load()" />
    <ScrollArea v-else class="flex-1">
      <div class="p-6"><div class="max-w-5xl mx-auto space-y-6">
        <div v-if="loading" class="space-y-3" data-testid="packs-loading"><Skeleton v-for="i in 3" :key="i" class="h-24 w-full" /></div>
        <template v-else>
          <p class="text-sm text-muted-foreground">{{ t('wl.packs.installHelp') }}</p>
          <EmptyState v-if="store.packs.length === 0" :icon="Package" :title="t('wl.packs.empty')" />
          <div class="grid gap-4 md:grid-cols-2">
            <Card v-for="pk in store.packs" :key="pk.id" :data-testid="`packs-card-${pk.id}`" :class="pk.recommended ? 'border-primary' : ''">
              <CardHeader class="pb-2">
                <CardTitle class="text-base">{{ pk.name }} <Badge v-if="pk.recommended" class="ml-1" data-testid="packs-recommended">{{ t('wl.packs.recommended') }}</Badge></CardTitle>
                <CardDescription>{{ pk.description }}</CardDescription>
              </CardHeader>
              <CardContent class="space-y-3 text-sm">
                <p class="text-muted-foreground">{{ t('wl.packs.counts', pk.counts) }}</p>
                <p v-if="pk.installed" class="text-emerald-500" :data-testid="`packs-installed-${pk.id}`">{{ t('wl.packs.installedOn', { date: day(pk.installed.installed_at) }) }}</p>
                <div class="flex gap-2">
                  <Button variant="outline" size="sm" :data-testid="`packs-preview-${pk.id}`" @click="store.open(pk.id)">{{ t('wl.packs.preview') }}</Button>
                  <Button v-if="canInstall" size="sm" :disabled="store.installing" :data-testid="`packs-install-${pk.id}`" @click="install(pk.id)">
                    {{ pk.installed ? t('wl.packs.installAgain') : t('wl.packs.install') }}</Button>
                </div>
              </CardContent>
            </Card>
          </div>
          <p v-if="errorText" class="text-sm text-destructive" role="alert" data-testid="packs-error">{{ errorText }}</p>

          <Card v-if="store.result" data-testid="packs-result">
            <CardHeader class="pb-2"><CardTitle class="text-base">{{ t('wl.packs.result') }}</CardTitle></CardHeader>
            <CardContent class="text-sm space-y-1">
              <p v-for="(it, i) in store.result.items" :key="i" class="flex justify-between gap-3" :data-testid="`packs-item-${it.kind}-${it.status}`">
                <span><span class="text-muted-foreground">{{ t(`wl.packs.kinds.${it.kind}`) }}:</span> {{ it.name }}</span>
                <span :class="it.status === 'failed' ? 'text-destructive' : it.status === 'created' ? 'text-emerald-500' : 'text-muted-foreground'">
                  {{ t(`wl.packs.statuses.${it.status}`) }}</span>
              </p>
            </CardContent>
          </Card>

          <Card v-if="p" data-testid="packs-preview">
            <CardHeader class="flex flex-row items-center justify-between pb-2">
              <CardTitle class="text-base">{{ p.name }}</CardTitle>
              <Button variant="ghost" size="sm" :aria-label="t('wl.packs.close')" @click="store.preview = null"><X class="h-4 w-4" /></Button>
            </CardHeader>
            <CardContent class="space-y-4 text-sm">
              <section>
                <h3 class="font-medium mb-1">{{ t('wl.packs.templates') }}</h3>
                <div v-for="tp in p.templates" :key="tp.name" class="rounded-md border border-border p-2 mb-2">
                  <p class="text-xs text-muted-foreground">{{ tp.name }} · {{ tp.category }}</p>
                  <p class="whitespace-pre-line">{{ tp.body }}</p>
                  <p v-if="tp.buttons?.length" class="flex gap-1 mt-1"><Badge v-for="b in tp.buttons" :key="b" variant="outline">{{ b }}</Badge></p>
                </div>
              </section>
              <section>
                <h3 class="font-medium mb-1">{{ t('wl.packs.ai') }}</h3>
                <p class="text-muted-foreground text-xs mb-1">{{ t('wl.packs.aiHelp') }}</p>
                <p>{{ p.ai.instructions }}</p>
              </section>
              <section v-if="p.segments.length">
                <h3 class="font-medium mb-1">{{ t('wl.packs.segments') }}</h3>
                <p v-for="s in p.segments" :key="s.name">{{ s.name }} <span class="text-muted-foreground">· {{ s.description }}</span></p>
              </section>
              <section v-if="p.reminders.length">
                <h3 class="font-medium mb-1">{{ t('wl.packs.reminders') }}</h3>
                <p v-for="r in p.reminders" :key="r.name">{{ r.name }} <span class="text-muted-foreground">·
                  {{ r.kind === 'reminder' ? t('wl.packs.before', { n: (r.offset_minutes ?? 0) / 60 }) : t('wl.packs.after', { n: r.recall_days }) }}</span></p>
              </section>
              <section v-if="p.flows.length">
                <h3 class="font-medium mb-1">{{ t('wl.packs.flows') }}</h3>
                <p v-for="fl in p.flows" :key="fl.name">{{ fl.name }} <span class="text-muted-foreground">· {{ fl.guide }}</span></p>
              </section>
              <p class="text-muted-foreground">{{ t('wl.packs.connectors') }}: {{ p.connectors.join(', ') }}</p>
            </CardContent>
          </Card>
        </template>
      </div></div>
    </ScrollArea>
  </div>
</template>
