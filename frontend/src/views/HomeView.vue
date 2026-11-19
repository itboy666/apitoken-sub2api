<template>
  <!-- An administrator's custom page always takes precedence. -->
  <div v-if="hasHomeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      :title="siteName"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <div v-else v-html="homeContent"></div>
  </div>

  <div v-else class="public-home" :data-testid="compactHomeEnabled ? 'compact-home' : undefined">
    <a class="skip-link" href="#home-main">{{ copy.skip }}</a>
    <header class="home-header">
      <nav class="home-container home-nav" :aria-label="copy.navigation">
        <a href="/home" class="home-brand" :aria-label="siteName">
          <img :src="siteLogo || '/logo.svg'" alt="" width="36" height="36" />
          <span>{{ siteName }}</span>
        </a>
        <div class="nav-links">
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="nav-text-link">
            {{ t('home.docs') }}
            <Icon name="externalLink" size="sm" aria-hidden="true" />
          </a>
          <router-link v-if="showModelPlazaEntry" to="/model-plaza" class="nav-text-link">
            {{ t('nav.modelPlaza') }}
          </router-link>
          <div class="nav-tools">
            <LocaleSwitcher />
            <button
              type="button"
              class="theme-button"
              :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
              :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
              @click="toggleTheme"
            >
              <Icon :name="isDark ? 'sun' : 'moon'" size="md" aria-hidden="true" />
            </button>
          </div>
          <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="header-login">
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
            <Icon name="arrowRight" size="sm" aria-hidden="true" />
          </router-link>
        </div>
      </nav>
    </header>

    <main id="home-main" class="home-container">
      <section class="home-hero" aria-labelledby="hero-title">
        <div class="hero-copy">
          <p class="eyebrow"><span aria-hidden="true"></span> {{ copy.eyebrow }}</p>
          <h1 id="hero-title">{{ copy.title }}<br /><span>{{ copy.titleAccent }}</span></h1>
          <p class="hero-description">{{ siteSubtitle }}</p>
          <div class="hero-actions">
            <router-link :to="primaryDestination" class="primary-action">
              {{ isAuthenticated ? t('home.goToDashboard') : registrationEnabled ? copy.start : t('home.login') }}
              <Icon name="arrowRight" size="md" aria-hidden="true" />
            </router-link>
            <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="secondary-action">
              {{ copy.docs }}
              <Icon name="externalLink" size="sm" aria-hidden="true" />
            </a>
            <router-link v-else-if="showModelPlazaEntry" to="/model-plaza" class="secondary-action">
              {{ t('nav.modelPlaza') }}
              <Icon name="arrowRight" size="sm" aria-hidden="true" />
            </router-link>
            <router-link v-else-if="!isAuthenticated && registrationEnabled" to="/login" class="secondary-action">
              {{ copy.existing }}
            </router-link>
          </div>
          <div class="hero-details">
            <span><Icon name="check" size="sm" aria-hidden="true" />{{ copy.detailOne }}</span>
            <span><Icon name="check" size="sm" aria-hidden="true" />{{ copy.detailTwo }}</span>
          </div>
        </div>

        <div class="terminal-container connection-panel" :aria-label="copy.illustration">
          <div class="panel-heading">
            <span class="panel-label">{{ siteName }} <span class="panel-slash">/</span> BUILT &amp; USED</span>
            <span class="example-label">{{ copy.example }}</span>
          </div>
          <div class="connection-map" aria-hidden="true">
            <svg class="connection-lines" viewBox="0 0 500 190" fill="none" preserveAspectRatio="none">
              <path d="M90 88H218M410 88H282" />
              <path class="connection-flow" d="M90 88H218M410 88H282" />
              <path class="connection-arrow" d="m210 82 8 6-8 6m80-12-8 6 8 6" />
            </svg>
            <div class="project-node project-node-own">
              <div class="app-node"><Icon name="terminal" size="lg" /></div>
              <span>{{ copy.ourProject }}</span><small>{{ copy.usedFirst }}</small>
            </div>
            <div class="gateway-node"><img :src="siteLogo || '/logo.svg'" alt="" width="52" height="52" /></div>
            <div class="project-node project-node-yours">
              <div class="app-node"><Icon name="cube" size="lg" /></div>
              <span>{{ copy.yourProject }}</span><small>{{ copy.sharedNext }}</small>
            </div>
          </div>
          <div class="code-card">
            <div class="code-toolbar"><span><i></i><i></i><i></i></span><span>client.config</span><span>API</span></div>
            <div class="code-body" :aria-label="copy.configuration">
              <div><span class="code-number">01</span><code><span class="code-keyword">const</span> client = <span class="code-keyword">new</span> OpenAI({</code></div>
              <div><span class="code-number">02</span><code>&nbsp; baseURL: <span class="code-string">"{{ apiBaseUrl }}"</span>,</code></div>
              <div><span class="code-number">03</span><code>&nbsp; apiKey: <span class="code-string">"YOUR_API_KEY"</span></code></div>
              <div><span class="code-number">04</span><code>});</code></div>
            </div>
          </div>
          <div class="panel-footer"><span class="connection-mark" aria-hidden="true"><Icon name="link" size="sm" /></span><span>{{ copy.panelCaption }}</span><span class="panel-index">REST / SDK</span></div>
        </div>
      </section>

      <section class="capabilities" :aria-label="copy.capabilities">
        <article v-for="feature in features" :key="feature.title" class="capability">
          <div class="feature-icon"><Icon :name="feature.icon" size="lg" aria-hidden="true" /></div>
          <div><h2>{{ feature.title }}</h2><p>{{ feature.description }}</p></div>
        </article>
      </section>

      <section v-if="!compactHomeEnabled" class="getting-started" aria-labelledby="start-title">
        <div class="section-intro"><p class="eyebrow">BUILT FOR YOUR WORKFLOW</p><h2 id="start-title">{{ copy.stepsTitle }}</h2><p>{{ copy.stepsDescription }}</p></div>
        <ol class="setup-steps">
          <li v-for="(step, index) in steps" :key="step.title"><span class="step-index">0{{ index + 1 }}</span><div><h3>{{ step.title }}</h3><p>{{ step.description }}</p></div></li>
        </ol>
      </section>
    </main>

    <footer class="home-footer">
      <div class="home-container footer-content"><p>&copy; {{ currentYear }} {{ siteName }}</p><span class="footer-note">{{ copy.footer }}</span><a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.docs') }}<Icon name="externalLink" size="sm" aria-hidden="true" /></a></div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'

const { t, locale } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()
const chinese = computed(() => !locale || locale.value.startsWith('zh'))
const copy = computed(() => chinese.value ? {
  skip: '跳转到主要内容', navigation: '主导航',
  eyebrow: '先自用，再开放 · BUILT FOR OUR OWN',
  title: '自己在用，', titleAccent: '才放心分享。',
  description: '这首先是我们自己项目在用的 AI 中转。把日常开发需要的接入、密钥和用量管理做好，再开放给同样认真做产品的你。',
  start: '开始使用', docs: '阅读接入文档', existing: '已有账号？登录',
  detailOne: '自己的项目先用', detailTwo: '兼容 OpenAI SDK',
  illustration: '我们自己的项目与用户项目通过同一个平台接入 API 的示意图',
  example: '接入示意', ourProject: '我们的项目', yourProject: '你的项目', usedFirst: '先自用', sharedNext: '再开放', configuration: '客户端配置示例',
  panelCaption: '同一份认真，用在你的项目里', capabilities: '我们如何做这个平台',
  stepsTitle: '下一次接入，也可以更简单。', stepsDescription: '从我们自己的开发习惯出发，让你的项目也能顺手接入。',
  footer: '自己认真用，也认真与你分享。',
} : {
  skip: 'Skip to main content', navigation: 'Main navigation',
  eyebrow: 'OUR PROJECTS FIRST · YOURS NEXT',
  title: 'Built for our work.', titleAccent: 'Shared with yours.',
  description: 'This is the AI gateway we use in our own projects. We build the integration, key management and usage tools we need every day, then share them with people who care about what they build.',
  start: 'Get started', docs: 'Read the docs', existing: 'Already have an account? Sign in',
  detailOne: 'Used in our own projects', detailTwo: 'OpenAI SDK compatible',
  illustration: 'Our projects and your projects connecting through the same API platform',
  example: 'Connection example', ourProject: 'Our projects', yourProject: 'Your projects', usedFirst: 'Used first', sharedNext: 'Shared next', configuration: 'Client configuration example',
  panelCaption: 'The same care, for your next project', capabilities: 'How we build this platform',
  stepsTitle: 'Make your next integration simpler.', stepsDescription: 'Built around our own development workflow, ready to become part of yours.',
  footer: 'Made for our projects. Shared with care.',
})
const features = computed(() => chinese.value ? [
  { icon: 'terminal' as const, title: '从自用出发', description: '先满足自己项目的日常接入，再把这份体验开放给你。' },
  { icon: 'shieldKey' as const, title: '为开发而做', description: '兼容熟悉的 SDK，按项目管理密钥，把精力留给产品。' },
  { icon: 'chart' as const, title: '每一笔都清楚', description: '请求记录、用量与余额，在控制台里随时查看。' },
] : [
  { icon: 'terminal' as const, title: 'Our own projects first', description: 'Built around our daily integration needs, then opened to yours.' },
  { icon: 'shieldKey' as const, title: 'Made for building', description: 'Familiar SDKs and project keys help keep your focus on the product.' },
  { icon: 'chart' as const, title: 'See what you use', description: 'Check requests, usage and account balance in your dashboard.' },
])
const steps = computed(() => chinese.value ? [
  { title: '进入控制台', description: '登录你的账号，查看可用的服务与账户信息。' },
  { title: '创建 API 密钥', description: '为应用创建密钥，并妥善保管访问凭证。' },
  { title: '接入你的应用', description: '配置 API 地址与密钥，开始你的第一条请求。' },
] : [
  { title: 'Open your dashboard', description: 'Sign in to view your available services and account.' },
  { title: 'Create an API key', description: 'Create a key for your application and store it securely.' },
  { title: 'Connect your application', description: 'Configure your API URL and key, then send your first request.' },
])

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => {
  const subtitle = appStore.cachedPublicSettings?.site_subtitle?.trim()
  return subtitle && !['Subscription to API Conversion Platform', 'AI API Gateway Platform'].includes(subtitle) ? subtitle : copy.value.description
})
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const hasHomeContent = computed(() => homeContent.value.trim().length > 0)
const compactHomeEnabled = computed(() => appStore.cachedPublicSettings?.compact_home_enabled === true)
const registrationEnabled = computed(() => appStore.cachedPublicSettings?.registration_enabled === true)
const isHomeContentUrl = computed(() => /^https?:\/\//.test(homeContent.value.trim()))
const isDark = ref(document.documentElement.classList.contains('dark'))
const isAuthenticated = computed(() => authStore.isAuthenticated)
const showModelPlazaEntry = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlaza) && (isAuthenticated.value || appStore.cachedPublicSettings?.model_plaza_require_auth !== true))
const dashboardPath = computed(() => authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
const primaryDestination = computed(() => isAuthenticated.value ? dashboardPath.value : registrationEnabled.value ? '/register' : '/login')
const apiBaseUrl = `${window.location.origin}/v1`
const currentYear = new Date().getFullYear()

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}
onMounted(() => {
  const savedTheme = localStorage.getItem('theme')
  isDark.value = savedTheme === 'dark' || (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', isDark.value)
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) appStore.fetchPublicSettings()
})
</script>

<style scoped>
.public-home {
  --home-bg: #f6f8f7;
  --home-surface: #ffffff;
  --home-ink: #102b2a;
  --home-muted: #60716d;
  --home-border: #dce5e1;
  --home-accent: #087f72;
  --home-accent-hover: #06695e;
  --home-accent-soft: #e7f1ec;
  --home-panel: #102f2d;
  --home-panel-ink: #effaf4;
  --home-panel-muted: #a2c0b6;
  --home-panel-border: #36544c;
  --home-panel-accent: #addec0;
  --home-glass: #ffffffb3;
  --home-glass-edge: #ffffffeb;
  --home-wash: #9ad9c671;
  --home-wash-secondary: #d2e8c780;
  --home-shadow: #244b3c0a;
  position: relative;
  isolation: isolate;
  background: var(--home-bg);
  color: var(--home-ink);
  min-height: 100dvh;
  display: flex;
  flex-direction: column;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif;
}
.dark .public-home {
  --home-bg: #0c1d1c;
  --home-surface: #132925;
  --home-ink: #e7f2ed;
  --home-muted: #a6bbb3;
  --home-border: #2d4540;
  --home-accent: #81d7ba;
  --home-accent-hover: #ade7cf;
  --home-accent-soft: #193b32;
  --home-glass: #132925b8;
  --home-glass-edge: #a6d8bf26;
  --home-wash: #20785c3b;
  --home-wash-secondary: #37634236;
  --home-shadow: #00000020;
}
.public-home::before {
  content: '';
  position: absolute;
  pointer-events: none;
  z-index: -1;
  inset: 0 0 auto;
  height: min(100%, 1050px);
  background: radial-gradient(ellipse at 82% 32%, var(--home-wash), transparent 48%), radial-gradient(ellipse at 8% 62%, var(--home-wash-secondary), transparent 44%);
}
.home-container { width: min(100% - 96px, 1200px); margin-inline: auto; }
.home-header { border-bottom: 1px solid var(--home-glass-edge); background: var(--home-glass); backdrop-filter: blur(18px); -webkit-backdrop-filter: blur(18px); }
.home-nav { min-height: 88px; display: flex; align-items: center; justify-content: space-between; gap: 32px; }
.home-brand { display: flex; align-items: center; min-height: 44px; min-width: 0; gap: 12px; font-size: 21px; font-weight: 700; letter-spacing: -0.7px; }
.home-brand img { width: 36px; height: 36px; flex-shrink: 0; border-radius: 10px; object-fit: contain; }
.home-brand span { overflow-wrap: anywhere; }
.nav-links, .nav-tools { display: flex; align-items: center; gap: 16px; }
.nav-links { gap: 28px; }
.nav-text-link { display: flex; align-items: center; gap: 7px; min-height: 44px; color: var(--home-muted); font-size: 14px; }
.nav-tools { gap: 4px; }
.nav-tools :deep(button) { min-height: 44px; }
.theme-button { width: 44px; height: 44px; display: grid; place-items: center; border-radius: 10px; color: var(--home-muted); }
.header-login { display: flex; align-items: center; gap: 16px; min-height: 44px; padding: 10px 17px; border: 1px solid var(--home-border); background: var(--home-surface); border-radius: 8px; font-size: 14px; font-weight: 600; white-space: nowrap; }
.public-home a, .public-home button { transition: color 160ms ease, background-color 160ms ease, border-color 160ms ease; }
.public-home a:focus-visible, .public-home button:focus-visible { outline: 3px solid var(--home-accent); outline-offset: 4px; }
.nav-text-link:hover, .secondary-action:hover { color: var(--home-accent); }
.theme-button:hover, .header-login:hover { background: var(--home-accent-soft); }
.home-hero { display: grid; grid-template-columns: 1.08fr 1fr; gap: 64px; align-items: center; padding: 96px 0 88px; animation: home-arrive 650ms cubic-bezier(0.2, 0.7, 0.2, 1) both; }
.hero-copy { min-width: 0; }
.eyebrow { display: flex; align-items: center; gap: 10px; font: 500 11px/1.6 ui-monospace, SFMono-Regular, Menlo, monospace; letter-spacing: 2px; color: var(--home-accent); }
.eyebrow > span { height: 7px; width: 7px; border-radius: 50%; background: var(--home-accent); }
.hero-copy h1 { font-size: clamp(42px, 4.1vw, 60px); font-weight: 650; line-height: 1.28; letter-spacing: -2px; margin: 24px 0; }
.hero-copy h1 > span { color: var(--home-accent); }
.hero-description { max-width: 470px; font-size: 17px; line-height: 1.9; color: var(--home-muted); white-space: pre-wrap; overflow-wrap: anywhere; }
.hero-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 24px; margin-top: 36px; }
.primary-action { display: inline-flex; justify-content: center; align-items: center; gap: 32px; min-height: 52px; padding: 14px 24px; background: var(--home-accent); color: var(--home-surface); border-radius: 8px; font-weight: 600; font-size: 15px; }
.dark .primary-action { color: var(--home-bg); }
.primary-action:hover { background: var(--home-accent-hover); }
.secondary-action { display: flex; align-items: center; gap: 8px; min-height: 44px; font-size: 14px; font-weight: 500; }
.hero-details { display: flex; flex-wrap: wrap; gap: 18px; margin-top: 26px; color: var(--home-muted); font-size: 12px; }
.hero-details > span { display: inline-flex; align-items: center; gap: 6px; }
.hero-details svg { color: var(--home-accent); }
.connection-panel { position: relative; min-width: 0; padding: 24px; border-radius: 22px; background: linear-gradient(145deg, #274e42df, #102f2ded 58%); color: var(--home-panel-ink); box-shadow: 0 28px 64px -30px #15362c70, inset 0 1px 0 #d5ffe12b; border: 1px solid #bce9d14d; backdrop-filter: blur(24px); -webkit-backdrop-filter: blur(24px); }
.connection-panel::before { content: ''; pointer-events: none; position: absolute; inset: 9px; border: 1px solid #dcffec0a; border-radius: 15px; }
.panel-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.panel-label { font: 500 11px/1.6 ui-monospace, SFMono-Regular, Menlo, monospace; letter-spacing: 1.4px; overflow-wrap: anywhere; }
.panel-slash { margin-inline: 7px; color: var(--home-panel-muted); }
.example-label { flex-shrink: 0; border: 1px solid var(--home-panel-border); padding: 4px 8px; border-radius: 5px; font-size: 10px; color: var(--home-panel-muted); }
.connection-map { height: 184px; position: relative; display: grid; grid-template-columns: 1fr 1fr 1fr; align-items: center; }
.connection-lines { position: absolute; inset: 0; width: 100%; height: 100%; stroke: #729c8a80; stroke-width: 1; }
.connection-flow { stroke: #c6f8dc; stroke-width: 2; stroke-dasharray: 16 100; stroke-dashoffset: 0; opacity: 0; animation: connection-pass 4.2s linear 1; }
.connection-arrow { stroke: var(--home-panel-accent); }
.project-node { position: relative; display: flex; align-items: center; flex-direction: column; gap: 7px; padding-top: 38px; }
.project-node-own { justify-self: start; }
.project-node-yours { justify-self: end; }
.project-node > span { font-size: 12px; color: var(--home-panel-ink); }
.project-node > small { font-size: 10px; color: var(--home-panel-muted); line-height: 1; }
.app-node { display: grid; place-items: center; width: 60px; height: 60px; border: 1px solid #a0ceb04a; background: #b6eaca0c; border-radius: 13px; color: var(--home-panel-accent); box-shadow: inset 0 1px 0 #d5ffe114; }
.gateway-node { position: relative; width: 76px; height: 76px; display: grid; place-items: center; justify-self: center; border: 1px solid #80b49a; background: #24463d; border-radius: 18px; box-shadow: 0 0 0 9px #b5edc508, 0 0 0 20px #b5edc504; }
.gateway-node img { width: 52px; height: 52px; object-fit: contain; border-radius: 11px; }
.code-card { min-width: 0; background: #071f1bd1; border: 1px solid var(--home-panel-border); border-radius: 10px; overflow: hidden; }
.code-toolbar { display: flex; align-items: center; justify-content: space-between; padding: 12px 14px; border-bottom: 1px solid #29433c; color: var(--home-panel-muted); font: 10px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace; }
.code-toolbar > span:first-child { display: flex; gap: 4px; }
.code-toolbar i { width: 5px; height: 5px; border-radius: 50%; background: #58786e; }
.code-body { padding: 18px 14px; font: 11px/2 ui-monospace, SFMono-Regular, Menlo, monospace; }
.code-body > div { display: flex; align-items: baseline; }
.code-body code { min-width: 0; white-space: pre-wrap; overflow-wrap: anywhere; }
.code-number { color: #77978c; flex-shrink: 0; width: 28px; font-size: 10px; user-select: none; }
.code-keyword { color: #a5d5e1; }
.code-string { color: var(--home-panel-accent); }
.panel-footer { display: flex; align-items: center; gap: 8px; margin-top: 20px; font-size: 11px; color: var(--home-panel-muted); }
.connection-mark { color: var(--home-panel-accent); }
.panel-index { margin-left: auto; font: 9px/1.6 ui-monospace, SFMono-Regular, Menlo, monospace; letter-spacing: 1px; }
.capabilities { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; margin-bottom: 56px; }
.capability { display: flex; align-items: flex-start; gap: 16px; min-width: 0; padding: 26px; border: 1px solid var(--home-glass-edge); border-radius: 16px; background: var(--home-glass); box-shadow: 0 12px 32px -16px var(--home-shadow), inset 0 1px 0 var(--home-glass-edge); backdrop-filter: blur(18px); -webkit-backdrop-filter: blur(18px); }
.feature-icon { width: 38px; height: 38px; display: grid; place-items: center; flex-shrink: 0; border-radius: 9px; color: var(--home-accent); background: var(--home-accent-soft); }
.capability h2 { margin-bottom: 8px; font-size: 15px; font-weight: 600; line-height: 1.5; }
.capability p { font-size: 13px; line-height: 1.8; color: var(--home-muted); max-width: 250px; }
.getting-started { display: grid; grid-template-columns: 1fr 1fr; gap: 80px; margin: 88px 0; }
.section-intro h2 { font-size: 36px; line-height: 1.4; font-weight: 600; letter-spacing: -1px; margin: 20px 0 16px; }
.section-intro > p:last-child { color: var(--home-muted); font-size: 15px; line-height: 1.9; }
.setup-steps { display: flex; flex-direction: column; gap: 26px; }
.setup-steps > li { display: flex; gap: 22px; }
.step-index { font: 12px/2 ui-monospace, SFMono-Regular, Menlo, monospace; color: var(--home-accent); padding-top: 2px; }
.setup-steps h3 { font-size: 17px; font-weight: 600; margin-bottom: 8px; }
.setup-steps p { font-size: 14px; line-height: 1.8; color: var(--home-muted); }
.home-footer { margin-top: auto; padding: 24px 0; }
.footer-content { display: flex; align-items: center; gap: 24px; color: var(--home-muted); font-size: 12px; }
.footer-content > p { overflow-wrap: anywhere; }
.footer-note { margin-left: auto; }
.footer-content a { display: flex; align-items: center; gap: 6px; min-height: 44px; }
.skip-link { position: absolute; left: 24px; top: -70px; z-index: 100; background: var(--home-surface); color: var(--home-ink); border: 1px solid var(--home-accent); padding: 12px 18px; border-radius: 8px; }
.skip-link:focus { top: 12px; }
@media (min-width: 1600px) { .home-hero { padding-top: 116px; padding-bottom: 112px; } }
@media (max-width: 1100px) {
  .home-container { width: calc(100% - 64px); }
  .home-hero { gap: 36px; padding-block: 72px; }
  .hero-copy h1 { font-size: 44px; }
  .hero-description { font-size: 16px; }
  .connection-panel { padding: 20px; }
  .capability { padding: 22px 18px; gap: 12px; }
  .nav-links { gap: 16px; }
}
@media (max-width: 820px) {
  .home-nav { gap: 16px; min-height: 80px; flex-wrap: wrap; padding-block: 16px; }
  .nav-links { gap: 12px; }
  .home-hero { grid-template-columns: 1fr; gap: 48px; padding: 64px 0 52px; }
  .hero-copy { max-width: 600px; }
  .hero-copy h1 { font-size: 50px; }
  .hero-description { max-width: 540px; }
  .connection-panel { max-width: 560px; width: 100%; justify-self: center; }
  .capabilities { gap: 16px; }
  .capability { flex-direction: column; padding: 22px; }
  .getting-started { gap: 40px; }
  .section-intro h2 { font-size: 30px; }
}
@media (max-width: 560px) {
  .home-container { width: calc(100% - 40px); }
  .home-nav { gap: 12px; }
  .home-brand { font-size: 18px; gap: 9px; max-width: calc(100% - 96px); }
  .home-brand img { width: 32px; height: 32px; }
  .nav-links { flex: 1 1 100%; justify-content: space-between; flex-wrap: wrap; gap: 8px; }
  .nav-tools { order: 3; margin-left: auto; }
  .header-login { order: 4; }
  .nav-text-link { font-size: 13px; }
  .home-hero { padding-top: 44px; gap: 38px; }
  .hero-copy h1 { font-size: clamp(35px, 10.4vw, 48px); letter-spacing: -1.4px; margin-block: 20px; }
  .hero-description { font-size: 16px; line-height: 1.85; }
  .eyebrow { font-size: 10px; letter-spacing: 1.5px; }
  .hero-actions { margin-top: 28px; gap: 14px; }
  .primary-action { padding-inline: 20px; gap: 24px; }
  .secondary-action { font-size: 13px; }
  .hero-details { gap: 10px 16px; font-size: 11px; margin-top: 22px; }
  .connection-panel { padding: 18px 16px; border-radius: 14px; }
  .panel-label { font-size: 10px; letter-spacing: 1px; }
  .panel-slash { margin-inline: 3px; }
  .connection-map { height: 164px; }
  .app-node { width: 56px; height: 56px; }
  .project-node > span { font-size: 11px; }
  .project-node { padding-top: 36px; }
  .gateway-node { width: 66px; height: 66px; }
  .gateway-node img { width: 46px; height: 46px; }
  .code-body { padding: 14px 10px; font-size: 10px; }
  .code-number { width: 23px; }
  .panel-footer { font-size: 10px; }
  .capabilities { grid-template-columns: 1fr; gap: 14px; margin-bottom: 16px; }
  .capability { flex-direction: row; gap: 16px; padding: 22px; }
  .capability p { max-width: none; font-size: 14px; }
  .capability h2 { font-size: 16px; margin-bottom: 4px; }
  .getting-started { grid-template-columns: 1fr; gap: 32px; margin-block: 56px; }
  .footer-content { flex-wrap: wrap; gap: 10px 20px; }
  .footer-note { margin-left: 0; }
  .home-footer { padding-block: 24px; }
}
@keyframes home-arrive {
  from { opacity: 0; transform: translateY(12px); }
  to { opacity: 1; transform: translateY(0); }
}
@keyframes connection-pass {
  0% { opacity: 0; stroke-dashoffset: 120; }
  18% { opacity: 0.8; }
  82% { opacity: 0.8; }
  100% { opacity: 0; stroke-dashoffset: -120; }
}
@supports not ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px))) {
  .home-header, .capability { background: var(--home-surface); border-color: var(--home-border); }
  .connection-panel { background: var(--home-panel); }
}
@media (prefers-reduced-motion: reduce) {
  .public-home a, .public-home button { transition: none; }
  .home-hero, .connection-flow { animation: none; }
}
</style>
