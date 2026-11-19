import { buildApiUrl } from './url'
import type { PublicPluginDescriptor } from '@/types/publicPlugins'

export async function getPublicPlugins(signal?: AbortSignal): Promise<PublicPluginDescriptor[]> {
  // This public endpoint deliberately receives neither session cookies nor bearer tokens.
  const response = await fetch(buildApiUrl('/plugins/public'), { credentials: 'omit', cache: 'no-store', signal })
  if (!response.ok) throw new Error('Public plugins unavailable')
  const body = await response.json()
  if (body?.code !== 0 || !Array.isArray(body.data)) throw new Error('Invalid public plugin response')
  return body.data
}

/** Keep iframe loading restricted to the host's content-addressed public plugin assets. */
export function publicPluginEntrypoint(plugin: PublicPluginDescriptor): string {
  if (!Number.isSafeInteger(plugin.id) || plugin.id <= 0 || typeof plugin.entrypoint_url !== 'string') return ''
  const expected = new RegExp(`^/api/v1/plugins/public/${plugin.id}/assets/[a-f0-9]{64}/public/index\\.html$`)
  return expected.test(plugin.entrypoint_url) ? plugin.entrypoint_url : ''
}

export function publicPluginURL(value: unknown): string {
  if (typeof value !== 'string') return ''
  const trimmed = value.trim()
  if (!/^https?:\/\//i.test(trimmed)) return ''
  try {
    const decoded = decodeURIComponent(trimmed)
    if ([...decoded].some(char => char.charCodeAt(0) < 32 || char.charCodeAt(0) === 127 || char === '\\')) return ''
    const url = new URL(trimmed)
    if (!url.hostname || url.username || url.password) return ''
    return url.toString()
  } catch { return '' }
}
