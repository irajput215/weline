<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { CheckCircle2, Circle, Lock, Rocket } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { PageHeader, ErrorState } from '@/components/shared'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { verticals, type Vertical } from '../api'
import { useOnboardingStore } from '../stores/onboarding'

const { t } = useI18n()
const auth = useAuthStore()
const store = useOnboardingStore()
const canEdit = computed(() => auth.hasPermission('wl_onboarding_setup', 'update'))
const loading = computed(() => store.state === 'loading' || store.state === 'idle')
const c = computed(() => store.checklist)
const pct = computed(() => (c.value && c.value.total ? Math.round((c.value.done / c.value.total) * 100) : 0))
const errorText = computed(() =>
  store.saveError === 'forbidden' ? t('wl.onboarding.saveForbidden') : store.saveError === 'error' ? t('wl.onboarding.saveFailed') : store.saveError ?? ''
)
const vertical = computed({
  get: () => c.value?.vertical || undefined,
  set: (v: string | undefined) => { if (v) void store.setVertical(v as Vertical) }
})
onMounted(() => void store.load())
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="t('wl.onboarding.title')" :description="t('wl.onboarding.subtitle')" :icon="Rocket"
      icon-gradient="bg-gradient-to-br from-sky-500 to-indigo-600 shadow-sky-500/20" />
    <EmptyState v-if="store.state === 'forbidden'" data-testid="onboarding-forbidden" :icon="Lock" :title="t('wl.onboarding.forbiddenTitle')"
      :description="t('wl.onboarding.forbiddenDescription')" class="flex-1" />
    <ErrorState v-else-if="store.state === 'error'" :title="t('common.loadErrorTitle')" :description="t('common.loadErrorDescription')"
      :retry-label="t('common.retryLoad')" class="flex-1" @retry="store.load()" />
    <ScrollArea v-else class="flex-1">
      <div class="p-6"><div class="max-w-3xl mx-auto space-y-6">
        <div v-if="loading" class="space-y-3" data-testid="onboarding-loading"><Skeleton v-for="i in 4" :key="i" class="h-16 w-full" /></div>
        <template v-else-if="c">
          <div class="space-y-2">
            <div class="flex items-center justify-between text-sm">
              <span data-testid="onboarding-progress">{{ t('wl.onboarding.progress', { done: c.done, total: c.total }) }}</span>
              <Button v-if="canEdit" variant="ghost" size="sm" data-testid="onboarding-dismiss" @click="store.setDismissed(!c.dismissed)">
                {{ c.dismissed ? t('wl.onboarding.show') : t('wl.onboarding.hide') }}</Button>
            </div>
            <div class="h-2 rounded-full bg-muted overflow-hidden" role="progressbar" :aria-valuenow="pct" aria-valuemin="0" aria-valuemax="100">
              <div class="h-full bg-primary transition-all" :style="{ width: `${pct}%` }" />
            </div>
            <p v-if="c.done === c.total" class="text-sm text-emerald-500" data-testid="onboarding-all-done">{{ t('wl.onboarding.allDone') }}</p>
            <p v-else-if="c.dismissed" class="text-sm text-muted-foreground" data-testid="onboarding-hidden">{{ t('wl.onboarding.hidden') }}</p>
          </div>
          <EmptyState v-if="c.steps.length === 0" :icon="Rocket" :title="t('wl.onboarding.allDone')" />
          <Card v-for="s in c.steps" :key="s.key" :data-testid="`onboarding-step-${s.key}`" :data-done="s.done">
            <CardContent class="p-4 flex items-start gap-3">
              <CheckCircle2 v-if="s.done" class="h-5 w-5 text-emerald-500 shrink-0 mt-0.5" :aria-label="t('wl.onboarding.done')" />
              <Circle v-else class="h-5 w-5 text-muted-foreground shrink-0 mt-0.5" />
              <div class="flex-1 space-y-2">
                <div>
                  <p class="font-medium">{{ t(`wl.onboarding.steps.${s.key}.title`) }}</p>
                  <p class="text-sm text-muted-foreground">{{ t(`wl.onboarding.steps.${s.key}.help`) }}</p>
                </div>
                <div v-if="s.key === 'vertical'" class="max-w-xs space-y-1">
                  <Label class="sr-only">{{ t('wl.onboarding.vertical') }}</Label>
                  <Select v-model="vertical" :disabled="!canEdit || store.saving">
                    <SelectTrigger :aria-label="t('wl.onboarding.vertical')" data-testid="onboarding-vertical"><SelectValue :placeholder="t('wl.onboarding.vertical')" /></SelectTrigger>
                    <SelectContent><SelectItem v-for="v in verticals" :key="v" :value="v">{{ t(`wl.onboarding.verticals.${v}`) }}</SelectItem></SelectContent>
                  </Select>
                </div>
                <p v-if="s.key === 'whatsapp'" class="text-xs text-muted-foreground">{{ t('wl.onboarding.metaCharges') }}</p>
              </div>
              <Button v-if="s.key !== 'vertical'" as-child variant="outline" size="sm">
                <RouterLink :to="s.link" :data-testid="`onboarding-go-${s.key}`">{{ t('wl.onboarding.go') }}</RouterLink>
              </Button>
            </CardContent>
          </Card>
        </template>
        <p v-if="errorText" class="text-sm text-destructive" role="alert" data-testid="onboarding-error">{{ errorText }}</p>
      </div></div>
    </ScrollArea>
  </div>
</template>
