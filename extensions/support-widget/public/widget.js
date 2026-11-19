(() => {
  'use strict';
  const token = new URLSearchParams(location.hash.slice(1)).get('bridge_token') || '';
  const get = (id) => document.getElementById(id);
  const pending = new Map();
  let sequence = 0, initialized = false, open = false, active = 'service', lastSize = '', launcher = null;
  let context = { locale: 'zh-CN', theme: 'light', viewport_width: 1280, viewport_height: 800, support_ticket_enabled: false };
  let config = {};
  const strings = {
    zh: { title: '联系与支持', description: '选择适合你的联系方式。', service: '客服', qq: 'QQ 群', help: '帮助', close: '关闭联系面板', open: '打开客服与交流群', channels: '联系方式', openService: '打开客服联系方式', openQQ: '打开 QQ 交流群', serviceLabel: '联系客服', copy: '复制', copyContact: '复制客服联系方式', copyQQ: '复制 QQ 群号', contact: '前往联系', ticket: '提交站内工单', ticketHint: '登录后可提交问题，并在站内查看回复。', qqTitle: '加入 QQ 交流群', join: '前往加入 QQ 群', qqLinkHint: '将打开你配置的加群页面。', qqCopyHint: '复制群号后，在 QQ 中搜索并申请加入。', copied: '已复制，可粘贴使用。', copyFailed: '复制失败，请选中上方内容手动复制。', linkFailed: '暂时无法打开链接，请稍后重试。' },
    en: { title: 'Contact & support', description: 'Choose a way to get in touch.', service: 'Support', qq: 'QQ group', help: 'Help', close: 'Close contact panel', open: 'Open support and community', channels: 'Contact options', openService: 'Open support contact', openQQ: 'Open QQ community', serviceLabel: 'Contact support', copy: 'Copy', copyContact: 'Copy support contact', copyQQ: 'Copy QQ group number', contact: 'Get in touch', ticket: 'Create a support ticket', ticketHint: 'Sign in to submit an issue and read replies on this site.', qqTitle: 'Join the QQ community', join: 'Open QQ group invitation', qqLinkHint: 'The configured invitation page will open.', qqCopyHint: 'Copy this group number, then search for it in QQ to request access.', copied: 'Copied. Ready to paste.', copyFailed: 'Copy failed. Select the contact above to copy it manually.', linkFailed: 'Could not open this link. Please retry.' }
  };
  const text = () => String(context.locale).startsWith('zh') ? strings.zh : strings.en;
  const trim = (value) => typeof value === 'string' ? value.trim() : '';
  const safeURL = (value, ticket = false) => {
    value = trim(value);
    if (ticket && value === '/support-tickets') return value;
    if (!/^https?:\/\//i.test(value) || [...value].some((c) => c.charCodeAt(0) < 33 || c.charCodeAt(0) === 127 || c === '\\')) return '';
    try { const url = new URL(value); return url.hostname && !url.username && !url.password ? value : ''; } catch { return ''; }
  };
  const channels = () => {
    const ticket = config.use_ticket === true && context.support_ticket_enabled === true;
    const serviceURL = safeURL(config.service_url, ticket);
    return { ticket, serviceURL, qqURL: safeURL(config.qq_group_url), serviceContact: trim(config.service_contact), qqNumber: trim(config.qq_group_number) };
  };
  const notify = (type, extra = {}) => {
    if (!token || parent === window) return;
    parent.postMessage({ source: 'sub2api-public-plugin', bridge_token: token, type, request_id: `support-${++sequence}`, ...extra }, '*');
  };
  const request = (type, extra) => new Promise((resolve, reject) => {
    if (!token || parent === window) { reject(new Error('Host unavailable')); return; }
    const requestID = `support-${++sequence}`;
    const timer = setTimeout(() => { pending.delete(requestID); reject(new Error('Host timeout')); }, 10000);
    pending.set(requestID, { resolve, reject, timer, type });
    parent.postMessage({ source: 'sub2api-public-plugin', bridge_token: token, type, request_id: requestID, ...extra }, '*');
  });
  function resize() {
    if (!initialized) return;
    const mobile = Number(context.viewport_width) < 640;
    const c = channels(), hasService = !!(c.serviceContact || c.serviceURL || c.ticket), hasQQ = !!(c.qqNumber || c.qqURL);
    const visible = hasService || hasQQ;
    const viewportHeight = Math.max(160, Number(context.viewport_height) || 800);
    document.documentElement.style.setProperty('--panel-max-height', `${Math.max(80, viewportHeight - (mobile ? 110 : 140))}px`);
    let width = mobile ? 102 : 86;
    let height = mobile ? 62 : Math.ceil(get('dock').getBoundingClientRect().height) + 16;
    if (open && visible) {
      width = mobile ? Math.max(44, Math.min(352, Number(context.viewport_width) - 16)) : 438;
      height = Math.max(height, Math.ceil(get('panel').getBoundingClientRect().height) + (mobile ? 74 : 16));
    }
    if (!visible) { width = 44; height = 44; }
    const signature = `${width}:${height}:${open && visible}`;
    if (signature !== lastSize) { lastSize = signature; notify('widget.resize', { width, height, open: open && visible }); }
  }
  function render() {
    const t = text(), c = channels();
    const hasService = !!(c.serviceContact || c.serviceURL || c.ticket), hasQQ = !!(c.qqNumber || c.qqURL);
    if (active === 'service' && !hasService) active = 'qq';
    if (active === 'qq' && !hasQQ) active = 'service';
    if (!hasService && !hasQQ) open = false;
    document.documentElement.classList.toggle('dark', context.theme === 'dark');
    document.documentElement.classList.toggle('mobile', Number(context.viewport_width) < 640);
    document.documentElement.lang = String(context.locale).startsWith('zh') ? 'zh-CN' : 'en';
    document.title = trim(config.title) || t.title;
    get('widget').hidden = !initialized || (!hasService && !hasQQ);
    get('panel').hidden = !open;
    get('panel-title').textContent = trim(config.title) || t.title;
    get('description').textContent = trim(config.description) || t.description;
    get('service-label').textContent = trim(config.service_label) || t.serviceLabel;
    document.querySelectorAll('[data-text]').forEach((element) => { element.textContent = t[element.dataset.text] || ''; });
    get('channels').hidden = !(hasService && hasQQ);
    get('channels').setAttribute('aria-label', t.channels);
    get('dock').setAttribute('aria-label', t.channels);
    get('tab-service').setAttribute('aria-pressed', String(active === 'service'));
    get('tab-qq').setAttribute('aria-pressed', String(active === 'qq'));
    get('service').hidden = active !== 'service' || !hasService;
    get('qq').hidden = active !== 'qq' || !hasQQ;
    get('service-contact-box').hidden = !c.serviceContact;
    get('service-contact').textContent = c.serviceContact;
    get('qq-number-box').hidden = !c.qqNumber;
    get('qq-number').textContent = c.qqNumber;
    get('service-url').hidden = !c.serviceURL || c.serviceURL === '/support-tickets';
    get('qq-url').hidden = !c.qqURL;
    get('ticket').hidden = !c.ticket;
    get('ticket-hint').hidden = !c.ticket;
    get('ticket').className = c.serviceURL && c.serviceURL !== '/support-tickets' ? 'secondary' : 'primary';
    get('qq-hint').textContent = c.qqURL ? t.qqLinkHint : t.qqCopyHint;
    get('launch-service').hidden = !hasService;
    get('launch-qq').hidden = !hasQQ;
    get('divider').hidden = !(hasService && hasQQ);
    get('launch-service').setAttribute('aria-expanded', String(open && active === 'service'));
    get('launch-qq').setAttribute('aria-expanded', String(open && active === 'qq'));
    get('mobile-launcher').setAttribute('aria-expanded', String(open));
    get('mobile-icon').setAttribute('href', open ? '#i-x' : '#i-chat');
    for (const [id, label] of [['close', t.close], ['launch-service', t.openService], ['launch-qq', t.openQQ], ['mobile-launcher', open ? t.close : t.open], ['copy-service', t.copyContact], ['copy-qq', t.copyQQ]]) get(id).setAttribute('aria-label', label);
    resize();
  }
  function close(restoreFocus = false) {
    open = false; get('feedback').textContent = ''; render();
    if (restoreFocus && launcher && !launcher.hidden) launcher.focus({ preventScroll: true });
  }
  function toggle(channel, event) {
    if (open && active === channel) return close(false);
    active = channel; open = true; launcher = event.currentTarget; get('feedback').textContent = ''; render();
    get('panel').focus({ preventScroll: true });
  }
  function select(channel) { active = channel; get('feedback').textContent = ''; render(); }
  async function copy(value, button) {
    button.disabled = true;
    try { const result = await request('clipboard.copy', { value }); get('feedback').textContent = result.message || text().copied; }
    catch { get('feedback').textContent = text().copyFailed; }
    finally { button.disabled = false; if (open) button.focus({ preventScroll: true }); resize(); }
  }
  async function visit(url, button) {
    button.disabled = true;
    try { await request('link.open', { url }); close(false); }
    catch { get('feedback').textContent = text().linkFailed; resize(); }
    finally { button.disabled = false; }
  }
  get('close').addEventListener('click', () => close(true));
  get('launch-service').addEventListener('click', (event) => toggle('service', event));
  get('launch-qq').addEventListener('click', (event) => toggle('qq', event));
  get('mobile-launcher').addEventListener('click', (event) => { if (open) close(false); else { const c = channels(); toggle(c.serviceContact || c.serviceURL || c.ticket ? 'service' : 'qq', event); } });
  get('tab-service').addEventListener('click', () => select('service'));
  get('tab-qq').addEventListener('click', () => select('qq'));
  get('copy-service').addEventListener('click', (event) => void copy(channels().serviceContact, event.currentTarget));
  get('copy-qq').addEventListener('click', (event) => void copy(channels().qqNumber, event.currentTarget));
  get('service-url').addEventListener('click', (event) => void visit(channels().serviceURL, event.currentTarget));
  get('qq-url').addEventListener('click', (event) => void visit(channels().qqURL, event.currentTarget));
  get('ticket').addEventListener('click', (event) => void visit('/support-tickets', event.currentTarget));
  document.addEventListener('keydown', (event) => { if (event.key === 'Escape' && open) { event.preventDefault(); close(true); } });
  document.addEventListener('pointerdown', (event) => { if (open && !get('panel').contains(event.target) && !get('dock').contains(event.target) && !get('mobile-launcher').contains(event.target)) close(false); });
  window.addEventListener('message', (event) => {
    const data = event.data;
    if (event.source !== parent || !data || data.source !== 'sub2api-public-host' || !token || data.bridge_token !== token) return;
    if (data.type === 'widget.init' && data.config && typeof data.config === 'object' && !Array.isArray(data.config)) {
      context = { ...context, locale: data.locale || 'zh-CN', theme: data.theme, viewport_width: data.viewport_width, viewport_height: data.viewport_height, support_ticket_enabled: data.support_ticket_enabled };
      config = data.config; initialized = true; clearInterval(readyTimer); render(); return;
    }
    if (data.type === 'widget.close') { close(false); return; }
    const entry = pending.get(data.request_id);
    if (!entry || data.type !== `${entry.type}.result`) return;
    pending.delete(data.request_id); clearTimeout(entry.timer);
    if (data.ok === true) entry.resolve(data); else entry.reject(new Error(data.message || 'Host rejected request'));
  });
  const observer = new ResizeObserver(resize); observer.observe(get('panel')); observer.observe(get('dock'));
  let attempts = 0;
  const readyTimer = setInterval(() => { if (initialized || ++attempts >= 10) clearInterval(readyTimer); else notify('widget.ready'); }, 500);
  notify('widget.ready');
  window.addEventListener('pagehide', () => { observer.disconnect(); clearInterval(readyTimer); for (const entry of pending.values()) { clearTimeout(entry.timer); entry.reject(new Error('Plugin unloaded')); } pending.clear(); }, { once: true });
})();
