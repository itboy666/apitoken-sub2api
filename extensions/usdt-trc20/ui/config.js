(() => {
 'use strict';
 const el = id => document.getElementById(id);
 const token = new URLSearchParams(location.hash.slice(1)).get('bridge_token') || '';
 const pending = new Map(); let seq = 0, saved = false;
 const request = (type, extra = {}) => new Promise((resolve,reject) => {
  if (!token || parent === window) return reject(new Error('请从插件管理打开配置页面。'));
  const id = `usdt-${++seq}`;
  const timer = setTimeout(() => { pending.delete(id); reject(new Error('请求超时，请重试。')); },120000);
  pending.set(id,{resolve,reject,timer,type});
  parent.postMessage({source:'sub2api-plugin-ui',bridge_token:token,type,request_id:id,...extra},'*');
 });
 const message = (text,error=false) => {el('message').textContent=text;el('message').classList.toggle('error',error);};
 const fill = c => {el('wallet').value=c.wallet_address || ''; el('api-key').value=''; saved=!!c.api_key_set; el('key-state').textContent=saved?'已安全保存':'';el('api-key').placeholder=saved?'留空保留已保存的 Key':'填写 API Key';};
 const busy = value => {el('fields').disabled=value;};
 window.addEventListener('message',event => {
  const d=event.data;
  if(event.source!==parent||!d||d.source!=='sub2api-plugin-host'||d.bridge_token!==token)return;
  const p=pending.get(d.request_id);if(!p||d.type!==`${p.type}.result`)return;
  clearTimeout(p.timer);pending.delete(d.request_id);
  if(d.ok)p.resolve(d);else p.reject(new Error(d.error||d.result?.message||'操作失败，请检查配置。'));
 });
 async function save(){
  const wallet=el('wallet').value.trim(),key=el('api-key').value.trim();
  if(!/^T[1-9A-HJ-NP-Za-km-z]{33}$/.test(wallet)){message('请填写有效的 TRON 收款地址。',true);el('wallet').focus();return;}
  if(!saved&&!key){message('首次配置需要填写 TronScan API Key。',true);el('api-key').focus();return;}
  busy(true);message('正在加密保存配置…');
  try{const r=await request('config.save',{config:{wallet_address:wallet,tronscan_api_key:key}});fill(r.config);message('配置已保存。可测试连接，然后在插件管理中启用。');}
  catch(e){message(e.message,true);}finally{busy(false);}
 }
 el('save').addEventListener('click',save);
 el('test').addEventListener('click',async()=>{busy(true);message('正在测试已保存的配置…');try{const r=await request('config.test');message(r.result.message);}catch(e){message(e.message,true);}finally{busy(false);}});
 el('form').addEventListener('submit',e=>e.preventDefault());
 el('form').addEventListener('keydown',e=>{if(e.key==='Enter'&&!e.isComposing&&e.target instanceof HTMLInputElement){e.preventDefault();save();}});
 parent.postMessage({source:'sub2api-plugin-ui',bridge_token:token,type:'sub2api.plugin.ready'},'*');
 request('config.load').then(r=>{document.documentElement.classList.toggle('dark',r.context?.theme==='dark');fill(r.config||{});busy(false);message(saved?'配置已读取，API Key 已隐藏。':'填写收款地址与 API Key，保存后测试连接。');}).catch(e=>message(e.message,true));
})();
