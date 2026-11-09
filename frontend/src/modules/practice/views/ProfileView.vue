<script setup lang="ts">
import { computed, onMounted, reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { CheckCircle2, Circle } from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import PracticeShell from '../components/PracticeShell.vue'
import { usePracticeStore } from '../stores/practice'

const { t } = useI18n()
const store = usePracticeStore()
const auth = useAuthStore()
const canEdit = computed(() => auth.hasPermission('wl_practice_settings', 'update'))

const form = reactive({
  name: '', time_zone: 'Australia/Sydney', booking_slug: '', slot_minutes: 15,
  abn: '', phone: '', email: '', website: ''
})

watch(
  () => store.profile,
  p => {
    if (p) Object.assign(form, { ...p })
  },
  { immediate: true }
)

const steps = ['profile', 'locations', 'hours', 'practitioners', 'services']
const stepLink: Record<string, string> = {
  profile: '/x/practice/profile', locations: '/x/practice/locations', hours: '/x/practice/hours',
  practitioners: '/x/practice/practitioners', services: '/x/practice/services'
}
const bookingLink = computed(() => `${window.location.origin}/book/${form.booking_slug || '…'}`)

async function submit() {
  if (await store.saveProfile({ ...form, slot_minutes: Number(form.slot_minutes) })) {
    toast.success(t('wl.practice.saved'))
  }
}

onMounted(store.loadProfile)
</script>

<template>
  <PracticeShell :state="store.state.profile" @retry="store.loadProfile">
    <Card v-if="store.setupMissing.length > 0" data-testid="practice-setup">
      <CardHeader>
        <CardTitle>{{ t('wl.practice.setup.title') }}</CardTitle>
        <CardDescription>{{ t('wl.practice.setup.description') }}</CardDescription>
      </CardHeader>
      <CardContent>
        <ul class="space-y-2">
          <li v-for="step in steps" :key="step" class="flex items-center gap-2 text-sm">
            <Circle v-if="store.setupMissing.includes(step)" class="h-4 w-4 text-muted-foreground" />
            <CheckCircle2 v-else class="h-4 w-4 text-emerald-500" />
            <RouterLink :to="stepLink[step]" class="hover:underline">{{ t(`wl.practice.setup.${step}`) }}</RouterLink>
          </li>
        </ul>
      </CardContent>
    </Card>

    <Card>
      <CardHeader>
        <CardTitle>{{ t('wl.practice.profile.title') }}</CardTitle>
        <CardDescription v-if="!store.profile">{{ t('wl.practice.profile.intro') }}</CardDescription>
      </CardHeader>
      <CardContent>
        <form class="grid gap-4 md:grid-cols-2" @submit.prevent="submit">
          <div class="grid gap-1.5 md:col-span-2">
            <Label for="p-name">{{ t('wl.practice.profile.name') }}</Label>
            <Input id="p-name" v-model="form.name" :disabled="!canEdit" maxlength="120" data-testid="profile-name" />
          </div>
          <div class="grid gap-1.5">
            <Label>{{ t('wl.practice.profile.timeZone') }}</Label>
            <Select v-model="form.time_zone" :disabled="!canEdit">
              <SelectTrigger data-testid="profile-tz"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem v-for="tz in store.timeZones" :key="tz" :value="tz">{{ tz.replace('Australia/', '') }}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div class="grid gap-1.5">
            <Label>{{ t('wl.practice.profile.slot') }}</Label>
            <Select :model-value="String(form.slot_minutes)" :disabled="!canEdit" @update:model-value="v => (form.slot_minutes = Number(v))">
              <SelectTrigger data-testid="profile-slot"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem v-for="s in store.slotLengths" :key="s" :value="String(s)">{{ t('wl.practice.minutes', { n: s }) }}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div class="grid gap-1.5 md:col-span-2">
            <Label for="p-slug">{{ t('wl.practice.profile.slug') }}</Label>
            <Input id="p-slug" v-model="form.booking_slug" :disabled="!canEdit" maxlength="40" data-testid="profile-slug" />
            <p class="text-xs text-muted-foreground" data-testid="profile-slug-preview">{{ t('wl.practice.profile.slugHint', { link: bookingLink }) }}</p>
          </div>
          <div class="grid gap-1.5">
            <Label for="p-abn">{{ t('wl.practice.profile.abn') }}</Label>
            <Input id="p-abn" v-model="form.abn" :disabled="!canEdit" maxlength="14" />
          </div>
          <div class="grid gap-1.5">
            <Label for="p-phone">{{ t('wl.practice.profile.phone') }}</Label>
            <Input id="p-phone" v-model="form.phone" :disabled="!canEdit" maxlength="40" />
          </div>
          <div class="grid gap-1.5">
            <Label for="p-email">{{ t('wl.practice.profile.email') }}</Label>
            <Input id="p-email" v-model="form.email" type="email" :disabled="!canEdit" maxlength="254" />
          </div>
          <div class="grid gap-1.5">
            <Label for="p-web">{{ t('wl.practice.profile.website') }}</Label>
            <Input id="p-web" v-model="form.website" :disabled="!canEdit" maxlength="255" />
          </div>
          <p v-if="store.saveError" class="text-sm text-destructive md:col-span-2" role="alert" data-testid="practice-save-error">
            {{ store.saveError === 'forbidden' ? t('wl.practice.saveForbidden') : store.saveError }}
          </p>
          <div v-if="canEdit" class="md:col-span-2">
            <Button type="submit" :disabled="store.saving" data-testid="profile-save">{{ t('wl.practice.save') }}</Button>
          </div>
          <p v-else class="text-sm text-muted-foreground md:col-span-2" data-testid="practice-readonly">{{ t('wl.practice.readOnly') }}</p>
        </form>
      </CardContent>
    </Card>
  </PracticeShell>
</template>
