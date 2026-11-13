<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { CreditCard, Lock } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { PageHeader, ErrorState } from '@/components/shared'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { Usage } from '../api'
import { useBillingStore } from '../stores/billing'

const { t } = useI18n()
const route = useRoute()
const auth = useAuthStore()
const store = useBillingStore()
const canBuy = computed(() => auth.hasPermission('wl_billing_plan', 'update'))
const loading = computed(() => store.state === 'loading' || store.state === 'idle')
const s = computed(() => store.summary)
const checkoutResult = computed(() => route.query.checkout as string | undefined)
const day = (iso: string) => new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'long', year: 'numeric' }).format(new Date(iso))
const money = (cents: number, cur: string) => new Intl.NumberFormat(undefined, { style: 'currency', currency: cur.toUpperCase() }).format(cents / 100)
const pct = (u: Usage) => (u.limit > 0 ? Math.min(100, Math.round((u.used / u.limit) * 100)) : u.limit === 0 && u.used > 0 ? 100 : 0)
const banner = computed(() => {
  if (!s.value?.enabled) return ''
  if (s.value.effective_plan === 'none') return s.value.plan === 'trial' ? 'trialOver' : 'ended'
  if (s.value.status === 'past_due') return 'pastDue'
  return ''
})
const paidCurrent = computed(() => !!s.value && s.value.plan !== 'trial' && s.value.effective_plan !== 'none')
const errorText = computed(() =>
  store.actionError === 'forbidden' ? t('wl.billing.actionForbidden') : store.actionError === 'error' ? t('wl.billing.actionFailed') : store.actionError ?? ''
)
async function choose(plan: string) {
  const url = await store.checkout(plan)
  if (url) window.location.assign(url)
}
async function manage() {
  const url = await store.portal()
  if (url) window.location.assign(url)
}
onMounted(() => void store.load())
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="t('wl.billing.title')" :description="t('wl.billing.subtitle')" :icon="CreditCard"
      icon-gradient="bg-gradient-to-br from-violet-500 to-fuchsia-600 shadow-violet-500/20" />
    <EmptyState v-if="store.state === 'forbidden'" data-testid="billing-forbidden" :icon="Lock" :title="t('wl.billing.forbiddenTitle')"
      :description="t('wl.billing.forbiddenDescription')" class="flex-1" />
    <ErrorState v-else-if="store.state === 'error'" :title="t('common.loadErrorTitle')" :description="t('common.loadErrorDescription')"
      :retry-label="t('common.retryLoad')" class="flex-1" @retry="store.load()" />
    <ScrollArea v-else class="flex-1">
      <div class="p-6"><div class="max-w-4xl mx-auto space-y-6">
        <div v-if="loading" class="space-y-3" data-testid="billing-loading"><Skeleton v-for="i in 4" :key="i" class="h-20 w-full" /></div>
        <template v-else-if="s">
          <p v-if="checkoutResult === 'done'" class="text-sm text-emerald-500" data-testid="billing-checkout-done">{{ t('wl.billing.checkoutDone') }}</p>
          <p v-else-if="checkoutResult === 'cancelled'" class="text-sm text-muted-foreground">{{ t('wl.billing.checkoutCancelled') }}</p>

          <Card v-if="!s.enabled" data-testid="billing-disabled">
            <CardHeader><CardTitle class="text-base">{{ t('wl.billing.disabledTitle') }}</CardTitle>
              <CardDescription>{{ t('wl.billing.disabledDescription') }}</CardDescription></CardHeader>
          </Card>
          <template v-else>
            <p v-if="banner" role="alert" class="rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm" :data-testid="`billing-banner-${banner}`">
              {{ t(`wl.billing.${banner}`) }}</p>
            <Card data-testid="billing-current">
              <CardContent class="p-4 flex flex-wrap items-center justify-between gap-3">
                <div>
                  <p class="text-sm text-muted-foreground">{{ t('wl.billing.current') }}</p>
                  <p class="text-lg font-semibold">
                    <span data-testid="billing-plan">{{ t(`wl.billing.plans.${s.effective_plan}`) }}</span>
                    <Badge variant="secondary" class="ml-2">{{ t(`wl.billing.statuses.${s.status}`) }}</Badge>
                  </p>
                  <p class="text-sm text-muted-foreground">
                    <span v-if="s.trial_ends_at && s.effective_plan === 'trial'" data-testid="billing-trial-ends">{{ t('wl.billing.trialEnds', { date: day(s.trial_ends_at) }) }}</span>
                    <span v-else-if="s.current_period_end && s.cancel_at_period_end">{{ t('wl.billing.cancels', { date: day(s.current_period_end) }) }}</span>
                    <span v-else-if="s.current_period_end">{{ t('wl.billing.renews', { date: day(s.current_period_end) }) }}</span>
                  </p>
                </div>
                <div v-if="s.has_customer && canBuy" class="text-right">
                  <Button :disabled="store.busy" data-testid="billing-manage" @click="manage">{{ t('wl.billing.manage') }}</Button>
                  <p class="text-xs text-muted-foreground mt-1 max-w-xs">{{ t('wl.billing.manageHelp') }}</p>
                </div>
              </CardContent>
            </Card>
            <p v-if="!s.stripe_ready" class="text-sm text-muted-foreground" data-testid="billing-stripe-not-ready">{{ t('wl.billing.stripeNotReady') }}</p>

            <div v-if="s.plans?.length" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <Card v-for="p in s.plans" :key="p.key" :data-testid="`billing-plan-${p.key}`" :class="p.key === s.effective_plan ? 'border-primary' : ''">
                <CardHeader class="pb-2"><CardTitle class="text-base">{{ t(`wl.billing.plans.${p.key}`) }}
                  <Badge v-if="p.key === s.effective_plan" class="ml-1">{{ t('wl.billing.currentBadge') }}</Badge></CardTitle></CardHeader>
                <CardContent class="space-y-1 text-sm">
                  <p v-for="(n, m) in p.limits" :key="m" class="flex justify-between gap-2">
                    <span class="text-muted-foreground">{{ t(`wl.billing.metrics.${m}`) }}</span><span>{{ n.toLocaleString() }}</span></p>
                  <template v-if="p.key !== 'trial' && canBuy && !paidCurrent">
                    <Button v-if="p.available && s.stripe_ready" size="sm" class="w-full mt-3" :disabled="store.busy" :data-testid="`billing-choose-${p.key}`"
                      @click="choose(p.key)">{{ t('wl.billing.choose', { plan: t(`wl.billing.plans.${p.key}`) }) }}</Button>
                    <p v-else class="text-xs text-muted-foreground mt-3">{{ t('wl.billing.notOnline') }}</p>
                  </template>
                </CardContent>
              </Card>
            </div>
          </template>
          <p v-if="errorText" class="text-sm text-destructive" role="alert" data-testid="billing-error">{{ errorText }}</p>

          <Card data-testid="billing-usage">
            <CardHeader><CardTitle class="text-base">{{ t('wl.billing.usage') }}</CardTitle>
              <CardDescription>{{ t('wl.billing.neverLimited') }}</CardDescription></CardHeader>
            <CardContent class="space-y-4">
              <div v-for="u in s.usage" :key="u.metric" :data-testid="`billing-usage-${u.metric}`">
                <div class="flex justify-between text-sm">
                  <span>{{ t(`wl.billing.metrics.${u.metric}`) }} <span v-if="u.monthly" class="text-muted-foreground">({{ t('wl.billing.perMonth') }})</span></span>
                  <span>{{ u.used.toLocaleString() }} / {{ u.limit < 0 ? t('wl.billing.unlimited') : u.limit.toLocaleString() }}</span>
                </div>
                <div v-if="u.limit >= 0" class="h-1.5 mt-1 rounded-full bg-muted overflow-hidden">
                  <div class="h-full" :class="pct(u) >= 100 ? 'bg-destructive' : pct(u) >= 80 ? 'bg-amber-500' : 'bg-primary'" :style="{ width: `${pct(u)}%` }" />
                </div>
                <p v-if="u.limit >= 0 && !u.enforced" class="text-xs text-muted-foreground mt-0.5">{{ t('wl.billing.softLimit') }}</p>
              </div>
            </CardContent>
          </Card>

          <Card data-testid="billing-meta">
            <CardHeader><CardTitle class="text-base">{{ t('wl.billing.meta') }}</CardTitle>
              <CardDescription>{{ t('wl.billing.metaHelp') }}</CardDescription></CardHeader>
            <CardContent class="text-sm space-y-1">
              <p class="font-medium">{{ t('wl.billing.metaCounts') }}</p>
              <p v-if="Object.keys(s.meta.counts).length === 0" class="text-muted-foreground">0</p>
              <p v-for="(n, c) in s.meta.counts" :key="c" class="flex justify-between max-w-xs">
                <span>{{ $te(`wl.billing.metaCategories.${c}`) ? t(`wl.billing.metaCategories.${c}`) : c }}</span><span>{{ n }}</span></p>
              <a :href="s.meta.pricing_url" target="_blank" rel="noopener" class="text-primary underline">{{ t('wl.billing.metaPricing') }}</a>
            </CardContent>
          </Card>

          <Card v-if="s.enabled">
            <CardHeader><CardTitle class="text-base">{{ t('wl.billing.invoices') }}</CardTitle></CardHeader>
            <CardContent :class="store.invoices.length ? 'p-0' : ''">
              <p v-if="store.invoices.length === 0" class="text-sm text-muted-foreground" data-testid="billing-invoices-empty">{{ t('wl.billing.invoicesEmpty') }}</p>
              <Table v-else data-testid="billing-invoices">
                <TableHeader><TableRow><TableHead>{{ t('wl.billing.invoice') }}</TableHead><TableHead>{{ t('wl.billing.date') }}</TableHead>
                  <TableHead>{{ t('wl.billing.amount') }}</TableHead><TableHead>{{ t('wl.billing.status') }}</TableHead><TableHead /></TableRow></TableHeader>
                <TableBody><TableRow v-for="i in store.invoices" :key="i.id">
                  <TableCell>{{ i.number }}</TableCell><TableCell>{{ day(i.created) }}</TableCell>
                  <TableCell>{{ money(i.total, i.currency) }}</TableCell><TableCell>{{ i.status }}</TableCell>
                  <TableCell class="text-right"><a v-if="i.hosted_url" :href="i.hosted_url" target="_blank" rel="noopener" class="text-primary underline">{{ t('wl.billing.view') }}</a></TableCell>
                </TableRow></TableBody>
              </Table>
            </CardContent>
          </Card>
        </template>
      </div></div>
    </ScrollArea>
  </div>
</template>
