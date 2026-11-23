<template>
  <div class="card card-hover dashboard-actions overflow-hidden">
    <div class="border-b border-gray-100/80 bg-gradient-to-r from-white/90 to-primary-50/45 px-6 py-5 dark:border-dark-700/80 dark:from-dark-800/90 dark:to-primary-950/25">
      <p class="text-[11px] font-semibold uppercase tracking-[0.16em] text-primary-600 dark:text-primary-400">{{ t('dashboard.title') }}</p>
      <h2 class="mt-1 text-lg font-semibold tracking-tight text-gray-900 dark:text-white">{{ t('dashboard.quickActions') }}</h2>
    </div>
    <div class="space-y-3 p-5">
      <button @click="router.push('/keys')" class="action-item group flex min-h-16 w-full items-center gap-4 rounded-2xl border border-gray-100 bg-gray-50/80 p-4 text-left transition-all duration-200 hover:-translate-y-0.5 hover:border-primary-200 hover:bg-primary-50/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/50 dark:border-dark-700/80 dark:bg-dark-800/50 dark:hover:border-primary-900/60 dark:hover:bg-primary-900/10">
        <div class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-xl bg-primary-100 transition-transform group-hover:scale-105 dark:bg-primary-900/30">
          <Icon name="key" size="lg" class="text-primary-600 dark:text-primary-400" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('dashboard.createApiKey') }}</p>
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('dashboard.generateNewKey') }}</p>
        </div>
        <Icon
          name="chevronRight"
          size="md"
          class="text-gray-400 transition-colors group-hover:text-primary-500 dark:text-dark-500"
        />
      </button>

      <button @click="router.push('/usage')" class="action-item group flex min-h-16 w-full items-center gap-4 rounded-2xl border border-gray-100 bg-gray-50/80 p-4 text-left transition-all duration-200 hover:-translate-y-0.5 hover:border-emerald-200 hover:bg-emerald-50/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500/50 dark:border-dark-700/80 dark:bg-dark-800/50 dark:hover:border-emerald-900/60 dark:hover:bg-emerald-900/10">
        <div class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-xl bg-emerald-100 transition-transform group-hover:scale-105 dark:bg-emerald-900/30">
          <Icon name="chart" size="lg" class="text-emerald-600 dark:text-emerald-400" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('dashboard.viewUsage') }}</p>
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('dashboard.checkDetailedLogs') }}</p>
        </div>
        <Icon
          name="chevronRight"
          size="md"
          class="text-gray-400 transition-colors group-hover:text-emerald-500 dark:text-dark-500"
        />
      </button>

      <button v-if="canUseBatchImage" @click="router.push('/batch-image')" class="action-item group flex min-h-16 w-full items-center gap-4 rounded-2xl border border-gray-100 bg-gray-50/80 p-4 text-left transition-all duration-200 hover:-translate-y-0.5 hover:border-sky-200 hover:bg-sky-50/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sky-500/50 dark:border-dark-700/80 dark:bg-dark-800/50 dark:hover:border-sky-900/60 dark:hover:bg-sky-900/10">
        <div class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-xl bg-sky-100 transition-transform group-hover:scale-105 dark:bg-sky-900/30">
          <Icon name="sparkles" size="lg" class="text-sky-600 dark:text-sky-400" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('dashboard.batchImageAgent') }}</p>
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('dashboard.batchImageAgentDesc') }}</p>
        </div>
        <Icon
          name="chevronRight"
          size="md"
          class="text-gray-400 transition-colors group-hover:text-sky-500 dark:text-dark-500"
        />
      </button>

      <button @click="router.push('/redeem')" class="action-item group flex min-h-16 w-full items-center gap-4 rounded-2xl border border-gray-100 bg-gray-50/80 p-4 text-left transition-all duration-200 hover:-translate-y-0.5 hover:border-amber-200 hover:bg-amber-50/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-500/50 dark:border-dark-700/80 dark:bg-dark-800/50 dark:hover:border-amber-900/60 dark:hover:bg-amber-900/10">
        <div class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-xl bg-amber-100 transition-transform group-hover:scale-105 dark:bg-amber-900/30">
          <Icon name="gift" size="lg" class="text-amber-600 dark:text-amber-400" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('dashboard.redeemCode') }}</p>
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('dashboard.addBalanceWithCode') }}</p>
        </div>
        <Icon
          name="chevronRight"
          size="md"
          class="text-gray-400 transition-colors group-hover:text-amber-500 dark:text-dark-500"
        />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useBatchImageAccess } from '@/composables/useBatchImageAccess'
const router = useRouter()
const { t } = useI18n()
const { canUseBatchImage, refreshBatchImageAccess } = useBatchImageAccess()

onMounted(() => {
  void refreshBatchImageAccess()
})
</script>

<style scoped>
.dashboard-actions {
  box-shadow: 0 14px 34px rgb(15 23 42 / 0.055);
}

.action-item {
  box-shadow: 0 5px 16px rgb(15 23 42 / 0.025);
}

@media (prefers-reduced-motion: reduce) {
  .action-item {
    transition: none;
  }

  .action-item:hover {
    transform: none;
  }
}
</style>
