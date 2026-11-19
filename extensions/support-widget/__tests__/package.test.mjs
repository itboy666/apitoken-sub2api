import assert from 'node:assert/strict'
import { test } from 'node:test'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
const require = createRequire(new URL('../../../frontend/package.json', import.meta.url))
const { JSDOM } = require('jsdom')
const root = new URL('../', import.meta.url)
const token = 'support-test-token'
const tick = () => new Promise(resolve => setTimeout(resolve, 0))
function setup(kind) {
  const html = readFileSync(new URL(`${kind}/index.html`, root), 'utf8')
  const source = readFileSync(new URL(`${kind}/${kind === 'public' ? 'widget' : 'config'}.js`, root), 'utf8')
  const dom = new JSDOM(html, { url: `https://example.test/${kind}/index.html#bridge_token=${token}`, runScripts: 'outside-only', pretendToBeVisual: true })
  const { window } = dom
  const messages = []
  const parent = { postMessage: message => messages.push(message) }
  Object.defineProperty(window, 'parent', { value: parent })
  window.ResizeObserver = class { observe() {} disconnect() {} }
  window.matchMedia = () => ({ matches: false })
  const prefix = kind === 'public' ? 'sub2api-public-host' : 'sub2api-plugin-host'
  const send = (data, override = {}) => window.dispatchEvent(new window.MessageEvent('message', { source: parent, data: { source: prefix, bridge_token: token, ...data }, ...override }))
  const get = id => window.document.getElementById(id)
  window.eval(source)
  return { window, messages, parent, send, get, close: () => window.close() }
}
const config = { enabled: true, title: '联系与支持', description: '我们自己也在用', service_label: '客服 QQ', service_contact: '123456789', service_url: '', qq_group_number: '987654321', qq_group_url: '', use_ticket: true }
function init(s, extra = {}) { s.send({ type: 'widget.init', config, locale: 'zh', theme: 'light', viewport_width: 1280, viewport_height: 900, support_ticket_enabled: true, ...extra }) }
function reply(s, request, extra = {}) { s.send({ type: `${request.type}.result`, request_id: request.request_id, ok: true, ...extra }) }
function last(s, type) { return s.messages.filter(message => message.type === type).at(-1) }
function edit(s, id, value) {
  s.get(id).value = value
  s.get(id).dispatchEvent(new s.window.Event('input', { bubbles: true }))
}

test('public widget accepts only its own parent and token for initialization', () => {
  const s = setup('public')
  try {
    s.send({ type: 'widget.init', config }, { source: null })
    assert.equal(s.get('widget').hidden, true)
    s.send({ type: 'widget.init', config, bridge_token: 'wrong' })
    assert.equal(s.get('widget').hidden, true)
    init(s)
    assert.equal(s.get('widget').hidden, false)
    assert.equal(s.get('panel').hidden, true)
    assert.equal(s.get('service-contact').textContent, '123456789')
    assert.equal(last(s, 'widget.resize').open, false)
  } finally { s.close() }
})
test('untrusted contact markup stays text and unsafe links stay hidden', () => {
  const s = setup('public')
  try {
    init(s, { config: { ...config, title: '<img src=x onerror=alert(1)>', service_url: 'javascript:alert(1)', qq_group_url: 'https://user:secret@example.test' } })
    assert.equal(s.get('panel-title').textContent, '<img src=x onerror=alert(1)>')
    assert.equal(s.get('panel-title').children.length, 0)
    assert.equal(s.get('service-url').hidden, true)
    assert.equal(s.get('qq-url').hidden, true)
  } finally { s.close() }
})
test('opening channels, Escape and host close restore dock state', () => {
  const s = setup('public')
  try {
    init(s)
    s.get('launch-qq').click()
    assert.equal(s.get('panel').hidden, false)
    assert.equal(s.get('qq').hidden, false)
    assert.equal(last(s, 'widget.resize').open, true)
    s.window.document.dispatchEvent(new s.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    assert.equal(s.get('panel').hidden, true)
    assert.equal(s.window.document.activeElement.id, 'launch-qq')
    s.get('launch-service').click()
    s.send({ type: 'widget.close' })
    assert.equal(s.get('panel').hidden, true)
    assert.equal(last(s, 'widget.resize').open, false)
  } finally { s.close() }
})
test('copy uses the selected contact and ignores unmatched responses', async () => {
  const s = setup('public')
  try {
    init(s)
    s.get('launch-qq').click()
    s.get('copy-qq').click()
    const request = last(s, 'clipboard.copy')
    assert.equal(request.value, '987654321')
    s.send({ type: 'clipboard.copy.result', request_id: 'not-pending', ok: true, message: 'wrong' })
    s.send({ type: 'link.open.result', request_id: request.request_id, ok: true, message: 'wrong' })
    await tick()
    assert.equal(s.get('feedback').textContent, '')
    assert.equal(s.get('copy-qq').disabled, true)
    reply(s, request)
    await tick()
    assert.match(s.get('feedback').textContent, /已复制/)
    assert.equal(s.get('copy-qq').disabled, false)
  } finally { s.close() }
})
test('ticket requires both flags; mobile and dark follow host context', () => {
  const s = setup('public')
  try {
    init(s, { support_ticket_enabled: false, viewport_width: 375, viewport_height: 812, theme: 'dark' })
    assert.equal(s.get('ticket').hidden, true)
    assert.equal(s.window.document.documentElement.classList.contains('dark'), true)
    assert.equal(s.window.document.documentElement.classList.contains('mobile'), true)
    s.get('mobile-launcher').click()
    assert.equal(last(s, 'widget.resize').width, 352)
    assert.equal(last(s, 'widget.resize').open, true)
    init(s, { config: { ...config, use_ticket: false } })
    assert.equal(s.get('ticket').hidden, true)
  } finally { s.close() }
})
test('admin save button uses the bridge without a sandbox-blocked form submission', async () => {
  const s = setup('ui')
  try {
    reply(s, last(s, 'config.load'), { config, context: { locale: 'zh', theme: 'dark' } })
    await tick()
    assert.equal(s.get('form').hidden, false)
    assert.equal(s.get('save').disabled, true)
    assert.equal(s.window.document.documentElement.classList.contains('dark'), true)
    let submissions = 0
    s.get('form').addEventListener('submit', () => { submissions++ })
    edit(s, 'service_contact', ' 135790246 ')
    assert.equal(s.get('save').type, 'button')
    s.get('save').click()
    const save = last(s, 'config.save')
    assert.equal(last(s, 'config.recover'), undefined)
    assert.equal(submissions, 0)
    assert.equal(save.config.service_contact, '135790246')
    assert.equal(save.config.enabled, true)
    assert.equal(s.get('fields').disabled, true)
    reply(s, save, { config: save.config })
    await tick()
    assert.equal(s.get('save').disabled, true)
    assert.match(s.get('status').textContent, /已保存/)
  } finally { s.close() }
})
test('admin rejects invalid groups and links, and retains edits after save failure', async () => {
  const s = setup('ui')
  try {
    reply(s, last(s, 'config.load'), { config })
    await tick()
    edit(s, 'qq_group_number', '123')
    s.get('save').click()
    assert.match(s.get('error').textContent, /5–12/)
    assert.equal(last(s, 'config.save'), undefined)
    edit(s, 'qq_group_number', '123456')
    edit(s, 'service_url', 'https://example.test/%0a')
    s.get('save').click()
    assert.match(s.get('error').textContent, /HTTP/)
    edit(s, 'service_url', '')
    s.get('save').click()
    const save = last(s, 'config.save')
    s.send({ type: 'config.save.result', request_id: save.request_id, ok: false, error: 'failure' })
    await tick()
    assert.equal(s.get('qq_group_number').value, '123456')
    assert.match(s.get('error').textContent, /修改内容已保留/)
    assert.equal(s.get('save').disabled, false)
  } finally { s.close() }
})
test('Enter saves text fields once without submitting, while textarea and IME remain unchanged', async () => {
  const s = setup('ui')
  try {
    reply(s, last(s, 'config.load'), { config })
    await tick()
    edit(s, 'service_contact', '135790246')
    const key = (id, extra = {}) => {
      const event = new s.window.KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true, ...extra })
      s.get(id).dispatchEvent(event)
      return event
    }
    assert.equal(key('description').defaultPrevented, false)
    assert.equal(key('service_contact', { isComposing: true }).defaultPrevented, false)
    assert.equal(key('use_ticket').defaultPrevented, false)
    assert.equal(last(s, 'config.save'), undefined)
    assert.equal(key('service_contact').defaultPrevented, true)
    assert.equal(s.messages.filter(message => message.type === 'config.save').length, 1)
    key('service_contact')
    s.get('save').click()
    assert.equal(s.messages.filter(message => message.type === 'config.save').length, 1)
    reply(s, last(s, 'config.save'), { config: last(s, 'config.save').config })
    await tick()
    assert.match(s.get('status').textContent, /已保存/)
  } finally { s.close() }
})
test('failed load requires explicit recovery confirmation before replacement config can save', async () => {
  const s = setup('ui')
  try {
    const load = last(s, 'config.load')
    s.get('confirm-recovery').click()
    assert.equal(s.get('form').hidden, true)
    s.send({ type: 'config.load.result', request_id: load.request_id, ok: false, error: 'failure' })
    await tick()
    assert.equal(s.get('form').hidden, true)
    assert.equal(s.get('retry').hidden, false)
    assert.equal(s.get('recover').hidden, false)
    s.get('save').click()
    s.get('service_contact').dispatchEvent(new s.window.KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }))
    assert.equal(last(s, 'config.save'), undefined)
    assert.equal(last(s, 'config.recover'), undefined)

    s.get('recover').click()
    assert.equal(s.get('recovery-confirm').hidden, false)
    assert.match(s.get('recovery-confirm').textContent, /只有点击保存才会替换本插件的整份旧配置/)
    s.get('cancel-recovery').click()
    s.get('confirm-recovery').click()
    assert.equal(s.get('form').hidden, true)
    assert.equal(last(s, 'config.save'), undefined)
    assert.equal(last(s, 'config.recover'), undefined)

    s.get('recover').click()
    s.get('confirm-recovery').click()
    assert.equal(s.get('form').hidden, false)
    assert.equal(s.get('save').disabled, false)
    assert.equal(s.get('service_contact').value, '')
    assert.equal(s.get('qq_group_number').value, '')
    assert.equal(s.get('description').value, '')
    assert.equal(s.get('service_label').value, '在线客服')
    assert.equal(s.get('service_url').value, '')
    assert.equal(s.get('qq_group_url').value, '')
    assert.equal(last(s, 'config.save'), undefined)
    assert.equal(last(s, 'config.recover'), undefined)
    edit(s, 'service_contact', config.service_contact)
    edit(s, 'qq_group_number', config.qq_group_number)
    edit(s, 'description', config.description)
    s.get('save').click()
    let recoverySave = last(s, 'config.recover')
    assert.ok(recoverySave)
    assert.equal(last(s, 'config.save'), undefined)
    assert.equal(recoverySave.config.service_contact, config.service_contact)
    assert.equal(recoverySave.config.qq_group_number, config.qq_group_number)
    assert.equal(recoverySave.config.description, config.description)
    s.send({ type: 'config.save.result', request_id: recoverySave.request_id, ok: true, config: recoverySave.config })
    await tick()
    assert.equal(s.get('fields').disabled, true)
    assert.equal(s.get('recovery-banner').hidden, false)
    s.send({ type: 'config.recover.result', request_id: recoverySave.request_id, ok: false, error: 'failure' })
    await tick()
    assert.equal(s.get('recovery-banner').hidden, false)
    assert.equal(s.get('service_contact').value, config.service_contact)
    assert.equal(s.get('save').disabled, false)
    s.get('save').click()
    recoverySave = last(s, 'config.recover')
    assert.ok(recoverySave)
    reply(s, recoverySave, { config: recoverySave.config })
    await tick()
    assert.equal(s.get('recovery-banner').hidden, true)
    assert.match(s.get('status').textContent, /已保存/)
    const recoveries = s.messages.filter(message => message.type === 'config.recover').length
    edit(s, 'description', 'Updated after recovery')
    s.get('save').click()
    const normalSave = last(s, 'config.save')
    assert.equal(normalSave.config.description, 'Updated after recovery')
    assert.equal(s.messages.filter(message => message.type === 'config.recover').length, recoveries)
    reply(s, normalSave, { config: normalSave.config })
    await tick()
  } finally { s.close() }
})
