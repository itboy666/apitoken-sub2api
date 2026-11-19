<template>
  <Teleport to="body">
    <dialog ref="dialog" class="crypto-payment-dialog" aria-labelledby="crypto-dialog-title" @cancel.prevent="emit('close')" @close="onNativeClose" @keydown="trapFocus">
      <CryptoCheckoutPanel v-if="show" :out-trade-no="outTradeNo" modal @close="emit('close')" @settled="emit('settled', $event)" />
    </dialog>
  </Teleport>
</template>
<script setup lang="ts">
import { ref, watch, nextTick, onUnmounted } from 'vue'
import CryptoCheckoutPanel from './CryptoCheckoutPanel.vue'
const props = defineProps<{ show: boolean; outTradeNo: string }>()
const emit = defineEmits<{ close: []; settled: [status: string] }>()
const dialog = ref<HTMLDialogElement | null>(null)
let previousFocus: HTMLElement | null = null
function trapFocus(event: KeyboardEvent) {
  if (event.key !== 'Tab' || !dialog.value) return
  const controls = [...dialog.value.querySelectorAll<HTMLElement>('button:not(:disabled), a[href], [tabindex="0"]')].filter(element => element.getClientRects().length)
  const first = controls[0]
  const last = controls[controls.length - 1]
  if (first && last && ((event.shiftKey && document.activeElement === first) || (!event.shiftKey && document.activeElement === last))) {
    event.preventDefault()
    ;(event.shiftKey ? last : first).focus()
  }
}
function onNativeClose() {
  // A queued close event from a previous opening must not close a reopened dialog.
  if (props.show && !dialog.value?.open) emit('close')
}
watch(() => props.show, async (show) => {
  await nextTick()
  if (!dialog.value || show !== props.show) return
  if (show && !dialog.value.open) {
    previousFocus = document.activeElement as HTMLElement
    dialog.value.showModal()
    document.body.classList.add('crypto-payment-open')
  } else if (!show && dialog.value.open) {
    dialog.value.close()
    document.body.classList.remove('crypto-payment-open')
    previousFocus?.focus()
  }
}, { immediate: true })
onUnmounted(() => { dialog.value?.close(); document.body.classList.remove('crypto-payment-open'); previousFocus?.focus() })
</script>
<style scoped>
.crypto-payment-dialog { width:min(860px,calc(100vw - 32px)); max-width:none; max-height:calc(100dvh - 40px); margin:auto; padding:0; border:0; border-radius:24px; background:transparent; color:inherit; overflow:auto; overscroll-behavior:contain; box-shadow:0 30px 100px #0f203442; }
.crypto-payment-dialog::backdrop { background:#1c2c4b72; backdrop-filter:blur(7px); }
.crypto-payment-dialog[open] { animation:crypto-dialog-in .2s ease-out; }
:global(body.crypto-payment-open) { overflow:hidden; }
@keyframes crypto-dialog-in { from { opacity:0; transform:translateY(12px) scale(.985); } to { opacity:1; transform:none; } }
@media(max-width:600px) { .crypto-payment-dialog { width:calc(100vw - 20px); max-height:calc(100dvh - 24px); border-radius:18px; } }
@media(prefers-reduced-motion:reduce) { .crypto-payment-dialog[open] { animation:none; } }
</style>
