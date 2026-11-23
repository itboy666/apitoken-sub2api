<template>
  <AppLayout>
    <div class="dashboard-page space-y-6">
      <header class="dashboard-intro relative overflow-hidden rounded-[24px] border border-white/70 bg-white/[0.78] px-5 py-5 shadow-[0_16px_42px_rgba(15,23,42,0.06)] backdrop-blur-xl dark:border-dark-700/70 dark:bg-dark-800/[0.72] sm:px-7 sm:py-6">
        <div aria-hidden="true" class="dashboard-intro-orb dashboard-intro-orb-primary"></div>
        <div aria-hidden="true" class="dashboard-intro-orb dashboard-intro-orb-secondary"></div>
        <div class="relative flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div class="min-w-0">
            <div class="mb-3 inline-flex items-center gap-2 rounded-full border border-primary-200/70 bg-primary-50/80 px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.18em] text-primary-700 dark:border-primary-800/60 dark:bg-primary-950/50 dark:text-primary-300">
              <span class="h-1.5 w-1.5 rounded-full bg-emerald-500 shadow-[0_0_0_3px_rgba(16,185,129,0.14)]"></span>
              {{ t('dashboard.title') }}
            </div>
            <h1 class="max-w-3xl text-2xl font-semibold tracking-[-0.025em] text-gray-950 dark:text-white sm:text-3xl">{{ t('dashboard.welcomeMessage') }}</h1>
            <p class="mt-2 max-w-2xl text-sm leading-6 text-gray-500 dark:text-dark-300">{{ t('dashboard.overviewHint') }}</p>
          </div>
          <div v-if="user?.email" class="inline-flex w-fit shrink-0 items-center gap-2 rounded-2xl border border-gray-200/80 bg-white/75 px-3.5 py-2 text-xs text-gray-600 shadow-sm backdrop-blur-sm dark:border-dark-600 dark:bg-dark-900/45 dark:text-dark-200">
            <span class="flex h-7 w-7 items-center justify-center rounded-xl bg-gradient-to-br from-primary-500 to-indigo-500 text-[10px] font-bold text-white">{{ initials }}</span>
            <span class="max-w-[13rem] truncate">{{ user.email }}</span>
          </div>
        </div>
      </header>
      <div v-if="loading" class="flex items-center justify-center py-12"><LoadingSpinner /></div>
      <template v-else-if="stats">
        <UserDashboardStats :stats="stats" :balance="user?.balance || 0" :is-simple="authStore.isSimpleMode" :platform-quotas="platformQuotas" />
        <UserDashboardCharts v-model:startDate="startDate" v-model:endDate="endDate" v-model:granularity="granularity" :loading="loadingCharts" :trend="trendData" :models="modelStats" @dateRangeChange="loadCharts" @granularityChange="loadCharts" @refresh="refreshAll" />
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
          <div class="lg:col-span-2"><UserDashboardRecentUsage :data="recentUsage" :loading="loadingUsage" /></div>
          <div class="lg:col-span-1"><UserDashboardQuickActions /></div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'; import { useAuthStore } from '@/stores/auth'; import { usageAPI, type UserDashboardStats as UserStatsType } from '@/api/usage'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'; import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'; import UserDashboardCharts from '@/components/user/dashboard/UserDashboardCharts.vue'
import UserDashboardRecentUsage from '@/components/user/dashboard/UserDashboardRecentUsage.vue'; import UserDashboardQuickActions from '@/components/user/dashboard/UserDashboardQuickActions.vue'
import type { UsageLog, TrendDataPoint, ModelStat, PlatformQuotaItem } from '@/types'
import { getMyPlatformQuotas } from '@/api/user'
import { formatDateLocalInput } from '@/utils/format'

const authStore = useAuthStore(); const user = computed(() => authStore.user)
const { t } = useI18n()
const initials = computed(() => {
  const value = user.value?.username || user.value?.email?.split('@')[0] || 'AI'
  return value.slice(0, 2).toUpperCase()
})
const stats = ref<UserStatsType | null>(null); const loading = ref(false); const loadingUsage = ref(false); const loadingCharts = ref(false)
const trendData = ref<TrendDataPoint[]>([]); const modelStats = ref<ModelStat[]>([]); const recentUsage = ref<UsageLog[]>([])
const platformQuotas = ref<PlatformQuotaItem[] | null>(null)

const startDate = ref(formatDateLocalInput(new Date(Date.now() - 6 * 86400000))); const endDate = ref(formatDateLocalInput(new Date())); const granularity = ref('day')

const loadStats = async () => { loading.value = true; try { await authStore.refreshUser(); stats.value = await usageAPI.getDashboardStats() } catch (error) { console.error('Failed to load dashboard stats:', error) } finally { loading.value = false } }
const loadCharts = async () => { loadingCharts.value = true; try { const res = await Promise.all([usageAPI.getDashboardTrend({ start_date: startDate.value, end_date: endDate.value, granularity: granularity.value as any }), usageAPI.getDashboardModels({ start_date: startDate.value, end_date: endDate.value })]); trendData.value = res[0].trend || []; modelStats.value = res[1].models || [] } catch (error) { console.error('Failed to load charts:', error) } finally { loadingCharts.value = false } }
const loadRecent = async () => { loadingUsage.value = true; try { const res = await usageAPI.getByDateRange(startDate.value, endDate.value); recentUsage.value = res.items.slice(0, 5) } catch (error) { console.error('Failed to load recent usage:', error) } finally { loadingUsage.value = false } }
const loadPlatformQuotas = async () => { try { const data = await getMyPlatformQuotas(); platformQuotas.value = data.platform_quotas ?? [] } catch (error) { console.warn('Failed to load platform quotas:', error); platformQuotas.value = [] } }
const refreshAll = () => { loadStats(); loadCharts(); loadRecent(); loadPlatformQuotas() }

onMounted(() => { refreshAll() })
</script>

<style scoped>
.dashboard-intro {
  isolation: isolate;
}

.dashboard-intro-orb {
  position: absolute;
  pointer-events: none;
  border-radius: 999px;
  filter: blur(2px);
  opacity: 0.68;
  z-index: -1;
}

.dashboard-intro-orb-primary {
  top: -5rem;
  right: 8%;
  width: 15rem;
  height: 15rem;
  background: radial-gradient(circle, rgb(20 184 166 / 0.17), transparent 68%);
}

.dashboard-intro-orb-secondary {
  bottom: -7rem;
  right: 28%;
  width: 18rem;
  height: 12rem;
  background: radial-gradient(circle, rgb(99 102 241 / 0.13), transparent 70%);
}

:global(.dark) .dashboard-intro-orb-primary {
  opacity: 0.42;
}

:global(.dark) .dashboard-intro-orb-secondary {
  opacity: 0.3;
}

@media (prefers-reduced-motion: reduce) {
  .dashboard-intro-orb {
    filter: none;
  }
}
</style>
