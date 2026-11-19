import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import type { VersionInfo } from '@/api/admin/system'

const update = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin/system', () => ({ performUpdate: update, restartService: vi.fn(), getRollbackVersions: vi.fn(), rollback: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/stores', () => ({ useAuthStore: () => ({ isAdmin: true }), useAppStore: () => state }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copied: false, copyToClipboard: vi.fn() }) }))

import VersionBadge from '../VersionBadge.vue'

const state = reactive({
  currentVersion: '2.23.0-apitoken.8', latestVersion: '', hasUpdate: false,
  buildType: 'release', versionLoading: false, releaseInfo: null,
  versionInfo: null as VersionInfo | null,
  fetchVersion: vi.fn(), clearVersionCache: vi.fn()
})
let wrapper: VueWrapper | undefined
beforeEach(() => {
  vi.clearAllMocks()
  Object.assign(state, { currentVersion: '2.23.0-apitoken.8', latestVersion: '', hasUpdate: false, versionInfo: null })
  update.mockResolvedValue({ need_restart: true })
})
afterEach(() => wrapper?.unmount())

async function render(info: Partial<VersionInfo> = {}) {
  state.versionInfo = {
    current_version: state.currentVersion, latest_version: '2.23.0-apitoken.9',
    has_update: true, cached: false, build_type: 'release', update_channel: 'apitoken',
    update_repository: 'example/apitoken', upstream_version: '2.23.0', auto_update_enabled: true,
    ...info
  }
  state.hasUpdate = state.versionInfo.has_update
  state.latestVersion = state.versionInfo.latest_version
  wrapper = mount(VersionBadge, { global: { stubs: { Icon: true } } })
  await wrapper.find('button').trigger('click')
  await flushPromises()
  return wrapper
}

describe('combined release updates', () => {
  it('shows the upstream base and uses the configured combined channel', async () => {
    const page = await render()
    expect(page.text()).toContain('v2.23.0')
    expect(page.get('a[href="https://github.com/example/apitoken/releases"]').text()).toBe('example/apitoken')
    await page.findAll('button').find(button => button.text().includes('version.updateNow'))!.trigger('click')
    await flushPromises()
    expect(update).toHaveBeenCalledTimes(1)
    expect(page.text()).toContain('version.restartNow')
  })

  it('does not report latest or offer updates before a repository is connected', async () => {
    const page = await render({ update_repository: '', latest_version: '', has_update: false, auto_update_enabled: false, update_blocked_reason: 'custom_repository_required' })
    expect(page.text()).toContain('version.combinedSetupHint')
    expect(page.text()).not.toContain('version.upToDate')
    expect(page.text()).not.toContain('version.updateNow')
  })

  it('keeps network failures distinct from a successful latest-version check', async () => {
    const page = await render({ warning: 'network unavailable', has_update: false })
    expect(page.text()).toContain('version.checkUnavailable')
    expect(page.text()).not.toContain('version.upToDate')
  })

  it('does not install a cached update while verification is unavailable', async () => {
    const page = await render({ warning: 'Using cached data', cached: true })
    expect(page.text()).not.toContain('version.updateNow')
    expect(update).not.toHaveBeenCalled()
  })

  it('treats legacy custom builds as source-managed until the server confirms the channel', async () => {
    const page = await render({ auto_update_enabled: undefined, update_repository: undefined })
    expect(page.text()).not.toContain('version.updateNow')
    expect(page.text()).toContain('version.combinedSourceBuildHint')
  })
})
