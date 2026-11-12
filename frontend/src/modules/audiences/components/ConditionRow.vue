<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { Condition, Field, Fields } from '../api'
import { FIELDS, OPS, newCondition } from '../stores/audiences'

const props = defineProps<{ fields: Fields | null; index: number; readonly?: boolean }>()
const cond = defineModel<Condition>({ required: true })
defineEmits<{ remove: [] }>()

const { t } = useI18n()
const listId = computed(() => `wl-aud-keys-${props.index}`)
const isDays = computed(() => cond.value.field === 'last_inbound' || cond.value.field === 'created')
const needsValue = computed(() => !(cond.value.field === 'metadata' && ['exists', 'not_exists'].includes(cond.value.op)))
const needsReplyDays = computed(() => cond.value.field === 'campaign' && ['read_no_reply', 'replied'].includes(cond.value.op))

function setField(f: unknown) {
  cond.value = newCondition(f as Field)
}
function setOp(op: unknown) {
  const next: Condition = { ...cond.value, op: String(op) }
  if (next.field === 'metadata' && !['exists', 'not_exists'].includes(next.op) && next.value === undefined) next.value = ''
  cond.value = next
}
function set(part: Partial<Condition>) {
  cond.value = { ...cond.value, ...part }
}
</script>

<template>
  <div class="flex flex-wrap items-center gap-2" :data-testid="`condition-${index}`">
    <Select :model-value="cond.field" :disabled="readonly" @update:model-value="setField">
      <SelectTrigger class="w-44" :aria-label="t('wl.audiences.builder.field')" data-testid="condition-field"><SelectValue /></SelectTrigger>
      <SelectContent>
        <SelectItem v-for="f in FIELDS" :key="f" :value="f">{{ t(`wl.audiences.fields.${f}`) }}</SelectItem>
      </SelectContent>
    </Select>

    <template v-if="cond.field === 'metadata'">
      <Input
        :model-value="cond.key ?? ''"
        :list="listId"
        class="w-40"
        :placeholder="t('wl.audiences.builder.chooseKey')"
        :aria-label="t('wl.audiences.builder.key')"
        :disabled="readonly"
        data-testid="condition-key"
        @update:model-value="v => set({ key: String(v).trim() })"
      />
      <datalist :id="listId">
        <option v-for="k in fields?.metadata_keys ?? []" :key="k" :value="k" />
      </datalist>
    </template>

    <Select :model-value="cond.op" :disabled="readonly" @update:model-value="setOp">
      <SelectTrigger class="w-56" :aria-label="t('wl.audiences.builder.op')" data-testid="condition-op"><SelectValue /></SelectTrigger>
      <SelectContent>
        <SelectItem v-for="op in OPS[cond.field]" :key="op" :value="op">{{ t(`wl.audiences.ops.${op}`) }}</SelectItem>
      </SelectContent>
    </Select>

    <Select
      v-if="cond.field === 'tag' && (fields?.tags.length ?? 0) > 0"
      :model-value="String(cond.value ?? '') || undefined"
      :disabled="readonly"
      @update:model-value="v => set({ value: String(v) })"
    >
      <SelectTrigger class="w-48" :aria-label="t('wl.audiences.builder.value')" data-testid="condition-value">
        <SelectValue :placeholder="t('wl.audiences.builder.chooseTag')" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem v-for="tag in fields?.tags ?? []" :key="tag" :value="tag">{{ tag }}</SelectItem>
      </SelectContent>
    </Select>
    <Select
      v-else-if="cond.field === 'whatsapp_account'"
      :model-value="String(cond.value ?? '') || undefined"
      :disabled="readonly"
      @update:model-value="v => set({ value: String(v) })"
    >
      <SelectTrigger class="w-48" :aria-label="t('wl.audiences.builder.value')" data-testid="condition-value">
        <SelectValue :placeholder="t('wl.audiences.builder.chooseAccount')" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem v-for="a in fields?.accounts ?? []" :key="a" :value="a">{{ a }}</SelectItem>
      </SelectContent>
    </Select>
    <Select
      v-else-if="cond.field === 'campaign'"
      :model-value="String(cond.value ?? '') || undefined"
      :disabled="readonly"
      @update:model-value="v => set({ value: String(v) })"
    >
      <SelectTrigger class="w-56" :aria-label="t('wl.audiences.builder.value')" data-testid="condition-value">
        <SelectValue :placeholder="t('wl.audiences.builder.chooseCampaign')" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem v-for="c in fields?.campaigns ?? []" :key="c.id" :value="c.id">{{ c.name }}</SelectItem>
      </SelectContent>
    </Select>
    <Input
      v-else-if="isDays"
      type="number"
      min="1"
      max="365"
      class="w-24"
      :model-value="cond.value"
      :aria-label="t('wl.audiences.builder.days')"
      :disabled="readonly"
      data-testid="condition-value"
      @update:model-value="v => set({ value: Number(v) })"
    />
    <Input
      v-else-if="needsValue"
      class="w-48"
      :model-value="String(cond.value ?? '')"
      :aria-label="t('wl.audiences.builder.value')"
      :disabled="readonly"
      data-testid="condition-value"
      @update:model-value="v => set({ value: String(v) })"
    />

    <Input
      v-if="needsReplyDays"
      type="number"
      min="1"
      max="365"
      class="w-24"
      :placeholder="'7'"
      :model-value="cond.days"
      :aria-label="t('wl.audiences.retarget.days')"
      :disabled="readonly"
      @update:model-value="v => set({ days: Number(v) || undefined })"
    />

    <Button v-if="!readonly" variant="ghost" size="icon" :aria-label="t('wl.audiences.builder.removeCondition')" @click="$emit('remove')">
      <X class="h-4 w-4" />
    </Button>
  </div>
</template>
