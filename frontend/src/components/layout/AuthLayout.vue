<template>
  <div class="auth-shell">
    <aside class="auth-story" :aria-label="copy.brandLabel">
      <router-link to="/home" class="auth-brand" :aria-label="`${siteName} · ${copy.home}`">
        <img :src="siteLogo || '/logo.svg'" alt="" width="40" height="40" />
        <span>{{ siteName }}</span>
      </router-link>

      <div class="auth-story-content">
        <p class="auth-eyebrow"><span></span> THE NEXT THING YOU BUILD</p>
        <p class="auth-statement">{{ copy.headline }}<br /><span>{{ copy.highlight }}</span></p>
        <p class="auth-story-description">{{ copy.description }}</p>

        <div class="auth-connection" aria-hidden="true">
          <div class="auth-orbit auth-orbit-outer"></div>
          <div class="auth-orbit auth-orbit-inner"></div>
          <div class="auth-connection-line"></div>
          <div class="auth-connection-center">
            <img :src="siteLogo || '/logo.svg'" alt="" width="52" height="52" />
          </div>
          <span class="auth-connection-node auth-connection-node-one">&lt;/&gt;</span>
          <span class="auth-connection-node auth-connection-node-two">{ }</span>
          <span class="auth-connection-node auth-connection-node-three"><Icon name="sparkles" size="md" /></span>
          <span class="auth-connection-dot"></span>
          <span class="auth-connection-caption">IDEA &nbsp; → &nbsp; API &nbsp; → &nbsp; BUILD</span>
        </div>

        <div class="auth-story-note">
          <Icon name="terminal" size="md" aria-hidden="true" />
          <span>{{ copy.note }}</span>
        </div>
      </div>
      <p class="auth-story-footer">{{ siteName }} <span>/</span> AI, CONNECTED.</p>
    </aside>

    <div class="auth-main">
      <header class="auth-toolbar">
        <router-link to="/home" class="auth-home-link">
          <Icon name="arrowLeft" size="sm" aria-hidden="true" />
          <span>{{ copy.home }}</span>
        </router-link>
        <div class="auth-toolbar-actions">
          <LocaleSwitcher />
          <button
            type="button"
            class="auth-theme-toggle"
            :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon :name="isDark ? 'sun' : 'moon'" size="md" aria-hidden="true" />
          </button>
        </div>
      </header>

      <main id="auth-content" class="auth-form-area">
        <div class="auth-form-wrap">
          <p class="auth-form-eyebrow">{{ siteName }} <span>/</span> WORKSPACE</p>
          <slot />
          <div v-if="$slots.footer" class="auth-form-footer">
            <slot name="footer" />
          </div>
        </div>
      </main>
      <footer class="auth-copyright">&copy; {{ currentYear }} {{ siteName }}</footer>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import Icon from '@/components/icons/Icon.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import { sanitizeUrl } from '@/utils/url'

const appStore = useAppStore()
const { t, locale } = useI18n()
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const currentYear = new Date().getFullYear()
const isDark = ref(document.documentElement.classList.contains('dark'))
const copy = computed(() => locale.value.startsWith('zh') ? {
  brandLabel: '关于平台',
  home: '返回首页',
  headline: '让每一个想法，',
  highlight: '连接更多可能。',
  description: '从灵感到应用，让 AI 成为你的创造力。一个工作空间，管理你的 API 与使用情况。',
  note: '专注创造，连接由此开始。'
} : {
  brandLabel: 'About the platform',
  home: 'Back to home',
  headline: 'Your next idea.',
  highlight: 'Connected to AI.',
  description: 'Bring ideas to life with AI. One workspace to manage your API access and usage.',
  note: 'Stay focused on what you are building.'
})

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.auth-shell {
  --auth-canvas: #f6f8f7;
  --auth-surface: #ffffff;
  --auth-ink: #102b2a;
  --auth-muted: #60716d;
  --auth-border: #dce5e1;
  --auth-accent: #087f72;
  --auth-accent-hover: #06695e;
  --auth-ring: #087f72;
  --auth-error: #b42318;
  --auth-error-surface: #fff3f1;
  --auth-story: #102b2a;
  --auth-story-text: #f2f8f5;
  --auth-story-muted: #b5c9c0;
  --auth-story-accent: #9ee0bc;
  display: grid;
  grid-template-columns: minmax(0, 0.95fr) minmax(0, 1.05fr);
  min-height: 100vh;
  min-height: 100dvh;
  color: var(--auth-ink);
  background: var(--auth-canvas);
}
.dark .auth-shell {
  --auth-canvas: #101c1b;
  --auth-surface: #182725;
  --auth-ink: #eef5f1;
  --auth-muted: #a7bcb4;
  --auth-border: #3a5149;
  --auth-accent: #67cbb1;
  --auth-accent-hover: #8ce0c8;
  --auth-ring: #67cbb1;
  --auth-error: #ffafa6;
  --auth-error-surface: #3c2323;
  --auth-story: #0b2422;
}
.auth-story {
  position: relative;
  display: flex;
  flex-direction: column;
  min-width: 0;
  padding: 44px clamp(32px, 4.8vw, 88px) 32px;
  overflow: hidden;
  color: var(--auth-story-text);
  background: var(--auth-story);
}
.auth-brand {
  position: relative;
  z-index: 1;
  display: inline-flex;
  align-items: center;
  align-self: flex-start;
  gap: 12px;
  min-height: 44px;
  font-size: 21px;
  font-weight: 650;
  letter-spacing: -0.7px;
  overflow-wrap: anywhere;
}
.auth-brand img { flex-shrink: 0; border-radius: 11px; object-fit: contain; }
.auth-story-content { width: 100%; max-width: 510px; margin: auto; padding: 76px 0 40px; }
.auth-eyebrow,
.auth-form-eyebrow,
.auth-story-footer,
.auth-connection-caption {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 11px;
  font-weight: 500;
  letter-spacing: 1.4px;
  line-height: 1.6;
}
.auth-eyebrow { display: flex; align-items: center; gap: 10px; color: var(--auth-story-muted); }
.auth-eyebrow span { width: 6px; height: 6px; border-radius: 50%; background: var(--auth-story-accent); }
.auth-statement { margin-top: 28px; font-size: clamp(34px, 3.35vw, 58px); font-weight: 600; line-height: 1.42; letter-spacing: -1.8px; }
.auth-statement span { color: var(--auth-story-accent); }
.auth-story-description { max-width: 370px; margin-top: 24px; color: var(--auth-story-muted); font-size: 15px; line-height: 1.95; }
.auth-connection { position: relative; height: 260px; max-width: 400px; margin-top: 26px; }
.auth-orbit { position: absolute; top: 50%; left: 50%; border: 1px solid rgba(181, 201, 192, 0.2); border-radius: 50%; transform: translate(-50%, -50%) rotate(-28deg); }
.auth-orbit-outer { width: 335px; height: 176px; }
.auth-orbit-inner { width: 225px; height: 225px; border-style: dashed; opacity: 0.7; }
.auth-connection-line { position: absolute; top: 50%; left: 0; width: 100%; height: 1px; background: linear-gradient(90deg, transparent, rgba(181, 201, 192, 0.25), transparent); }
.auth-connection-center { position: absolute; top: 50%; left: 50%; display: grid; place-items: center; width: 92px; height: 92px; border: 1px solid rgba(181, 201, 192, 0.3); border-radius: 25px; background: #193a35; box-shadow: 0 0 0 12px rgba(158, 224, 188, 0.025), 0 20px 50px rgba(0, 0, 0, 0.16); transform: translate(-50%, -50%); }
.auth-connection-center img { border-radius: 13px; object-fit: contain; }
.auth-connection-node { position: absolute; display: grid; place-items: center; width: 43px; height: 43px; border: 1px solid rgba(181, 201, 192, 0.28); border-radius: 13px; color: var(--auth-story-accent); background: #193a35; font-family: ui-monospace, monospace; font-size: 15px; }
.auth-connection-node-one { top: 144px; left: 27px; }
.auth-connection-node-two { top: 40px; right: 26px; }
.auth-connection-node-three { top: 18px; left: 118px; font-size: 24px; }
.auth-connection-dot { position: absolute; right: 108px; bottom: 22px; width: 6px; height: 6px; border-radius: 50%; background: var(--auth-story-accent); }
.auth-connection-caption { position: absolute; bottom: -12px; left: 0; width: 100%; color: var(--auth-story-muted); text-align: center; font-size: 10px; }
.auth-story-note { display: flex; align-items: center; gap: 10px; margin-top: 44px; color: var(--auth-story-muted); font-size: 13px; }
.auth-story-footer { margin-top: 32px; color: var(--auth-story-muted); font-size: 10px; }
.auth-story-footer span { margin: 0 10px; opacity: 0.6; }
.auth-main { display: flex; flex-direction: column; min-width: 0; }
.auth-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 38px clamp(24px, 3.5vw, 64px) 12px; }
.auth-toolbar-actions { display: flex; align-items: center; gap: 8px; }
.auth-home-link { display: inline-flex; align-items: center; gap: 8px; min-height: 44px; color: var(--auth-muted); font-size: 13px; }
.auth-home-link:hover { color: var(--auth-accent); }
.auth-theme-toggle { display: grid; place-items: center; width: 44px; height: 44px; border-radius: 10px; color: var(--auth-muted); cursor: pointer; transition: background 160ms, color 160ms; }
.auth-theme-toggle:hover { color: var(--auth-ink); background: var(--auth-border); }
.auth-form-area { display: flex; flex: 1; align-items: center; justify-content: center; width: 100%; padding: 60px 36px; }
.auth-form-wrap { width: 100%; min-width: 0; max-width: 420px; }
.auth-form-eyebrow { margin-bottom: 24px; color: var(--auth-muted); font-size: 10px; overflow-wrap: anywhere; }
.auth-form-eyebrow span { padding: 0 6px; opacity: 0.6; }
.auth-form-footer { margin-top: 28px; padding-top: 24px; border-top: 1px solid var(--auth-border); font-size: 14px; text-align: center; line-height: 1.8; }
.auth-copyright { padding: 16px 24px 28px; color: var(--auth-muted); font-size: 11px; text-align: center; }
.auth-shell :deep(.auth-view-header) { margin-bottom: 30px; text-align: left; }
.auth-shell :deep(.auth-view-title) { color: var(--auth-ink); font-size: 32px; font-weight: 650; line-height: 1.35; letter-spacing: -1px; }
.auth-shell :deep(.auth-view-subtitle) { margin-top: 12px; color: var(--auth-muted); font-size: 14px; line-height: 1.85; }
.auth-shell :deep(.input-label) { margin-bottom: 9px; color: var(--auth-ink); font-size: 13px; font-weight: 600; }
.auth-shell :deep(.input) { min-height: 50px; border: 1px solid var(--auth-border); border-radius: 10px; color: var(--auth-ink); background: var(--auth-surface); font-size: 16px; box-shadow: none; transition: border-color 160ms, box-shadow 160ms; }
.auth-shell :deep(.input::placeholder) { color: var(--auth-muted); opacity: 0.8; }
.auth-shell :deep(.input:focus) { border-color: var(--auth-ring); box-shadow: 0 0 0 3px color-mix(in srgb, var(--auth-ring) 15%, transparent); outline: none; }
.auth-shell :deep(.input.input-error) { border-color: var(--auth-error); }
.auth-shell :deep(.input:disabled) { opacity: 0.65; cursor: not-allowed; }
.auth-shell :deep(.input-hint) { color: var(--auth-muted); line-height: 1.65; }
.auth-shell :deep(.btn) { min-height: 50px; border-radius: 10px; font-size: 14px; box-shadow: none; transition: background 160ms, border-color 160ms; }
.auth-shell :deep(.btn:active) { transform: none; }
.auth-shell :deep(.btn-primary) { color: #ffffff; background: var(--auth-accent); font-weight: 600; }
.auth-shell :deep(.btn-primary:hover:not(:disabled)) { background: var(--auth-accent-hover); }
.dark .auth-shell :deep(.btn-primary) { color: #102b2a; }
.auth-shell :deep(.btn-secondary) { color: var(--auth-ink); background: var(--auth-surface); border-color: var(--auth-border); }
.auth-shell :deep(.auth-password-toggle) { justify-content: center; width: 44px; padding-right: 0; border-radius: 8px; }
.auth-shell :deep(.auth-field-error) { margin-top: 7px; color: var(--auth-error); font-size: 12px; line-height: 1.6; }
.auth-shell :deep(.auth-api-error) { padding: 12px 14px; border: 1px solid var(--auth-error); border-radius: 10px; color: var(--auth-error); background: var(--auth-error-surface); font-size: 13px; line-height: 1.7; overflow-wrap: anywhere; }
.auth-shell :deep(.auth-form-footer p) { color: var(--auth-muted); }
.auth-shell :deep(a.text-primary-600) { color: var(--auth-accent); }
.auth-shell :deep(a.text-primary-600:hover) { text-decoration: underline; text-underline-offset: 3px; }
.auth-shell :deep(a:focus-visible),
.auth-shell :deep(button:focus-visible) { outline: 2px solid var(--auth-ring); outline-offset: 4px; }
.auth-brand:focus-visible { outline-color: var(--auth-story-accent); }
.auth-toolbar-actions :deep(button) { min-height: 44px; min-width: 44px; }
@media (min-width: 1600px) { .auth-form-area { padding-top: 80px; padding-bottom: 100px; } }
@media (max-width: 1023px) {
  .auth-shell { grid-template-columns: minmax(0, 0.85fr) minmax(0, 1.15fr); }
  .auth-story { padding: 32px; }
  .auth-story-content { padding-top: 56px; }
  .auth-statement { font-size: 34px; }
  .auth-connection { transform: scale(0.85); transform-origin: center; margin-left: -16px; margin-right: -16px; }
  .auth-form-area { padding: 48px 32px; }
  .auth-toolbar { padding: 28px 28px 8px; }
}
@media (max-width: 767px) {
  .auth-shell { display: flex; flex-direction: column; }
  .auth-story { padding: 20px 24px; }
  .auth-brand { min-height: 40px; font-size: 19px; }
  .auth-brand img { width: 34px; height: 34px; border-radius: 9px; }
  .auth-story-content, .auth-story-footer { display: none; }
  .auth-main { flex: 1; }
  .auth-toolbar { padding: 12px 20px 0; }
  .auth-form-area { align-items: flex-start; padding: 36px 24px 40px; }
  .auth-form-eyebrow { margin-bottom: 20px; }
  .auth-shell :deep(.auth-view-title) { font-size: 30px; }
  .auth-copyright { margin-top: auto; padding-bottom: 24px; }
}
@media (prefers-reduced-motion: reduce) {
  .auth-shell :deep(*) { transition-duration: 0.01ms !important; animation-duration: 0.01ms !important; animation-iteration-count: 1 !important; }
}
</style>
