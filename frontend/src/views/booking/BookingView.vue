<script setup lang="ts">
// Weline-only view. Placeholder that proves the backend wiring end to end.
import { ref, onMounted } from 'vue'
import { CalendarDays } from 'lucide-vue-next'
import { Card, CardContent } from '@/components/ui/card'
import { PageHeader } from '@/components/shared'
import { bookingService, type Appointment } from '@/services/weline/booking'

const appointments = ref<Appointment[]>([])
const isLoading = ref(true)
const error = ref<string | null>(null)

onMounted(async () => {
  try {
    const response = await bookingService.listAppointments()
    appointments.value = response.data.data?.appointments ?? []
  } catch {
    error.value = 'Could not load appointments.'
  } finally {
    isLoading.value = false
  }
})
</script>

<template>
  <div class="flex flex-col h-full">
    <PageHeader title="Booking" description="Clinic appointments" :icon="CalendarDays" />
    <div class="flex-1 overflow-auto p-6">
      <Card>
        <CardContent class="p-6">
          <p v-if="isLoading" class="text-sm text-muted-foreground">Loading…</p>
          <p v-else-if="error" class="text-sm text-destructive">{{ error }}</p>
          <p v-else-if="appointments.length === 0" class="text-sm text-muted-foreground">No appointments yet.</p>
          <ul v-else class="space-y-2">
            <li v-for="a in appointments" :key="a.id" class="text-sm">
              {{ new Date(a.starts_at).toLocaleString() }} · {{ a.status }}
            </li>
          </ul>
        </CardContent>
      </Card>
    </div>
  </div>
</template>
