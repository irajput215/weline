<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { PERIODS, useAdsStore } from '../stores/ads'

// The last 7, 30 or 90 days, or custom dates; changes reload through the parent.
const emit = defineEmits<{ change: [] }>()
const { t } = useI18n()
const store = useAdsStore()

function setDays(v: unknown) {
  store.days = Number(v)
  if (store.days !== 0) emit('change')
}
</script>

<template>
  <div class="flex flex-wrap items-center gap-2">
    <Select :model-value="String(store.days)" @update:model-value="setDays">
      <SelectTrigger class="w-44" :aria-label="t('wl.ads.period.label')" data-testid="ads-period"><SelectValue /></SelectTrigger>
      <SelectContent>
        <SelectItem v-for="d in PERIODS" :key="d" :value="String(d)">{{ t('wl.ads.period.days', { days: d }) }}</SelectItem>
        <SelectItem value="0">{{ t('wl.ads.period.custom') }}</SelectItem>
      </SelectContent>
    </Select>
    <template v-if="store.days === 0">
      <Input v-model="store.range.from" type="date" class="w-40" :aria-label="t('wl.ads.period.from')" @change="emit('change')" />
      <Input v-model="store.range.to" type="date" class="w-40" :aria-label="t('wl.ads.period.to')" @change="emit('change')" />
    </template>
  </div>
</template>
