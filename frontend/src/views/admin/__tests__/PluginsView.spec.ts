import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import PluginsView from '../PluginsView.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import type { PluginUISession } from '@/api/admin'

enableAutoUnmount(afterEach)

const {
  listPlugins,
  uploadPlugin,
  enablePlugin,
  disablePlugin,
  removePlugin,
  savePluginConfig,
  recoverPluginConfig,
  getPluginConfig,
  showSuccess,
  showError,
  createUISession,
  stepUpRun,
} = vi.hoisted(() => ({
  listPlugins: vi.fn(),
  uploadPlugin: vi.fn(),
  enablePlugin: vi.fn(),
  disablePlugin: vi.fn(),
  removePlugin: vi.fn(),
  savePluginConfig: vi.fn(),
  recoverPluginConfig: vi.fn(),
  getPluginConfig: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
  createUISession: vi.fn(),
  stepUpRun: vi.fn((action: () => Promise<unknown>) => action()),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    plugins: {
      list: listPlugins,
      upload: uploadPlugin,
      enable: enablePlugin,
      disable: disablePlugin,
      remove: removePlugin,
      getConfig: getPluginConfig,
      saveConfig: savePluginConfig,
      recoverConfig: recoverPluginConfig,
      test: vi.fn().mockResolvedValue({ success: true, message: 'ok', latency_ms: 1 }),
      createUISession,
    },
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showInfo: vi.fn(),
  }),
}))

vi.mock('@/composables/useStepUp', () => ({
  useStepUp: () => ({ run: stepUpRun }),
  isStepUpBlocked: () => false,
  isStepUpCancelled: () => false,
  stepUpBlockReason: () => '',
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))

const plugin = {
  id: 7,
  plugin_key: 'local.test.transport',
  name: 'Test Transport',
  version: '1.0.0',
  description: '',
  author: 'test',
  manifest: {
    schema_version: 1,
    id: 'local.test.transport',
    name: 'Test Transport',
    version: '1.0.0',
    requires: {
      sub2api: '>=0.1.0',
      plugin_protocol: 1,
      transport_api: 1,
      ui_bridge: 1,
    },
    capabilities: [],
    ui: { entrypoint: 'ui/index.html' },
  },
  binary_sha256: 'a'.repeat(64),
  signature_status: 'trusted' as const,
  state: 'disabled' as const,
  last_error: '',
  installed_at: '2026-08-22T00:00:00Z',
  updated_at: '2026-08-22T00:00:00Z',
  bindings: [
    {
      id: 1,
      plugin_id: 7,
      capability: 'openai.oauth.outbound_transport.v1',
      platform: 'openai',
      account_type: 'oauth',
      enabled: false,
      rollout_percent: 100,
    },
  ],
  compatibility: {
    compatible: true,
    tested: true,
    status: 'compatible' as const,
    message: '',
    current_sub2api_version: '0.1.0',
    required_sub2api_version: '>=0.1.0',
    recommended_sub2api_version: '0.1.0',
    plugin_protocol: 1,
    transport_api: 1,
    ui_bridge: 1,
  },
  runtime_healthy: false,
  runtime_message: '',
}

function mountView() {
  return mount(PluginsView, {
    attachTo: document.body,
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        BaseDialog: { template: '<div><slot /></div>' },
        ConfirmDialog: { props: ['show', 'message', 'danger'], emits: ['confirm', 'cancel'], template: '<div v-if="show" data-test="confirm"><p>{{ message }}</p><button data-test="accept" @click="$emit(\'confirm\')">确认</button><button data-test="cancel" @click="$emit(\'cancel\')">取消</button></div>' },
        Icon: true,
        TotpStepUpDialog: true,
      },
    },
  })
}

function sendBridge(wrapper: ReturnType<typeof mountView>, type: string, requestID: string, extra: Record<string, unknown> = {}) {
  const iframe = wrapper.get('iframe').element as HTMLIFrameElement
  const token = new URL(iframe.src).hash.split('=')[1]
  window.dispatchEvent(new MessageEvent('message', { source: iframe.contentWindow, origin: 'null', data: {
    source: 'sub2api-plugin-ui', bridge_token: token, type, request_id: requestID, ...extra,
  } }))
}
async function openConfig(wrapper: ReturnType<typeof mountView>, index = 0) {
  await wrapper.findAll('button').filter(button => button.text() === 'admin.plugins.configure')[index].trigger('click')
  await flushPromises()
}
async function closeConfig(wrapper: ReturnType<typeof mountView>) {
  wrapper.getComponent(BaseDialog).vm.$emit('close')
  await flushPromises()
}

describe('管理员插件页二次验证', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    stepUpRun.mockImplementation((action: () => Promise<unknown>) => action())
    listPlugins.mockResolvedValue([plugin])
    uploadPlugin.mockResolvedValue(plugin)
    enablePlugin.mockResolvedValue(plugin)
    disablePlugin.mockResolvedValue(plugin)
    removePlugin.mockResolvedValue(plugin)
    savePluginConfig.mockResolvedValue({ enabled: true })
    recoverPluginConfig.mockResolvedValue({ enabled: true })
    getPluginConfig.mockResolvedValue({})
    createUISession.mockResolvedValue({
      url: '/api/v1/plugin-ui/token/index.html#bridge_token=bridge',
      bridge_token: 'bridge',
      ui_bridge_version: 1,
      expires_at: '2026-08-22T01:00:00Z',
    })
  })

  it('启用插件通过 step-up 控制器执行', async () => {
    const wrapper = mountView()
    await flushPromises()

    const button = wrapper.findAll('button').find((item) => item.text().includes('admin.plugins.enable'))
    expect(button).toBeDefined()
    await button!.trigger('click')
    await flushPromises()

    expect(stepUpRun).toHaveBeenCalledTimes(1)
    expect(enablePlugin).toHaveBeenCalledWith(7, 100, false)
  })

  it.each([7, 8])('keeps the latest session when reopening configuration for plugin %s', async (id) => {
    let resolveOld!: (session: PluginUISession) => void
    createUISession.mockReturnValueOnce(new Promise<PluginUISession>((resolve) => { resolveOld = resolve }))
    listPlugins.mockResolvedValue([plugin, { ...plugin, id: 8, name: 'Other Transport' }])
    const wrapper = mountView()
    await flushPromises()
    const buttons = wrapper.findAll('button').filter(item => item.text() === 'admin.plugins.configure')
    await buttons[0].trigger('click')
    wrapper.getComponent(BaseDialog).vm.$emit('close')
    await buttons[id === 7 ? 0 : 1].trigger('click')
    await flushPromises()
    const currentURL = wrapper.get('iframe').attributes('src')

    resolveOld({ url: '/obsolete-session', bridge_token: 'old', ui_bridge_version: 1, expires_at: '' })
    await flushPromises()
    expect(wrapper.get('iframe').attributes('src')).toBe(currentURL)
  })

  it('ignores an old session error while the reopened configuration is loading', async () => {
    let rejectOld!: (error: Error) => void
    let resolveCurrent!: (session: PluginUISession) => void
    createUISession
      .mockReturnValueOnce(new Promise((_, reject) => { rejectOld = reject }))
      .mockReturnValueOnce(new Promise<PluginUISession>((resolve) => { resolveCurrent = resolve }))
    const wrapper = mountView()
    await flushPromises()
    const button = wrapper.findAll('button').find(item => item.text() === 'admin.plugins.configure')!
    await button.trigger('click')
    wrapper.getComponent(BaseDialog).vm.$emit('close')
    await button.trigger('click')
    rejectOld(new Error('obsolete failure'))
    await flushPromises()

    expect(wrapper.text()).not.toContain('obsolete failure')
    expect(wrapper.text()).toContain('admin.plugins.loadingUI')
    resolveCurrent({ url: '/current-session', bridge_token: 'current', ui_bridge_version: 1, expires_at: '' })
    await flushPromises()
    await wrapper.get('iframe').trigger('load')
    expect(wrapper.get('iframe').attributes('src')).toBe('/current-session')
    expect(wrapper.text()).not.toContain('admin.plugins.loadingUI')
  })

  it('does not restore a session after the configuration dialog closes', async () => {
    let resolveSession!: (session: PluginUISession) => void
    createUISession.mockReturnValueOnce(new Promise<PluginUISession>((resolve) => { resolveSession = resolve }))
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(item => item.text() === 'admin.plugins.configure')!.trigger('click')
    wrapper.getComponent(BaseDialog).vm.$emit('close')
    resolveSession({ url: '/closed-session', bridge_token: 'old', ui_bridge_version: 1, expires_at: '' })
    await flushPromises()
    expect(wrapper.find('iframe').exists()).toBe(false)
  })

  it('上传插件通过 step-up 控制器执行', async () => {
    const wrapper = mountView()
    await flushPromises()
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', {
      configurable: true,
      value: [new File(['plugin'], 'transport.s2plugin', { type: 'application/zip' })],
    })

    await input.trigger('change')
    await flushPromises()

    expect(stepUpRun).toHaveBeenCalledTimes(1)
    expect(uploadPlugin).toHaveBeenCalledTimes(1)
  })

  it('keeps OAuth rollout controls for transport packages', async () => {
    const wrapper = mountView(); await flushPromises()
    expect(wrapper.find('input[type="range"]').exists()).toBe(true)
  })

  it('shows native enabled state for a website plugin without OAuth bindings', async () => {
    listPlugins.mockResolvedValue([{ ...plugin, name: 'Support', state: 'enabled', bindings: [], manifest: { ...plugin.manifest, capabilities: [{ id: 'website.support_widget.v1', platform: 'website', account_type: '' }], public_ui: { entrypoint: 'public/index.html' } } }])
    const wrapper = mountView(); await flushPromises()
    expect(wrapper.find('input[type="range"]').exists()).toBe(false)
    const buttons = wrapper.findAll('button')
    expect(buttons.some(button => button.text() === 'admin.plugins.disable')).toBe(true)
    expect(buttons.some(button => button.text() === 'admin.plugins.enable')).toBe(false)
    expect(buttons.find(button => button.text() === 'admin.plugins.uninstall')?.attributes('disabled')).toBeDefined()
  })

  it('requires the in-app confirmation before disabling a website plugin', async () => {
    listPlugins.mockResolvedValue([{ ...plugin, state: 'enabled', bindings: [], manifest: { ...plugin.manifest, capabilities: [{ id: 'website.support_widget.v1', platform: 'website', account_type: '' }], public_ui: { entrypoint: 'public/index.html' } } }])
    const wrapper = mountView(); await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'admin.plugins.disable')!.trigger('click')
    const dialog = wrapper.getComponent(ConfirmDialog)
    expect(dialog.props('show')).toBe(true)
    expect(dialog.props('message')).toBe('admin.plugins.confirmDisableWebsite')
    expect(disablePlugin).not.toHaveBeenCalled()
    dialog.vm.$emit('confirm'); await flushPromises()
    expect(disablePlugin).toHaveBeenCalledWith(7)
    expect(stepUpRun).toHaveBeenCalledTimes(1)
  })

  it('does not uninstall until the in-app confirmation is accepted', async () => {
    const wrapper = mountView(); await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'admin.plugins.uninstall')!.trigger('click')
    const dialog = wrapper.getComponent(ConfirmDialog)
    expect(dialog.props('show')).toBe(true)
    expect(dialog.props('danger')).toBe(true)
    expect(removePlugin).not.toHaveBeenCalled()
    dialog.vm.$emit('cancel'); await flushPromises()
    expect(removePlugin).not.toHaveBeenCalled()
    expect(stepUpRun).not.toHaveBeenCalled()
  })

  it('does not leak an old load into a reopened dialog reusing the same request ID', async () => {
    let finishOld!: (value: unknown) => void
    let finishNew!: (value: unknown) => void
    getPluginConfig
      .mockReturnValueOnce(new Promise(resolve => { finishOld = resolve }))
      .mockReturnValueOnce(new Promise(resolve => { finishNew = resolve }))
    const wrapper = mountView(); await flushPromises(); await openConfig(wrapper)
    const oldFrame = wrapper.get('iframe').element as HTMLIFrameElement
    const oldPost = vi.spyOn(oldFrame.contentWindow!, 'postMessage')
    sendBridge(wrapper, 'config.load', 'load1')
    await closeConfig(wrapper); await openConfig(wrapper)
    const newFrame = wrapper.get('iframe').element as HTMLIFrameElement
    const newPost = vi.spyOn(newFrame.contentWindow!, 'postMessage')
    sendBridge(wrapper, 'config.load', 'load1')
    finishOld({ old_secret: 'must-not-leak' }); await flushPromises()
    expect(oldPost).not.toHaveBeenCalled(); expect(newPost).not.toHaveBeenCalled()
    finishNew({ current: true }); await flushPromises()
    expect(newPost).toHaveBeenCalledTimes(1)
    expect(newPost).toHaveBeenCalledWith(expect.objectContaining({ request_id: 'load1', config: { current: true } }), '*')
  })

  it('rejects config writes from other windows, origins, and bridge sessions', async () => {
    const wrapper = mountView(); await flushPromises(); await openConfig(wrapper)
    const iframe = wrapper.get('iframe').element as HTMLIFrameElement
    expect(iframe.getAttribute('sandbox')).toBe('allow-scripts')
    const token = new URL(iframe.src).hash.split('=')[1]
    for (const attack of [
      { source: window, origin: 'null', token },
      { source: iframe.contentWindow, origin: 'https://attacker.invalid', token },
      { source: iframe.contentWindow, origin: 'null', token: 'forged' },
    ]) {
      window.dispatchEvent(new MessageEvent('message', { source: attack.source, origin: attack.origin, data: {
        source: 'sub2api-plugin-ui', bridge_token: attack.token, request_id: 'forged-save', type: 'config.save', config: { fee_rate: 100 },
      } }))
    }
    await flushPromises()
    expect(savePluginConfig).not.toHaveBeenCalled()
    expect(stepUpRun).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('invalidates old async responses when the iframe navigates with the same WindowProxy', async () => {
    let finishOld!: (value: unknown) => void
    getPluginConfig.mockReturnValueOnce(new Promise(resolve => { finishOld = resolve }))
    const wrapper = mountView(); await flushPromises(); await openConfig(wrapper)
    await wrapper.get('iframe').trigger('load')
    const iframe = wrapper.get('iframe').element as HTMLIFrameElement
    const post = vi.spyOn(iframe.contentWindow!, 'postMessage')
    sendBridge(wrapper, 'config.load', 'load1')
    await wrapper.get('iframe').trigger('load')
    getPluginConfig.mockResolvedValueOnce({ new_document: true })
    sendBridge(wrapper, 'config.load', 'load1'); await flushPromises()
    finishOld({ old_document_secret: 'must-not-leak' }); await flushPromises()
    expect(post).toHaveBeenCalledTimes(1)
    expect(post).toHaveBeenCalledWith(expect.objectContaining({ config: { new_document: true } }), '*')
  })

  it.each([{ id: 7, type: 'config.save' }, { id: 8, type: 'config.save' }, { id: 7, type: 'config.recover' }])('does not run a stale step-up mutation after reopening plugin $id ($type)', async ({ id, type }) => {
    let authorize!: () => void
    stepUpRun.mockImplementationOnce(action => new Promise((resolve, reject) => {
      authorize = () => { Promise.resolve().then(action).then(resolve, reject) }
    }))
    listPlugins.mockResolvedValue([plugin, { ...plugin, id: 8, name: 'Other Transport' }])
    const wrapper = mountView(); await flushPromises(); await openConfig(wrapper)
    sendBridge(wrapper, type, 'save1', { config: { old: true } }); await flushPromises()
    await closeConfig(wrapper); await openConfig(wrapper, id === 7 ? 0 : 1)
    authorize(); await flushPromises()
    expect(savePluginConfig).not.toHaveBeenCalled()
    expect(recoverPluginConfig).not.toHaveBeenCalled()
    expect(showSuccess).not.toHaveBeenCalled(); expect(showError).not.toHaveBeenCalled()
    const post = vi.spyOn((wrapper.get('iframe').element as HTMLIFrameElement).contentWindow!, 'postMessage')
    sendBridge(wrapper, type, 'save1', { config: { current: true } }); await flushPromises()
    const expected = type === 'config.recover' ? recoverPluginConfig : savePluginConfig
    expect(expected).toHaveBeenCalledTimes(1)
    expect(expected).toHaveBeenCalledWith(id, { current: true })
    expect(post).toHaveBeenCalledWith(expect.objectContaining({ type: `${type}.result`, ok: true }), '*')
  })

  it('keeps a started save bound to its original plugin and drops the late reply', async () => {
    let finishSave!: (value: unknown) => void
    savePluginConfig.mockReturnValueOnce(new Promise(resolve => { finishSave = resolve }))
    listPlugins.mockResolvedValue([plugin, { ...plugin, id: 8, name: 'Other Transport' }])
    const wrapper = mountView(); await flushPromises(); await openConfig(wrapper)
    sendBridge(wrapper, 'config.save', 'save1', { config: { original: true } }); await flushPromises()
    expect(savePluginConfig).toHaveBeenCalledWith(7, { original: true })
    await closeConfig(wrapper); await openConfig(wrapper, 1)
    const iframe = wrapper.get('iframe').element as HTMLIFrameElement
    const post = vi.spyOn(iframe.contentWindow!, 'postMessage')
    finishSave({ original_secret: 'must-not-leak' }); await flushPromises()
    expect(post).not.toHaveBeenCalled(); expect(showSuccess).not.toHaveBeenCalled()
    expect(savePluginConfig).toHaveBeenCalledTimes(1)
  })

})
