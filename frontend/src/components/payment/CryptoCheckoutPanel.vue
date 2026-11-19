<template>
  <section class="crypto-checkout" :aria-busy="checking && !checkout">
    <header class="crypto-header">
      <div class="crypto-brand"><img :src="isUSDT ? usdtIcon : paymentIcon" alt="" /></div>
      <div class="min-w-0 flex-1">
        <h1 :id="modal ? 'crypto-dialog-title' : undefined">{{ title }}</h1>
        <p>{{ t('payment.cryptoCheckout.subtitle') }}</p>
      </div>
      <span v-if="checkout" class="network-badge">{{ networkBadge }}</span>
      <button v-if="modal" type="button" class="crypto-icon-button" :aria-label="t('common.close')" @click="emit('close')"><Icon name="x" aria-hidden="true" /></button>
    </header>

    <div class="crypto-body">
      <p v-if="error" role="alert" class="crypto-error">{{ error }}</p>
      <div v-if="!checkout && !error" class="crypto-loading" role="status"><span class="crypto-spinner" />{{ t('common.loading') }}</div>
      <template v-if="checkout">
        <div class="crypto-total">
          <div class="min-w-0">
            <p class="crypto-label">{{ t('payment.cryptoCheckout.exactAmount') }}</p>
            <div class="crypto-amount">
              <strong>{{ checkout.crypto_amount }}</strong><span>{{ coinName }}</span>
              <button v-if="canTransfer" type="button" class="crypto-icon-button" :aria-label="t('payment.cryptoCheckout.copyAmount')" @click="copy(checkout.crypto_amount, 'amount')"><Icon :name="copied && copyTarget === 'amount' ? 'check' : 'copy'" aria-hidden="true" /></button>
            </div>
          </div>
          <div class="crypto-countdown" :class="{ 'crypto-countdown-urgent': seconds > 0 && seconds < 180 }">
            <p class="crypto-label"><Icon name="clock" size="sm" aria-hidden="true" />{{ t('payment.cryptoCheckout.timeLabel') }}</p>
            <strong class="tabular-nums">{{ remaining }}</strong>
          </div>
        </div>
        <div class="crypto-fiat">
          <p>{{ t('payment.cryptoCheckout.fiatLabel') }} <strong>{{ checkout.fiat_amount }} {{ checkout.currency }}</strong></p>
          <p v-if="checkout.exchange_rate">1 USDT = {{ checkout.exchange_rate }} CNY <span aria-hidden="true">·</span> {{ t('payment.cryptoCheckout.precisionHint') }}</p>
        </div>
        <p v-if="networkDelayed" role="alert" class="crypto-notice" data-testid="payment-network-delay">{{ t('payment.cryptoCheckout.networkDelayHint') }}</p>
        <div v-if="canTransfer" class="crypto-transfer-grid">
          <div class="crypto-qr-column">
            <div class="crypto-qr"><img v-if="qr" data-testid="payment-qr" :src="qr" :alt="t('payment.cryptoCheckout.addressQR')" width="208" height="208" /><span v-else class="crypto-spinner" /></div>
            <p>{{ t('payment.cryptoCheckout.scanHint') }}</p>
            <span>{{ networkName }}</span>
          </div>
          <div class="crypto-details">
            <label class="crypto-label">{{ t('payment.cryptoCheckout.address') }}</label>
            <div class="crypto-address" data-testid="receiving-address">{{ checkout.address }}</div>
            <button type="button" class="crypto-copy-address" @click="copy(checkout.address, 'address')"><Icon :name="copied && copyTarget === 'address' ? 'check' : 'copy'" aria-hidden="true" />{{ copied && copyTarget === 'address' ? t('payment.cryptoCheckout.copied') : t('payment.cryptoCheckout.copyAddress') }}</button>
            <div class="crypto-network-row"><span>{{ t('payment.cryptoCheckout.networkLabel') }}</span><strong>{{ networkName }}</strong></div>
            <div v-if="checkout.contract_address" class="crypto-contract">
              <p class="crypto-label">{{ t('payment.cryptoCheckout.contractLabel') }}</p>
              <a :href="'https://tronscan.org/#/token20/' + encodeURIComponent(checkout.contract_address)" target="_blank" rel="noopener noreferrer">{{ checkout.contract_address }}<Icon name="externalLink" size="xs" aria-hidden="true" /></a>
              <p>{{ t('payment.cryptoCheckout.confirmationsHint') }}</p>
            </div>
            <div class="crypto-order"><span class="crypto-label">{{ t('payment.cryptoCheckout.orderLabel') }}</span><div><code>{{ checkout.out_trade_no }}</code><button type="button" class="crypto-icon-button" :aria-label="t('payment.cryptoCheckout.copyOrder')" @click="copy(checkout.out_trade_no, 'order')"><Icon :name="copied && copyTarget === 'order' ? 'check' : 'copy'" aria-hidden="true" /></button></div></div>
          </div>
        </div>
        <div v-else class="crypto-terminal-state"><Icon :name="completed ? 'checkCircle' : 'clock'" size="xl" aria-hidden="true" /><p>{{ statusText }}</p></div>
        <div v-if="canTransfer" class="crypto-notice"><Icon name="exclamationCircle" aria-hidden="true" /><div><strong>{{ t('payment.cryptoCheckout.noticeTitle') }}</strong><p>{{ checkout.exchange_rate ? t('payment.cryptoCheckout.usdtNotice') : t('payment.cryptoCheckout.instruction', { network: networkName }) }}</p></div></div>
        <div class="crypto-status" role="status"><span class="crypto-status-dot" :class="{ 'is-pending': payable && !checkout.confirming }" /><span>{{ statusText }}<template v-if="canTransfer"> · {{ t('payment.cryptoCheckout.autoUpdate') }}</template></span></div>
        <p v-if="copied" class="sr-only" role="status">{{ t('payment.cryptoCheckout.copied') }}</p>
        <div v-if="confirmCancel" class="crypto-cancel-confirm"><p>{{ t('payment.cryptoCheckout.cancelConfirm') }}</p><button type="button" class="crypto-button-secondary" :disabled="checking" @click="cancel">{{ t('common.confirm') }}</button><button type="button" class="crypto-button-secondary" @click="confirmCancel = false">{{ t('payment.cryptoCheckout.keepWaiting') }}</button></div>
      </template>
    </div>

    <footer class="crypto-footer">
      <div class="crypto-footer-help"><p>{{ t('payment.cryptoCheckout.checkHint') }}</p><button v-if="canTransfer" type="button" class="crypto-cancel" :disabled="checking" @click="confirmCancel = true">{{ t('payment.cryptoCheckout.cancelOrder') }}</button></div>
      <div class="crypto-footer-actions">
        <button v-if="modal" type="button" class="crypto-button-secondary" @click="emit('close')">{{ t('common.close') }}</button>
        <RouterLink v-else to="/purchase" class="crypto-button-secondary">{{ t('payment.cryptoCheckout.back') }}</RouterLink>
        <RouterLink v-if="completed && checkout" :to="{ path: '/payment/result', query: { order_id: checkout.order_id, out_trade_no: checkout.out_trade_no } }" class="crypto-button-primary">{{ t('payment.cryptoCheckout.viewResult') }}</RouterLink>
        <button v-else type="button" class="crypto-button-primary" :disabled="checking" @click="refresh(true)"><span v-if="checking" class="crypto-spinner" /><Icon v-else name="checkCircle" aria-hidden="true" />{{ t('payment.cryptoCheckout.check') }}</button>
      </div>
    </footer>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import usdtIcon from '@/assets/icons/usdt.svg'
import paymentIcon from '@/assets/icons/payment.svg'
import QRCode from 'qrcode'
import { paymentAPI, type CryptoCheckout } from '@/api/payment'
import { useClipboard } from '@/composables/useClipboard'
import { BEPUSDT_TRADE_TYPES } from '@/components/payment/providerConfig'
const { t } = useI18n()
const props = defineProps<{ outTradeNo: string; modal?: boolean }>()
const emit = defineEmits<{ close: []; settled: [status: string] }>()
const checkout = ref<CryptoCheckout | null>(null)
const error = ref('')
const qr = ref('')
const checking = ref(false)
const { copied, copyToClipboard } = useClipboard()
const confirmCancel = ref(false)
const now = ref(Date.now())
let alive = true
let timer: ReturnType<typeof setInterval> | undefined
let revision = 0
const outTradeNo = computed(() => props.outTradeNo)
const canTransfer = computed(() => payable.value && !checkout.value?.confirming && !networkDelayed.value)
const isUSDT = computed(() => checkout.value?.crypto === 'USDT' || !checkout.value)
const networkBadge = computed(() => checkout.value?.network === 'usdt.trc20' ? 'TRC20' : networkName.value)
const title = computed(() => isUSDT.value ? t('payment.cryptoCheckout.rechargeTitle') : t('payment.cryptoCheckout.title'))
const copyTarget = ref('')
const networkName = computed(() => BEPUSDT_TRADE_TYPES.find(method => method.value === checkout.value?.network)?.label || checkout.value?.network)
const coinName = computed(() => checkout.value?.crypto === 'GRAM' ? 'TON' : checkout.value?.crypto)
const completed = computed(() => checkout.value?.status === 'COMPLETED')
const networkDelayed = computed(() => checkout.value?.network_status === 'delayed' && ['PENDING', 'EXPIRED'].includes(checkout.value?.status || ''))
const seconds = computed(() => Math.max(0, Math.ceil((new Date(checkout.value?.expires_at || 0).getTime() - now.value) / 1000)))
const payable = computed(() => checkout.value?.status === 'PENDING' && (seconds.value > 0 || checkout.value.confirming))
const remaining = computed(() => `${Math.floor(seconds.value / 60)}:${String(seconds.value % 60).padStart(2, '0')}`)
const statusText = computed(() => {
  const status = checkout.value?.status
  if (completed.value) return t('payment.cryptoCheckout.paid')
  if (['PAID', 'RECHARGING'].includes(status || '')) return t('payment.cryptoCheckout.fulfillment')
  if (checkout.value?.confirming) return t('payment.cryptoCheckout.confirming')
  if (status === 'CANCELLED') return t('payment.cryptoCheckout.cancelled')
  if (status === 'FAILED') return t('payment.cryptoCheckout.failed')
  if (status === 'EXPIRED' || seconds.value === 0) return t('payment.cryptoCheckout.expired')
  if (networkDelayed.value) return t('payment.cryptoCheckout.networkDelayed')
  return t('payment.cryptoCheckout.waiting')
})
async function refresh(verify = false) {
  if (checking.value || !outTradeNo.value) return
  checking.value = true
  const currentRevision = ++revision
  const currentOrder = outTradeNo.value
  try {
    if (verify) await paymentAPI.verifyOrder(currentOrder)
    const { data } = await paymentAPI.getCryptoCheckout(currentOrder)
    if (!alive || currentRevision !== revision) return
    const changedAddress = checkout.value?.address !== data.address || !qr.value
    checkout.value = data
    error.value = ''
    if (changedAddress) {
      const nextQR = await QRCode.toDataURL(data.address, { width: 240, margin: 2 })
      if (alive && currentRevision === revision) qr.value = nextQR
    }
  } catch (cause) { if (alive && currentRevision === revision) { checkout.value = null; qr.value = ''; error.value = (cause as { message?: string }).message || t('payment.cryptoCheckout.loadError') } }
  finally { if (alive && currentRevision === revision) checking.value = false }
}
async function copy(value: string, target = '') {
  copyTarget.value = target
  if (!await copyToClipboard(value, t('payment.cryptoCheckout.copied'))) error.value = t('payment.cryptoCheckout.copyError')
}
async function cancel() {
  if (!checkout.value || checking.value) return
  checking.value = true
  try { await paymentAPI.cancelOrder(checkout.value.order_id); confirmCancel.value = false }
  catch (cause) { error.value = (cause as { message?: string }).message || t('payment.cryptoCheckout.loadError') }
  finally { checking.value = false }
  await refresh()
}
watch(() => checkout.value?.status, (status) => { if (status && ['COMPLETED', 'CANCELLED', 'FAILED', 'EXPIRED'].includes(status)) emit('settled', status) })
onMounted(() => {
  void refresh()
  let ticks = 0
  timer = setInterval(() => {
    now.value = Date.now()
    ticks++
    const status = checkout.value?.status || ''
    if (ticks % 5 === 0 && ['PENDING', 'PAID', 'RECHARGING'].includes(status)) void refresh(true)
    // A transfer sent before expiry can be recognized during outage recovery.
    if (ticks % 15 === 0 && status === 'EXPIRED' && now.value < new Date(checkout.value?.expires_at || 0).getTime() + 3 * 60 * 60 * 1000) void refresh(true)
  }, 1000)
})
watch(outTradeNo, () => { revision++; checking.value = false; checkout.value = null; qr.value = ''; error.value = ''; void refresh() })
onUnmounted(() => { alive = false; if (timer) clearInterval(timer) })
</script>

<style scoped>
.crypto-checkout { --pay-ink: #1c3154; --pay-muted: #60728e; --pay-line: #e0e7f1; --pay-surface: #fff; --pay-soft: #f5f8fe; --pay-blue: #2563eb; --pay-green: #16866e; color: var(--pay-ink); background: var(--pay-surface); border: 1px solid var(--pay-line); border-radius: 24px; overflow: hidden; box-shadow: 0 24px 80px #1c31540c; }
:global(.dark .crypto-checkout) { --pay-ink: #e8eef9; --pay-muted: #a6b5cd; --pay-line: #344256; --pay-surface: #172232; --pay-soft: #1e2c40; --pay-blue: #8db4ff; --pay-green: #69d2b4; }
.crypto-header { display:flex; align-items:center; gap:16px; padding:24px 28px; border-bottom:1px solid var(--pay-line); }
.crypto-brand { display:grid; place-items:center; width:54px; height:54px; flex-shrink:0; border:1px solid #c6ebdf; border-radius:16px; background:#ecf9f4; }.crypto-brand img { width:34px; height:34px; }
.crypto-header h1 { font-size:24px; font-weight:750; letter-spacing:-.025em; line-height:1.3; }.crypto-header p { margin-top:6px; font-size:14px; color:var(--pay-muted); }
.network-badge { border:1px solid #bae4d7; border-radius:8px; padding:7px 11px; font-size:12px; font-weight:750; letter-spacing:.06em; color:#146c59; background:#eef9f5; white-space:nowrap; }
.crypto-body { padding:24px 28px 20px; }.crypto-total { display:flex; align-items:center; justify-content:space-between; gap:24px; background:var(--pay-soft); border:1px solid var(--pay-line); border-radius:16px; padding:20px 24px; }
.crypto-label { color:var(--pay-muted); font-size:13px; line-height:1.6; }.crypto-amount { display:flex; flex-wrap:wrap; align-items:baseline; gap:10px; margin-top:7px; }.crypto-amount strong { font-size:38px; letter-spacing:-.03em; line-height:1.2; font-variant-numeric:tabular-nums; overflow-wrap:anywhere; }.crypto-amount>span { font-size:15px; font-weight:650; color:var(--pay-muted); }.crypto-amount .crypto-icon-button { align-self:center; }.crypto-countdown { text-align:right; flex-shrink:0; }.crypto-countdown p { display:flex; justify-content:flex-end; align-items:center; gap:7px; }.crypto-countdown strong { display:block; margin-top:8px; font-size:31px; letter-spacing:.04em; line-height:1.25; }.crypto-countdown-urgent strong { color:#c05b17; }
.crypto-fiat { margin:17px 0 23px; font-size:13px; color:var(--pay-muted); line-height:1.9; }.crypto-fiat strong { color:var(--pay-ink); }.crypto-fiat p+p { margin-top:2px; }.crypto-fiat span { margin:0 5px; }
.crypto-transfer-grid { display:grid; grid-template-columns:208px minmax(0,1fr); gap:26px; }.crypto-qr-column { text-align:center; }.crypto-qr { display:grid; place-items:center; aspect-ratio:1; border:1px solid var(--pay-line); border-radius:16px; padding:14px; background:white; }.crypto-qr img { width:100%; height:auto; }.crypto-qr-column p { margin-top:11px; font-size:13px; }.crypto-qr-column>span { display:block; color:var(--pay-muted); font-size:12px; margin-top:3px; }
.crypto-address { margin-top:8px; padding:14px 15px; border:1px solid var(--pay-line); border-radius:10px; background:var(--pay-soft); font:500 14px/1.7 ui-monospace,SFMono-Regular,Consolas,monospace; overflow-wrap:anywhere; user-select:all; }.crypto-copy-address { display:flex; align-items:center; justify-content:center; gap:9px; width:100%; min-height:44px; border:1px solid #c7d8fc; border-radius:10px; margin-top:10px; color:var(--pay-blue); background:var(--pay-soft); font-size:14px; font-weight:650; }
.crypto-network-row { display:flex; flex-wrap:wrap; justify-content:space-between; gap:8px; padding:17px 0 13px; font-size:13px; border-bottom:1px solid var(--pay-line); }.crypto-network-row>span { color:var(--pay-muted); }.crypto-network-row strong { color:var(--pay-green); }
.crypto-contract { padding-top:12px; font-size:12px; }.crypto-contract a { display:inline-flex; align-items:center; gap:5px; max-width:100%; margin:3px 0; color:var(--pay-blue); overflow-wrap:anywhere; word-break:break-all; }.crypto-contract a svg { flex-shrink:0; }.crypto-contract>p:last-child { color:var(--pay-muted); }
.crypto-order { padding-top:12px; }.crypto-order>div { display:flex; align-items:center; gap:8px; justify-content:space-between; }.crypto-order code { overflow-wrap:anywhere; font-size:12px; }.crypto-order .crypto-icon-button { flex-shrink:0; }
.crypto-icon-button { display:inline-flex; align-items:center; justify-content:center; min-width:44px; min-height:44px; border-radius:10px; color:var(--pay-muted); cursor:pointer; }.crypto-icon-button:hover { background:var(--pay-soft); color:var(--pay-blue); }
.crypto-notice { display:flex; align-items:flex-start; gap:12px; margin-top:22px; padding:14px 16px; border:1px solid #f1dcad; border-radius:12px; background:#fffbf2; color:#905c18; font-size:12px; line-height:1.8; }.crypto-notice>svg { flex-shrink:0; margin-top:2px; }.crypto-notice strong { font-weight:650; }.crypto-notice p { margin-top:3px; }:global(.dark .crypto-notice) { background:#332a1e; color:#f5cc87; border-color:#66502d; }
.crypto-status { display:flex; align-items:center; justify-content:center; gap:8px; text-align:center; color:var(--pay-muted); font-size:13px; margin-top:18px; }.crypto-status-dot { height:7px; width:7px; border-radius:50%; background:var(--pay-green); box-shadow:0 0 0 4px #16866e12; flex-shrink:0; }.is-pending { animation:crypto-pulse 2s ease-in-out infinite; }
.crypto-footer { display:flex; align-items:center; justify-content:space-between; gap:16px; border-top:1px solid var(--pay-line); padding:18px 28px; background:var(--pay-soft); }.crypto-footer-help { color:var(--pay-muted); font-size:12px; line-height:1.6; }.crypto-footer-actions { display:flex; gap:10px; flex-shrink:0; }.crypto-cancel { margin-top:4px; text-decoration:underline; text-underline-offset:3px; min-height:28px; }.crypto-button-primary,.crypto-button-secondary { display:inline-flex; justify-content:center; align-items:center; gap:8px; min-height:44px; padding:11px 18px; border-radius:10px; font-size:14px; font-weight:650; cursor:pointer; }.crypto-button-primary { background:#2563eb; color:white; border:1px solid #2563eb; }.crypto-button-primary:hover { background:#1d4ed8; }.crypto-button-secondary { background:var(--pay-surface); color:var(--pay-muted); border:1px solid var(--pay-line); }.crypto-button-secondary:hover,.crypto-copy-address:hover { filter:brightness(.97); }
.crypto-checkout button { transition:background-color .16s,color .16s,opacity .16s; }.crypto-checkout button:disabled { opacity:.55; cursor:wait; }.crypto-checkout :is(button,a):focus-visible { outline:3px solid var(--pay-blue); outline-offset:3px; }.crypto-error { border-radius:12px; padding:14px; color:#b42318; background:#fff0ee; font-size:14px; }.crypto-loading,.crypto-terminal-state { display:flex; flex-direction:column; align-items:center; justify-content:center; gap:16px; padding:60px 24px; text-align:center; }.crypto-terminal-state { color:var(--pay-green); }.crypto-spinner { display:inline-block; width:18px; height:18px; border:2px solid currentColor; border-right-color:transparent; border-radius:50%; animation:crypto-spin .8s linear infinite; }.crypto-cancel-confirm { display:flex; flex-wrap:wrap; justify-content:center; align-items:center; gap:12px; border-top:1px solid var(--pay-line); padding-top:18px; margin-top:18px; font-size:13px; }.crypto-cancel-confirm p { width:100%; text-align:center; }
@keyframes crypto-spin { to { transform:rotate(360deg); } }@keyframes crypto-pulse { 50% { opacity:.45; } }
@media(max-width:600px) { .crypto-checkout { border-radius:18px; }.crypto-header { padding:18px; gap:11px; }.crypto-brand { width:42px; height:42px; border-radius:12px; }.crypto-brand img { width:27px; height:27px; }.crypto-header h1 { font-size:21px; }.crypto-header p { font-size:12px; max-width:220px; }.network-badge { display:none; }.crypto-body { padding:18px; }.crypto-total { padding:16px; gap:10px; }.crypto-amount { gap:5px; }.crypto-amount strong { font-size:28px; }.crypto-amount>span { font-size:12px; }.crypto-amount .crypto-icon-button { min-width:32px; }.crypto-countdown strong { font-size:24px; }.crypto-countdown .crypto-label { font-size:11px; }.crypto-countdown svg { display:none; }.crypto-transfer-grid { grid-template-columns:1fr; gap:20px; }.crypto-qr-column { width:190px; margin:auto; }.crypto-fiat { font-size:12px; }.crypto-footer { padding:16px 18px; flex-direction:column; align-items:stretch; gap:12px; }.crypto-footer-help { text-align:center; }.crypto-footer-actions>* { flex:1; }.crypto-footer-actions>.crypto-button-secondary { flex:0 0 auto; min-width:76px; }.crypto-notice { padding:12px; }.crypto-status { font-size:12px; } }
@media(prefers-reduced-motion:reduce) { .crypto-checkout *, .crypto-checkout *::before { animation:none !important; transition:none !important; } }
</style>
