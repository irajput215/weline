<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Bot, CalendarClock, History, Lock, Megaphone, MessageCircle, Send, ShieldCheck, UserRound, Bell, MousePointerClick } from 'lucide-vue-next'
import { PageHeader, ErrorState } from '@/components/shared'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import type { Item, Kind } from '../api'
import { useTimelineStore } from '../stores/timeline'

const { t, te } = useI18n()
const route = useRoute()
const router = useRouter()
const store = useTimelineStore()
const q = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
const tl = computed(() => store.timeline)
const icons: Record<Kind, unknown> = { message_in: MessageCircle, message_out: Send, broadcast: Megaphone, appointment: CalendarClock,
  reminder: Bell, consent: ShieldCheck, ad_arrival: MousePointerClick, ai: Bot, handover: UserRound }
const when = (iso: string) => new Intl.DateTimeFormat(undefined, { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric', hour: 'numeric', minute: '2-digit' }).format(new Date(iso))
const day = (iso: string) => new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'long', year: 'numeric' }).format(new Date(iso))
const lbl = (key: string, v: string) => (te(key) ? t(key) : v)

/** One line describing the item, from its administrative fields only. */
function describe(it: Item): string {
  const f = it.fields
  switch (it.kind) {
    case 'message_in':
    case 'message_out':
      return [f.type, f.status].filter(Boolean).join(' · ')
    case 'broadcast':
      return [f.campaign, f.status].filter(Boolean).join(' · ')
    case 'appointment':
      return [f.service, f.practitioner, f.location, f.status].filter(Boolean).join(' · ')
    case 'reminder':
      return [f.rule, f.status, f.reason, f.reply ? lbl(`wl.timeline.reply.${f.reply}`, f.reply) : ''].filter(Boolean).join(' · ')
    case 'consent':
      return [lbl(`wl.timeline.consent.${f.action}`, f.action), f.source].filter(Boolean).join(' · ')
    case 'ad_arrival':
      return [f.entry_point, f.channel].filter(Boolean).join(' · ')
    case 'ai':
      return [f.intent, f.outcome].filter(Boolean).join(' · ')
    case 'handover':
      return [f.status, f.source].filter(Boolean).join(' · ')
  }
  return ''
}

function onSearch() {
  clearTimeout(timer)
  timer = setTimeout(() => void store.search(q.value), 300)
}
function pick(id: string) {
  void router.replace({ query: { contact: id } })
}
watch(() => route.query.contact, id => { if (typeof id === 'string' && id) void store.open(id) })
onMounted(() => {
  const id = route.query.contact
  if (typeof id === 'string' && id) void store.open(id)
})
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader :title="t('wl.timeline.title')" :description="t('wl.timeline.subtitle')" :icon="History"
      icon-gradient="bg-gradient-to-br from-amber-500 to-rose-600 shadow-amber-500/20" />
    <EmptyState v-if="store.state === 'forbidden' || store.searchState === 'forbidden'" data-testid="timeline-forbidden" :icon="Lock"
      :title="t('wl.timeline.forbiddenTitle')" :description="t('wl.timeline.forbiddenDescription')" class="flex-1" />
    <ScrollArea v-else class="flex-1">
      <div class="p-6"><div class="max-w-3xl mx-auto space-y-6">
        <div class="space-y-2">
          <Input v-model="q" :placeholder="t('wl.timeline.search')" maxlength="100" data-testid="timeline-search" @input="onSearch" />
          <p v-if="store.searchState === 'ready' && store.results.length === 0" class="text-sm text-muted-foreground" data-testid="timeline-search-empty">
            {{ t('wl.timeline.searchEmpty') }}</p>
          <div v-if="store.results.length" class="flex flex-wrap gap-2">
            <Button v-for="c in store.results" :key="c.id" variant="outline" size="sm" :data-testid="`timeline-result-${c.name}`" @click="pick(c.id)">
              {{ c.name || c.phone_masked }} <span class="text-muted-foreground ml-1">{{ c.phone_masked }}</span></Button>
          </div>
        </div>

        <p v-if="store.state === 'idle'" class="text-sm text-muted-foreground" data-testid="timeline-pick">{{ t('wl.timeline.pick') }}</p>
        <div v-else-if="store.state === 'loading'" class="space-y-3" data-testid="timeline-loading"><Skeleton v-for="i in 5" :key="i" class="h-12 w-full" /></div>
        <ErrorState v-else-if="store.state === 'error'" :title="t('common.loadErrorTitle')" :description="t('common.loadErrorDescription')"
          :retry-label="t('common.retryLoad')" @retry="store.open(String(route.query.contact))" />
        <p v-else-if="store.state === 'notfound'" class="text-sm text-destructive" data-testid="timeline-notfound">{{ t('wl.timeline.notFound') }}</p>
        <template v-else-if="tl">
          <Card><CardContent class="p-4 flex items-center justify-between gap-3">
            <div>
              <p class="font-semibold" data-testid="timeline-contact">{{ tl.contact.name || tl.contact.phone_masked }}</p>
              <p class="text-sm text-muted-foreground">{{ tl.contact.phone_masked }} · {{ t('wl.timeline.since', { date: day(tl.contact.created_at) }) }}</p>
            </div>
            <Button as-child variant="outline" size="sm"><RouterLink :to="`/chat/${tl.contact.id}`">{{ t('wl.timeline.openChat') }}</RouterLink></Button>
          </CardContent></Card>
          <p class="text-xs text-muted-foreground">{{ t('wl.timeline.privacy') }}</p>
          <p v-if="tl.appointments_unavailable" class="text-sm text-amber-500" role="status" data-testid="timeline-appointments-unavailable">
            {{ t('wl.timeline.appointmentsUnavailable') }}</p>
          <EmptyState v-if="tl.items.length === 0" data-testid="timeline-empty" :icon="History" :title="t('wl.timeline.empty')" />
          <ol v-else class="relative border-l border-border ml-3 space-y-4" data-testid="timeline-items">
            <li v-for="(it, i) in tl.items" :key="i" class="ml-6" :data-testid="`timeline-item-${it.kind}`">
              <span class="absolute -left-3 flex h-6 w-6 items-center justify-center rounded-full bg-muted">
                <component :is="icons[it.kind]" class="h-3.5 w-3.5" /></span>
              <p class="text-sm"><Badge variant="secondary">{{ t(`wl.timeline.kinds.${it.kind}`) }}</Badge>
                <span class="ml-2 text-muted-foreground">{{ when(it.at) }}</span></p>
              <p class="text-sm mt-1">{{ describe(it) }}</p>
            </li>
          </ol>
          <div v-if="tl.next_before" class="text-center">
            <Button variant="ghost" :disabled="store.loadingMore" data-testid="timeline-more" @click="store.more()">{{ t('wl.timeline.more') }}</Button></div>
        </template>
      </div></div>
    </ScrollArea>
  </div>
</template>
