<template>
  <div class="crypto-page">
    <main class="mx-auto w-full max-w-[860px]">
      <CryptoCheckoutPanel :out-trade-no="String(route.params.outTradeNo || '')" />
    </main>
  </div>
</template>
<script setup lang="ts">
import { useRoute } from 'vue-router'
import CryptoCheckoutPanel from '@/components/payment/CryptoCheckoutPanel.vue'
const route = useRoute()
</script>
<style scoped>
.crypto-page { min-height:100dvh; padding:40px 24px; background:radial-gradient(ellipse at 15% 0%,#e8f3f5 0,transparent 55%),#f3f6fb; }
:global(.dark .crypto-page) { background:radial-gradient(ellipse at 15% 0%,#193339 0,transparent 55%),#101924; }
@media(max-width:600px) { .crypto-page { padding:16px 12px; } }
</style>
