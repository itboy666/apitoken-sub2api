import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { reactive, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import PublicPluginFrame from '../PublicPluginFrame.vue'
import type { PublicPluginDescriptor } from '@/types/publicPlugins'

enableAutoUnmount(afterEach)
const { copyValue, push } = vi.hoisted(() => ({ copyValue: vi.fn(), push: vi.fn() }))
const locale = ref('zh')
const store = reactive({ cachedPublicSettings: { support_ticket_enabled: true }, auth_token: 'must-never-cross-bridge' })
vi.mock('@/stores', () => ({ useAppStore: () => store }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: copyValue }) }))
const plugin: PublicPluginDescriptor = { id: 12, plugin_key: 'apitoken.support', name: '客服', version: '1.0.0', entrypoint_url: `/api/v1/plugins/public/12/assets/${'a'.repeat(64)}/public/index.html`, config: { enabled: true, title: '支持', description: '', service_label: '客服 QQ', service_contact: '154215395', service_url: 'https://support.example/help', qq_group_number: '154895805', qq_group_url: '', use_ticket: true } }
function context(wrapper: VueWrapper) {
  const iframe = wrapper.find('iframe').element as HTMLIFrameElement
  return { iframe, token: new URL(iframe.src).hash.split('=')[1], source: iframe.contentWindow }
}
function message(wrapper: VueWrapper, payload: Record<string, unknown>, override: Partial<MessageEventInit> = {}) {
  const ctx = context(wrapper)
  window.dispatchEvent(new MessageEvent('message', { source: ctx.source, origin: 'null', data: { source: 'sub2api-public-plugin', bridge_token: ctx.token, ...payload }, ...override }))
}
beforeEach(() => { vi.clearAllMocks(); locale.value = 'zh'; store.cachedPublicSettings.support_ticket_enabled = true; document.documentElement.classList.remove('dark'); copyValue.mockResolvedValue(true); push.mockResolvedValue(undefined) })

describe('PublicPluginFrame sandbox and bridge', () => {
  it('loads a sandboxed package and only sends explicit public initialization fields', async () => {
    const wrapper = mount(PublicPluginFrame, { attachTo: document.body, props: { plugin: { ...plugin, config: { ...plugin.config, private_token: 'secret' } } as PublicPluginDescriptor } })
    const ctx = context(wrapper); const post = vi.spyOn(ctx.source!, 'postMessage')
    expect(ctx.iframe.getAttribute('sandbox')).toBe('allow-scripts')
    expect(ctx.iframe.getAttribute('referrerpolicy')).toBe('no-referrer')
    message(wrapper, { type: 'widget.ready' })
    const payload = post.mock.calls[0]?.[0] as Record<string, unknown>
    expect(payload).toMatchObject({ source: 'sub2api-public-host', type: 'widget.init', locale: 'zh', theme: 'light', support_ticket_enabled: true })
    expect(JSON.stringify(payload)).not.toContain('secret'); expect(JSON.stringify(payload)).not.toContain('auth_token')
  })
  it.each([
    { origin: 'https://evil.example' },
    { source: window },
    { data: { source: 'sub2api-public-plugin', bridge_token: 'wrong', type: 'clipboard.copy', request_id: 'wrong', value: '154215395' } }
  ])('rejects messages without the matching source, origin and token', async override => {
    const wrapper = mount(PublicPluginFrame, { attachTo: document.body, props: { plugin } })
    message(wrapper, { type: 'clipboard.copy', request_id: 'copy1', value: '154215395' }, override)
    await flushPromises(); expect(copyValue).not.toHaveBeenCalled()
  })
  it('permits copying public contacts only and rejects replayed requests', async () => {
    const wrapper = mount(PublicPluginFrame, { attachTo: document.body, props: { plugin } })
    message(wrapper, { type: 'clipboard.copy', request_id: 'a', value: 'something else' })
    message(wrapper, { type: 'clipboard.copy', request_id: 'b', value: '154215395' })
    message(wrapper, { type: 'clipboard.copy', request_id: 'b', value: '154215395' })
    await flushPromises(); expect(copyValue).toHaveBeenCalledTimes(1); expect(copyValue).toHaveBeenCalledWith('154215395')
  })
  it('allows configured HTTPS links but blocks arbitrary and dangerous destinations', async () => {
    const popup = vi.spyOn(window, 'open').mockReturnValue(null)
    const wrapper = mount(PublicPluginFrame, { attachTo: document.body, props: { plugin } })
    message(wrapper, { type: 'link.open', request_id: 'bad', url: 'javascript:alert(1)' })
    message(wrapper, { type: 'link.open', request_id: 'other', url: 'https://evil.example/' })
    message(wrapper, { type: 'link.open', request_id: 'ok', url: 'https://support.example/help' })
    await flushPromises(); expect(popup).toHaveBeenCalledTimes(1); expect(popup).toHaveBeenCalledWith('https://support.example/help', '_blank', 'noopener,noreferrer'); popup.mockRestore()
  })
  it('only navigates to tickets while both ticket controls are enabled', async () => {
    const wrapper = mount(PublicPluginFrame, { attachTo: document.body, props: { plugin } })
    store.cachedPublicSettings.support_ticket_enabled = false
    message(wrapper, { type: 'link.open', request_id: 'denied', url: '/support-tickets' }); await flushPromises(); expect(push).not.toHaveBeenCalled()
    store.cachedPublicSettings.support_ticket_enabled = true
    message(wrapper, { type: 'link.open', request_id: 'yes', url: '/support-tickets' }); await flushPromises(); expect(push).toHaveBeenCalledWith('/support-tickets')
  })
  it('clamps frame size to the viewport and collapses on outside interaction', async () => {
    const wrapper = mount(PublicPluginFrame, { attachTo: document.body, props: { plugin } })
    const ctx = context(wrapper); const post = vi.spyOn(ctx.source!, 'postMessage')
    message(wrapper, { type: 'widget.resize', width: 100000, height: 100000, open: true }); await flushPromises()
    expect(parseInt(ctx.iframe.style.width)).toBeLessThan(window.innerWidth)
    expect(parseInt(ctx.iframe.style.height)).toBeLessThan(window.innerHeight)
    document.dispatchEvent(new Event('pointerdown')); await flushPromises()
    expect(ctx.iframe.style.width).toBe('86px')
    expect(post).toHaveBeenCalledWith(expect.objectContaining({ type: 'widget.close' }), '*')
  })
  it('ignores nonfinite dimensions and follows locale and theme changes', async () => {
    const wrapper = mount(PublicPluginFrame, { attachTo: document.body, props: { plugin } })
    const ctx = context(wrapper); const post = vi.spyOn(ctx.source!, 'postMessage')
    message(wrapper, { type: 'widget.resize', width: NaN, height: Infinity, open: true }); await flushPromises(); expect(ctx.iframe.style.width).toBe('86px')
    locale.value = 'en'; document.documentElement.classList.add('dark'); await flushPromises()
    expect(post).toHaveBeenLastCalledWith(expect.objectContaining({ type: 'widget.init', locale: 'en', theme: 'dark' }), '*')
    expect(ctx.iframe.style.colorScheme).toBe('dark')
  })
})
