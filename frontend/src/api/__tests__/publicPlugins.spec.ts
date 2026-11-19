import { afterEach, describe, expect, it, vi } from 'vitest'
import { getPublicPlugins, publicPluginURL, publicPluginEntrypoint } from '../publicPlugins'
import type { PublicPluginDescriptor } from '@/types/publicPlugins'
afterEach(() => { vi.unstubAllGlobals() })
describe('public plugins transport', () => {
  it('fetches without any credentials and unwraps the public response', async () => {
    const fetcher = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ code: 0, data: [] }) })
    vi.stubGlobal('fetch', fetcher)
    expect(await getPublicPlugins()).toEqual([])
    expect(fetcher).toHaveBeenCalledWith('/api/v1/plugins/public', expect.objectContaining({ credentials: 'omit', cache: 'no-store' }))
  })
  it('rejects invalid responses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({ code: 1, data: [] }) }))
    await expect(getPublicPlugins()).rejects.toThrow()
  })
  it.each(['javascript:alert(1)', 'https://user:pass@example.org/', 'https://example.org/%0a', 'https://example.org/\\path'])('rejects unsafe URL %s', value => { expect(publicPluginURL(value)).toBe('') })
  it('only accepts an exact public content-addressed entrypoint for the declared plugin', () => {
    const descriptor = { id: 7, entrypoint_url: `/api/v1/plugins/public/7/assets/${'a'.repeat(64)}/public/index.html` } as PublicPluginDescriptor
    expect(publicPluginEntrypoint(descriptor)).toBe(descriptor.entrypoint_url)
    expect(publicPluginEntrypoint({ ...descriptor, id: 8 })).toBe('')
    expect(publicPluginEntrypoint({ ...descriptor, entrypoint_url: descriptor.entrypoint_url + '?redirect=evil' })).toBe('')
  })
})
