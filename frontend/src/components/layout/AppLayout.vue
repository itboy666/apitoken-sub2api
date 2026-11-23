<template>
  <div class="app-shell relative min-h-screen overflow-x-clip bg-gray-50 text-gray-900 transition-colors dark:bg-dark-950 dark:text-gray-100">
    <!-- Background Decoration -->
    <div
      aria-hidden="true"
      class="pointer-events-none fixed inset-0 z-0 bg-mesh-gradient opacity-75 dark:opacity-50"
    ></div>
    <div
      aria-hidden="true"
      class="pointer-events-none fixed inset-0 z-0 bg-[linear-gradient(180deg,rgba(255,255,255,0.32)_0%,transparent_28%,rgba(15,23,42,0.02)_100%)] dark:bg-[linear-gradient(180deg,rgba(15,23,42,0.12)_0%,transparent_34%,rgba(2,6,23,0.22)_100%)]"
    ></div>

    <!-- Sidebar -->
    <AppSidebar />

    <!-- Main Content Area -->
    <div
      class="relative z-10 min-h-screen min-w-0 transition-[margin] duration-300 ease-out"
      :class="[sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64']"
    >
      <!-- Header -->
      <AppHeader />

      <!-- Main Content -->
      <main class="mx-auto w-full max-w-[1680px] p-4 sm:p-5 md:p-6 lg:p-8">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'

const appStore = useAppStore()
const authStore = useAuthStore()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')

const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>

<style scoped>
@media (prefers-reduced-motion: reduce) {
  .app-shell,
  .app-shell * {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    scroll-behavior: auto !important;
    transition-duration: 0.01ms !important;
  }
}
</style>
