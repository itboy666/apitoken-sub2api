import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import PublicPluginHost from '../PublicPluginHost.vue'

enableAutoUnmount(afterEach)
const { getPlugins } = vi.hoisted(() => ({ getPlugins: vi.fn() }))
const route = reactive({ path: '/home', fullPath: '/home' })
vi.mock('vue-router', () => ({ useRoute: () => route }))
vi.mock('@/api/publicPlugins', async original => ({ ...await original<typeof import('@/api/publicPlugins')>(), getPublicPlugins: getPlugins }))
vi.mock('../PublicPluginFrame.vue', () => ({ default: { props: ['plugin'], template: '<div data-testid="plugin">{{ plugin.name }}</div>' } }))
const plugin = { id: 12, plugin_key: 'apitoken.support', name: '客服', version: '1.0.0', entrypoint_url: `/api/v1/plugins/public/12/assets/${'a'.repeat(64)}/public/index.html`, config: { title: '支持' } }
beforeEach(() => { vi.clearAllMocks(); vi.useFakeTimers(); route.path = '/home'; route.fullPath = '/home'; getPlugins.mockResolvedValue([plugin]) })
afterEach(() => { vi.useRealTimers() })

describe('PublicPluginHost', () => {
  it('loads packages and removes a disabled package on the next poll', async () => {
    const wrapper = mount(PublicPluginHost); await flushPromises()
    expect(wrapper.text()).toContain('客服')
    getPlugins.mockResolvedValue([])
    await vi.advanceTimersByTimeAsync(15_000); await flushPromises()
    expect(wrapper.find('[data-testid="plugin"]').exists()).toBe(false)
  })
  it.each(['/admin/plugins', '/setup'])('does not load public plugins on %s', async path => {
    route.path = path; route.fullPath = path
    const wrapper = mount(PublicPluginHost); await flushPromises(); await vi.advanceTimersByTimeAsync(15_000)
    expect(getPlugins).not.toHaveBeenCalled(); expect(wrapper.text()).toBe('')
  })
  it('ignores an in-flight request after navigating into the admin area', async () => {
    let finish!: (value: unknown[]) => void
    getPlugins.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    const wrapper = mount(PublicPluginHost)
    route.path = '/admin/plugins'; route.fullPath = route.path
    await flushPromises(); finish([plugin]); await flushPromises()
    expect(wrapper.text()).toBe('')
  })
  it('rejects untrusted iframe URLs and duplicate entries', async () => {
    getPlugins.mockResolvedValue([plugin, plugin, { ...plugin, id: 15, entrypoint_url: 'https://evil.example/frame' }])
    const wrapper = mount(PublicPluginHost); await flushPromises()
    expect(wrapper.findAll('[data-testid="plugin"]')).toHaveLength(1)
  })
  it('fails closed on network errors and refreshes on visibility return', async () => {
    const wrapper = mount(PublicPluginHost); await flushPromises()
    getPlugins.mockRejectedValueOnce(new Error('offline'))
    window.dispatchEvent(new Event('sub2api:public-plugins-changed')); await flushPromises()
    expect(wrapper.text()).toBe('')
    document.dispatchEvent(new Event('visibilitychange')); await flushPromises()
    expect(wrapper.text()).toContain('客服')
  })
})
