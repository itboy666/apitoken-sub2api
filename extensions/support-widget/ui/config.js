(() => {
  'use strict';
  const get = (id) => document.getElementById(id);
  const token = new URLSearchParams(location.hash.slice(1)).get('bridge_token') || '';
  const pending = new Map();
  const keys = ['title', 'description', 'service_label', 'service_contact', 'service_url', 'qq_group_number', 'qq_group_url'];
  const defaults = { enabled: true, title: '联系与支持', description: '', service_label: '在线客服', service_contact: '', service_url: '', qq_group_number: '', qq_group_url: '', use_ticket: true };
  let sequence = 0, loaded = false, saving = false, saved = null, language = 'zh', success = false, recovering = false, loading = false, recoveryAllowed = false;
  const translations = {
    zh: { title: '客服浮窗配置', subtitle: '让访客在网站右侧找到客服与 QQ 群。', lifecycle: '启用、停用和卸载请使用插件管理页面的操作按钮。', loading: '正在读取插件配置…', retry: '重新读取', recover: '重新填写联系方式', recoveryTitle: '配置无法读取', recoveryWarning: '旧配置未能读取或解密。确认后将打开空白联系方式表单；只有点击保存才会替换本插件的整份旧配置。旧值不会自动恢复，请先重试或核对备份。', cancelRecovery: '返回', confirmRecovery: '确认并重新填写', recoveryBanner: '恢复模式：保存会替换无法读取的旧配置，请核对联系方式和链接。', recoveryHint: '请重新填写联系方式；保存将替换无法读取的旧配置。', panelTitle: '面板标题', serviceLabel: '客服入口名称', description: '欢迎语（可选）', contact: '客服联系方式（可选）', contactHint: '访客可以一键复制。', serviceURL: '客服链接（可选）', urlHint: '填写完整的 HTTP / HTTPS 链接。', group: 'QQ 群号（可选）', groupURL: '加群链接（可选）', ticket: '显示网站工单入口', ticketHint: '网站工单功能开启时，访客可登录后提交问题。', preview: '面板预览', qqLabel: 'QQ 交流群', empty: '添加联系方式后，这里会显示入口。', publicHint: '插件启用后，这些联系方式将对网站访客公开。', save: '保存插件配置', saving: '正在保存…', saved: '已保存，启用中的插件将自动更新。', hint: '配置独立保存在本插件中。', defaultTitle: '联系与支持', defaultService: '在线客服', defaultDescription: '在这里找到我们，交流项目与使用问题。', linkReady: '已设置访问链接', emptyError: '请至少填写一种联系方式，或选择显示网站工单入口。', groupError: 'QQ 群号码需为 5–12 位数字。', urlError: '请使用不含用户名、密码或控制字符的 HTTP / HTTPS 链接。', loadError: '暂时无法读取插件配置，请重试。', saveError: '保存失败，修改内容已保留，请重试。', disconnected: '请从 Sub2API 的插件管理页面打开配置。', descriptionPlaceholder: '我们自己也在用，有问题一起解决。', contactPlaceholder: 'QQ 号、微信号或邮箱', groupPlaceholder: '5–12 位群号码' },
    en: { title: 'Support widget settings', subtitle: 'Help visitors find support and your QQ community.', lifecycle: 'Use the plugin manager to enable, disable or uninstall this plugin.', loading: 'Loading plugin configuration…', retry: 'Retry', recover: 'Re-enter contacts', recoveryTitle: 'Configuration unavailable', recoveryWarning: 'The previous configuration could not be read or decrypted. Confirming opens blank contact fields. Only saving replaces this plugin’s entire previous configuration. Old values are not recovered automatically; retry or check a backup first.', cancelRecovery: 'Back', confirmRecovery: 'Confirm and re-enter', recoveryBanner: 'Recovery mode: saving replaces the unreadable configuration. Review contacts and links.', recoveryHint: 'Re-enter your contacts. Saving replaces the unreadable configuration.', panelTitle: 'Panel title', serviceLabel: 'Support label', description: 'Welcome message (optional)', contact: 'Support contact (optional)', contactHint: 'Visitors can copy this contact.', serviceURL: 'Support URL (optional)', urlHint: 'Enter a full HTTP or HTTPS URL.', group: 'QQ group number (optional)', groupURL: 'Join-group URL (optional)', ticket: 'Show support tickets', ticketHint: 'When site tickets are enabled, visitors can sign in to submit a question.', preview: 'PANEL PREVIEW', qqLabel: 'QQ community', empty: 'Add a contact to display an entry here.', publicHint: 'These contacts are public to site visitors while this plugin is enabled.', save: 'Save configuration', saving: 'Saving…', saved: 'Saved. An enabled widget will update automatically.', hint: 'Configuration is saved independently in this plugin.', defaultTitle: 'Contact & support', defaultService: 'Support', defaultDescription: 'Get in touch about your projects and questions.', linkReady: 'Link configured', emptyError: 'Add a contact or select the support ticket entry.', groupError: 'QQ group numbers must contain 5–12 digits.', urlError: 'Use HTTP or HTTPS links without usernames, passwords or control characters.', loadError: 'Could not load plugin configuration. Please retry.', saveError: 'Could not save. Your edits are preserved; please retry.', disconnected: 'Open this configuration from the Sub2API plugin manager.', descriptionPlaceholder: 'We use this too. Let’s work through questions together.', contactPlaceholder: 'QQ, WeChat or email', groupPlaceholder: '5–12 digits' }
  };
  const text = () => translations[language];
  const post = (type, extra = {}) => { if (token && parent !== window) parent.postMessage({ source: 'sub2api-plugin-ui', bridge_token: token, type, request_id: `support-config-${++sequence}`, ...extra }, '*'); };
  const request = (type, extra = {}) => new Promise((resolve, reject) => {
    if (!token || parent === window) { reject(new Error(text().disconnected)); return; }
    const requestID = `support-config-${++sequence}`;
    const timer = setTimeout(() => { pending.delete(requestID); reject(new Error('Host timeout')); }, type === 'config.save' || type === 'config.recover' ? 120000 : 30000);
    pending.set(requestID, { resolve, reject, timer, type });
    parent.postMessage({ source: 'sub2api-plugin-ui', bridge_token: token, type, request_id: requestID, ...extra }, '*');
  });
  function read() {
    const config = { enabled: true, use_ticket: get('use_ticket').checked };
    for (const key of keys) config[key] = get(key).value.trim();
    return config;
  }
  function fill(raw) {
    const config = { ...defaults, ...raw };
    for (const key of keys) get(key).value = typeof config[key] === 'string' ? config[key] : defaults[key];
    get('use_ticket').checked = config.use_ticket === true;
  }
  const dirty = () => saved === null ? recovering : JSON.stringify(read()) !== JSON.stringify(saved);
  function refresh() {
    const t = text(), config = read();
    document.documentElement.lang = language === 'zh' ? 'zh-CN' : 'en';
    document.title = t.title;
    document.querySelectorAll('[data-copy]').forEach((element) => { element.textContent = t[element.dataset.copy] || ''; });
    get('language').value = language;
    get('description').placeholder = t.descriptionPlaceholder;
    get('service_contact').placeholder = t.contactPlaceholder;
    get('qq_group_number').placeholder = t.groupPlaceholder;
    get('save').textContent = saving ? t.saving : t.save;
    get('save').disabled = !loaded || saving || !dirty();
    get('fields').disabled = saving;
    get('status').textContent = success && !dirty() ? t.saved : recovering ? t.recoveryHint : t.hint;
    get('preview-title').textContent = config.title || t.defaultTitle;
    get('preview-description').textContent = config.description || t.defaultDescription;
    const service = !!(config.service_contact || config.service_url || config.use_ticket);
    const qq = !!(config.qq_group_number || config.qq_group_url);
    get('preview-service').hidden = !service;
    get('preview-service-label').textContent = config.service_label || t.defaultService;
    get('preview-contact').textContent = config.service_contact || (config.service_url ? t.linkReady : t.ticket);
    get('preview-group').hidden = !qq;
    get('preview-qq').textContent = config.qq_group_number || t.linkReady;
    get('preview-empty').hidden = service || qq;
  }
  function error(message) { get('error').textContent = message; get('error').hidden = false; get('error').focus({ preventScroll: false }); }
  function validURL(value, ticket = false) {
    if (!value || (ticket && value === '/support-tickets')) return true;
    if (!/^https?:\/\//i.test(value) || [...value].some((c) => c.charCodeAt(0) <= 32 || c.charCodeAt(0) === 127 || c === '\\')) return false;
    try { const url = new URL(value); const decoded = decodeURIComponent(value); return Boolean(url.hostname) && !url.username && !url.password && ![...decoded].some((c) => c.charCodeAt(0) < 32 || c.charCodeAt(0) === 127); } catch { return false; }
  }
  async function load() {
    if (loading || saving) return;
    loading = true; recoveryAllowed = false;
    get('loading').hidden = false; get('retry').hidden = true; get('recover').hidden = true; get('recovery-confirm').hidden = true; get('recovery-banner').hidden = true; get('error').hidden = true;
    recovering = false; loaded = false; saved = null; get('form').hidden = true; refresh();
    try {
      const response = await request('config.load');
      if (response.context) {
        language = String(response.context.locale || 'zh').startsWith('zh') ? 'zh' : 'en';
        document.documentElement.classList.toggle('dark', response.context.theme === 'dark');
      }
      fill(response.config || {}); saved = read(); loaded = true; get('form').hidden = false; refresh();
    } catch { error(token && parent !== window ? text().loadError : text().disconnected); get('retry').hidden = false; recoveryAllowed = Boolean(token && parent !== window); get('recover').hidden = !recoveryAllowed; }
    finally { loading = false; get('loading').hidden = true; }
  }
  async function save() {
    if (!loaded || saving || !dirty()) return;
    get('error').hidden = true; success = false;
    const config = read();
    if (!config.use_ticket && !config.service_contact && !config.service_url && !config.qq_group_number && !config.qq_group_url) return error(text().emptyError);
    if (config.qq_group_number && !/^[0-9]{5,12}$/.test(config.qq_group_number)) return error(text().groupError);
    if (!validURL(config.service_url, true) || !validURL(config.qq_group_url)) return error(text().urlError);
    saving = true; refresh();
    const requestType = recovering ? 'config.recover' : 'config.save';
    try { const response = await request(requestType, { config }); fill(response.config || config); saved = read(); success = true; recovering = false; get('recovery-banner').hidden = true; }
    catch { error(text().saveError); }
    finally { saving = false; refresh(); }
  }
  window.addEventListener('message', (event) => {
    const data = event.data;
    if (event.source !== parent || !data || data.source !== 'sub2api-plugin-host' || !token || data.bridge_token !== token) return;
    const item = pending.get(data.request_id);
    if (!item || data.type !== `${item.type}.result`) return;
    pending.delete(data.request_id); clearTimeout(item.timer);
    if (data.ok === true) item.resolve(data); else item.reject(new Error(data.error || data.message || 'Request rejected'));
  });
  get('language').addEventListener('change', () => { language = get('language').value === 'en' ? 'en' : 'zh'; refresh(); });
  get('form').addEventListener('input', refresh);
  // The host deliberately excludes allow-forms. Save via the message bridge,
  // without invoking the browser's sandbox-blocked native form submission.
  get('save').addEventListener('click', () => void save());
  get('form').addEventListener('submit', (event) => event.preventDefault());
  get('form').addEventListener('keydown', (event) => {
    if (event.key !== 'Enter' || event.isComposing || event.repeat || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return;
    if (!(event.target instanceof HTMLInputElement) || event.target.type === 'checkbox') return;
    event.preventDefault();
    void save();
  });
  get('retry').addEventListener('click', () => void load());
  get('recover').addEventListener('click', () => {
    if (!recoveryAllowed || loaded || loading) return;
    get('error').hidden = true;
    get('recovery-confirm').hidden = false;
    get('confirm-recovery').focus();
  });
  get('cancel-recovery').addEventListener('click', () => {
    get('recovery-confirm').hidden = true;
    get('recover').focus();
  });
  get('confirm-recovery').addEventListener('click', () => {
    if (!recoveryAllowed || get('recovery-confirm').hidden || loading) return;
    recoveryAllowed = false;
    fill(defaults);
    recovering = true;
    loaded = true;
    saved = null;
    success = false;
    get('recovery-confirm').hidden = true;
    get('retry').hidden = true;
    get('recover').hidden = true;
    get('recovery-banner').hidden = false;
    get('form').hidden = false;
    refresh();
    get('service_contact').focus();
  });
  let lastHeight = 0;
  const observer = new ResizeObserver(() => { const height = Math.ceil(document.querySelector('.settings').getBoundingClientRect().height); if (height !== lastHeight) { lastHeight = height; post('ui.resize', { height }); } });
  observer.observe(document.querySelector('.settings'));
  document.documentElement.classList.toggle('dark', matchMedia('(prefers-color-scheme: dark)').matches);
  fill(defaults); refresh(); post('sub2api.plugin.ready'); void load();
  window.addEventListener('pagehide', () => { observer.disconnect(); for (const item of pending.values()) { clearTimeout(item.timer); item.reject(new Error('Plugin unloaded')); } pending.clear(); }, { once: true });
})();
