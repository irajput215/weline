<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { CheckCircle2, Circle, Copy, ExternalLink } from 'lucide-vue-next'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import BookingShell from '../components/BookingShell.vue'
import { useBookingStore } from '../stores/booking'

const { t } = useI18n()
const store = useBookingStore()

const steps = ['profile', 'services', 'availability', 'captcha']
const link = computed(() => (store.ready?.path ? `${window.location.origin}${store.ready.path}` : ''))

async function copy() {
  try {
    await navigator.clipboard.writeText(link.value)
    toast.success(t('wl.booking.online.copied'))
  } catch {
    toast.error(t('wl.booking.online.copyFailed'))
  }
}

onMounted(store.loadSettings)
</script>

<template>
  <BookingShell :state="store.state.settings" @retry="store.loadSettings">
    <Card>
      <CardHeader>
        <CardTitle>{{ t('wl.booking.online.title') }}</CardTitle>
        <CardDescription>{{ t('wl.booking.online.description') }}</CardDescription>
      </CardHeader>
      <CardContent class="space-y-4">
        <div v-if="link" class="flex gap-2">
          <Input :model-value="link" readonly :aria-label="t('wl.booking.online.link')" data-testid="online-link" />
          <Button variant="outline" size="icon" :aria-label="t('wl.booking.online.copy')" @click="copy"><Copy class="h-4 w-4" /></Button>
          <Button as="a" :href="link" target="_blank" rel="noopener" variant="outline" size="icon" :aria-label="t('wl.booking.online.open')"><ExternalLink class="h-4 w-4" /></Button>
        </div>
        <p v-if="store.ready?.ok" class="text-sm text-emerald-600" data-testid="online-ready">{{ t('wl.booking.online.ready') }}</p>
        <template v-else>
          <p class="text-sm text-muted-foreground" data-testid="online-not-ready">{{ t('wl.booking.online.notReady') }}</p>
          <ul class="space-y-2">
            <li v-for="s in steps" :key="s" class="flex items-center gap-2 text-sm">
              <Circle v-if="store.ready?.missing.includes(s)" class="h-4 w-4 text-muted-foreground" />
              <CheckCircle2 v-else class="h-4 w-4 text-emerald-500" />
              {{ t(`wl.booking.online.steps.${s}`) }}
            </li>
          </ul>
        </template>
      </CardContent>
    </Card>
  </BookingShell>
</template>
