<template>
  <iframe
    ref="frame"
    :src="source"
    :title="plugin.name"
    sandbox="allow-scripts"
    referrerpolicy="no-referrer"
    class="public-plugin-frame"
    :style="frameStyle"
    data-testid="public-plugin-frame"
    @load="sendContext"
  />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { useClipboard } from '@/composables/useClipboard'
import { publicPluginEntrypoint, publicPluginURL } from '@/api/publicPlugins'
import type { PublicPluginDescriptor } from '@/types/publicPlugins'

const props = defineProps<{ plugin: PublicPluginDescriptor }>()
const router = useRouter()
const { locale } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const frame = ref<HTMLIFrameElement | null>(null)
const token = crypto.randomUUID()
const source = computed(() => `${publicPluginEntrypoint(props.plugin)}#bridge_token=${encodeURIComponent(token)}`)
const viewportWidth = ref(document.documentElement.clientWidth || window.innerWidth)
const viewportHeight = ref(window.innerHeight)
const theme = ref(document.documentElement.classList.contains('dark') ? 'dark' : 'light')
const mobile = computed(() => viewportWidth.value < 640)
const width = ref(mobile.value ? 102 : 86)
const height = ref(mobile.value ? 62 : 174)
const open = ref(false)
const ticketEnabled = computed(() => props.plugin.config.use_ticket === true && appStore.cachedPublicSettings?.support_ticket_enabled === true)
const right = computed(() => mobile.value ? 8 : 16)
const bottom = computed(() => mobile.value ? 16 : 108)
const maxWidth = computed(() => Math.max(44, viewportWidth.value - right.value * 2))
const maxHeight = computed(() => Math.max(44, viewportHeight.value - bottom.value - 16))
const frameStyle = computed(() => ({ colorScheme: theme.value, width: `${Math.min(width.value, maxWidth.value)}px`, height: `${Math.min(height.value, maxHeight.value)}px`, right: `${right.value}px`, bottom: `max(${bottom.value}px, env(safe-area-inset-bottom))` }))
const completedRequests = new Set<string>()
let observer: MutationObserver | undefined
let disposed = false
let copying = false

function post(payload: Record<string, unknown>) {
  if (disposed) return
  frame.value?.contentWindow?.postMessage({ source: 'sub2api-public-host', bridge_token: token, ...payload }, '*')
}
function sendContext() {
  const config = props.plugin.config
  // Explicit field selection prevents future private descriptor fields leaking into a frame.
  post({ type: 'widget.init', config: {
    enabled: config.enabled, title: config.title, description: config.description,
    service_label: config.service_label, service_contact: config.service_contact, service_url: config.service_url,
    qq_group_number: config.qq_group_number, qq_group_url: config.qq_group_url, use_ticket: config.use_ticket
  }, locale: locale.value, theme: theme.value, viewport_width: viewportWidth.value, viewport_height: viewportHeight.value,
  support_ticket_enabled: ticketEnabled.value })
}
function close() {
  if (!open.value) return
  open.value = false
  width.value = mobile.value ? 102 : 86
  height.value = mobile.value ? 62 : 174
  post({ type: 'widget.close' })
}
function resize() {
  viewportWidth.value = document.documentElement.clientWidth || window.innerWidth
  viewportHeight.value = window.innerHeight
  if (!open.value) { width.value = mobile.value ? 102 : 86; height.value = mobile.value ? 62 : 174 }
  sendContext()
}
function outside(event: Event) {
  if (copying && event.type === 'focusin') return
  if (event.target !== frame.value) close()
}
function keydown(event: KeyboardEvent) {
  if (event.key === 'Escape') close()
}
function respond(message: Record<string, unknown>, ok: boolean, text: string) {
  post({ type: `${message.type}.result`, request_id: message.request_id, ok, message: text, error: ok ? undefined : text })
}
async function onMessage(event: MessageEvent) {
  const message = event.data
  const frameWindow = frame.value?.contentWindow
  if (!frameWindow || event.source !== frameWindow || event.origin !== 'null' || !message || typeof message !== 'object' ||
    message.source !== 'sub2api-public-plugin' || message.bridge_token !== token) return
  if (message.type === 'widget.ready') { sendContext(); return }
  if (message.type === 'widget.resize') {
    if (typeof message.width !== 'number' || typeof message.height !== 'number' || !Number.isFinite(message.width) || !Number.isFinite(message.height) || typeof message.open !== 'boolean') return
    width.value = Math.max(44, Math.min(Math.round(message.width), maxWidth.value))
    height.value = Math.max(44, Math.min(Math.round(message.height), maxHeight.value))
    open.value = message.open
    return
  }
  if (message.type !== 'clipboard.copy' && message.type !== 'link.open') return
  if (typeof message.request_id !== 'string' || !/^[a-zA-Z0-9_.:-]{1,100}$/.test(message.request_id) || completedRequests.has(message.request_id)) return
  // Bound replay tracking while retaining all recent interactions.
  if (completedRequests.size >= 256) completedRequests.delete(completedRequests.values().next().value!)
  completedRequests.add(message.request_id)
  const denied = locale.value.startsWith('zh') ? '此操作不可用。' : 'This action is unavailable.'
  try {
    if (message.type === 'clipboard.copy') {
      const allowed = [props.plugin.config.service_contact, props.plugin.config.qq_group_number].filter(value => typeof value === 'string' && value.length > 0)
      if (typeof message.value !== 'string' || !allowed.includes(message.value)) { respond(message, false, denied); return }
      copying = true
      let copied = false
      try { copied = await copyToClipboard(message.value) }
      finally { copying = false; frame.value?.focus({ preventScroll: true }) }
      respond(message, copied, copied ? (locale.value.startsWith('zh') ? '已复制，可粘贴使用。' : 'Copied. Ready to paste.') : (locale.value.startsWith('zh') ? '复制失败，请手动复制。' : 'Copy failed. Please copy manually.'))
    } else if (message.url === '/support-tickets' && ticketEnabled.value) {
      close()
      await router.push('/support-tickets')
      respond(message, true, '')
    } else {
      const url = publicPluginURL(message.url)
      const allowed = [props.plugin.config.service_url, props.plugin.config.qq_group_url].map(publicPluginURL).filter(Boolean)
      if (!url || !allowed.includes(url)) { respond(message, false, denied); return }
      window.open(url, '_blank', 'noopener,noreferrer')
      respond(message, true, '')
      close()
    }
  } catch { respond(message, false, denied) }
}
watch([() => props.plugin.config, locale, theme, ticketEnabled], sendContext, { deep: true })
onMounted(() => {
  window.addEventListener('message', onMessage)
  window.addEventListener('resize', resize)
  document.addEventListener('pointerdown', outside)
  document.addEventListener('focusin', outside)
  document.addEventListener('keydown', keydown)
  observer = new MutationObserver(() => { theme.value = document.documentElement.classList.contains('dark') ? 'dark' : 'light' })
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})
onBeforeUnmount(() => {
  disposed = true
  observer?.disconnect()
  window.removeEventListener('message', onMessage)
  window.removeEventListener('resize', resize)
  document.removeEventListener('pointerdown', outside)
  document.removeEventListener('focusin', outside)
  document.removeEventListener('keydown', keydown)
})
</script>

<style scoped>
.public-plugin-frame { position: fixed; z-index: 45; border: 0; background: transparent; }
</style>
