<script setup lang="ts">
import { computed, onMounted, reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { useAuthStore } from '@/stores/auth'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import ConsentShell from '../components/ConsentShell.vue'
import KeywordList from '../components/KeywordList.vue'
import SaveError from '../components/SaveError.vue'
import { useConsentStore } from '../stores/consent'
import type { Settings } from '../api'

const { t } = useI18n()
const auth = useAuthStore()
const store = useConsentStore()
const canEdit = computed(() => auth.hasPermission('wl_consent_settings', 'update'))

const form = reactive<Settings>({
  opt_out_keywords: [], opt_in_keywords: [], opt_in_phrases: [], opt_out_reply: '', opt_in_reply: '',
  opt_out_tag: '', require_opt_in: false, service_after_stop: 'continue'
})
watch(() => store.settings, s => {
  if (s) Object.assign(form, JSON.parse(JSON.stringify(s)))
}, { immediate: true })

async function save() {
  if (await store.saveSettings({ ...form })) toast.success(t('wl.consent.saved'))
}
onMounted(() => {
  store.saveError = null
  void store.loadSettings()
})
</script>

<template>
  <ConsentShell :state="store.state.settings" @retry="store.loadSettings">
    <p v-if="!canEdit" class="text-sm text-muted-foreground" data-testid="consent-settings-readonly">{{ t('wl.consent.settings.readOnly') }}</p>

    <Card>
      <CardHeader>
        <CardTitle>{{ t('wl.consent.settings.keywords') }}</CardTitle>
        <CardDescription>{{ t('wl.consent.settings.keywordsHelp') }}</CardDescription>
      </CardHeader>
      <CardContent class="space-y-5">
        <KeywordList
          v-model="form.opt_out_keywords"
          :label="t('wl.consent.settings.optOut')"
          testid="kw-out"
          :taken="[...form.opt_in_keywords, ...form.opt_in_phrases]"
          :readonly="!canEdit"
        />
        <KeywordList v-model="form.opt_in_keywords" :label="t('wl.consent.settings.optIn')" testid="kw-in" :taken="form.opt_out_keywords" :readonly="!canEdit" />
        <div class="space-y-1">
          <KeywordList v-model="form.opt_in_phrases" :label="t('wl.consent.settings.phrases')" testid="kw-phrases" :taken="form.opt_out_keywords" :readonly="!canEdit" />
          <p class="text-xs text-muted-foreground">{{ t('wl.consent.settings.phrasesHelp') }}</p>
        </div>
      </CardContent>
    </Card>

    <Card>
      <CardHeader>
        <CardTitle>{{ t('wl.consent.settings.replies') }}</CardTitle>
        <CardDescription>{{ t('wl.consent.settings.businessHint', { business: '{business}' }) }}</CardDescription>
      </CardHeader>
      <CardContent class="space-y-4">
        <div class="space-y-1">
          <Label for="wl-consent-out-reply">{{ t('wl.consent.settings.optOutReply') }}</Label>
          <Textarea id="wl-consent-out-reply" v-model="form.opt_out_reply" :rows="3" maxlength="500" :disabled="!canEdit" />
        </div>
        <div class="space-y-1">
          <Label for="wl-consent-in-reply">{{ t('wl.consent.settings.optInReply') }}</Label>
          <Textarea id="wl-consent-in-reply" v-model="form.opt_in_reply" :rows="3" maxlength="500" :disabled="!canEdit" />
        </div>
        <div class="space-y-1">
          <Label for="wl-consent-tag">{{ t('wl.consent.settings.tag') }}</Label>
          <Input id="wl-consent-tag" v-model="form.opt_out_tag" class="w-64" maxlength="50" :disabled="!canEdit" />
        </div>
      </CardContent>
    </Card>

    <Card>
      <CardHeader><CardTitle>{{ t('wl.consent.settings.policies') }}</CardTitle></CardHeader>
      <CardContent class="space-y-5">
        <div class="flex items-start gap-3">
          <Switch id="wl-consent-require" v-model:checked="form.require_opt_in" :disabled="!canEdit" data-testid="consent-require-opt-in" />
          <div>
            <Label for="wl-consent-require">{{ t('wl.consent.settings.requireOptIn') }}</Label>
            <p class="text-xs text-muted-foreground">{{ t('wl.consent.settings.requireOptInHelp') }}</p>
          </div>
        </div>
        <div class="space-y-2">
          <p class="text-sm font-medium">{{ t('wl.consent.settings.serviceAfterStop') }}</p>
          <RadioGroup v-model="form.service_after_stop" :disabled="!canEdit">
            <div class="flex items-center gap-2">
              <RadioGroupItem id="wl-consent-svc-continue" value="continue" />
              <Label for="wl-consent-svc-continue" class="font-normal">{{ t('wl.consent.settings.serviceContinue') }}</Label>
            </div>
            <div class="flex items-center gap-2">
              <RadioGroupItem id="wl-consent-svc-stop" value="stop" />
              <Label for="wl-consent-svc-stop" class="font-normal">{{ t('wl.consent.settings.serviceStop') }}</Label>
            </div>
          </RadioGroup>
        </div>
      </CardContent>
    </Card>

    <Card>
      <CardHeader><CardTitle>{{ t('wl.consent.settings.tipsTitle') }}</CardTitle></CardHeader>
      <CardContent>
        <ul class="list-disc pl-5 text-sm text-muted-foreground space-y-1">
          <li>{{ t('wl.consent.settings.tipFooter') }}</li>
          <li>{{ t('wl.consent.settings.tipChatbot') }}</li>
        </ul>
      </CardContent>
    </Card>

    <SaveError />
    <Button v-if="canEdit" :disabled="store.saving" data-testid="consent-settings-save" @click="save">{{ t('wl.consent.save') }}</Button>
  </ConsentShell>
</template>
