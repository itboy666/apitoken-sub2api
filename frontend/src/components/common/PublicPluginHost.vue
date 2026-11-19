<template>
  <PublicPluginFrame v-for="plugin in plugins" :key="`${plugin.id}:${plugin.version}:${plugin.entrypoint_url}`" :plugin="plugin" />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { getPublicPlugins, publicPluginEntrypoint } from '@/api/publicPlugins'
import type { PublicPluginDescriptor } from '@/types/publicPlugins'
import PublicPluginFrame from './PublicPluginFrame.vue'

const route = useRoute()
const hidden = computed(() => /^\/(?:admin|setup)(?:\/|$)/.test(route.path))
const plugins = ref<PublicPluginDescriptor[]>([])
let controller: AbortController | null = null
let interval: ReturnType<typeof setInterval> | undefined
let disposed = false

async function refresh() {
  controller?.abort()
  if (disposed || hidden.value || document.visibilityState === 'hidden') {
    if (hidden.value) plugins.value = []
    return
  }
  const request = new AbortController()
  controller = request
  try {
    const descriptors = await getPublicPlugins(request.signal)
    if (disposed || request.signal.aborted || hidden.value) return
    const seen = new Set<number>()
    const next = descriptors.filter(plugin => {
      if (!plugin || !publicPluginEntrypoint(plugin) || !plugin.config || seen.has(plugin.id)) return false
      seen.add(plugin.id)
      return true
    })
    // Preserve an open frame and its keyboard focus when a poll returns unchanged data.
    if (JSON.stringify(next) !== JSON.stringify(plugins.value)) plugins.value = next
  } catch {
    // Fail closed: a disabled/uninstalled plugin must not linger after refresh fails.
    if (!request.signal.aborted) plugins.value = []
  } finally {
    if (controller === request) controller = null
  }
}
function visibility() { if (document.visibilityState !== 'hidden') void refresh() }
watch(() => route.fullPath, () => { plugins.value = []; void refresh() })
onMounted(() => {
  void refresh()
  interval = setInterval(() => { void refresh() }, 15_000)
  document.addEventListener('visibilitychange', visibility)
  window.addEventListener('sub2api:public-plugins-changed', refresh)
})
onBeforeUnmount(() => {
  disposed = true
  controller?.abort()
  if (interval) clearInterval(interval)
  document.removeEventListener('visibilitychange', visibility)
  window.removeEventListener('sub2api:public-plugins-changed', refresh)
})
</script>
