(() => {
 'use strict';
 const el = id => document.getElementById(id);
 const token = new URLSearchParams(location.hash.slice(1)).get('bridge_token') || '';
 const pending = new Map(); let seq = 0, saved = false, savedFields = '';
 const fieldValues = () => JSON.stringify(['wallet','api-key','rate','fee-rate'].map(id=>el(id).value).concat(el('fee-enabled').checked));
 const request = (type, extra = {}) => new Promise((resolve,reject) => {
  if (!token || parent === window) return reject(new Error('请从插件管理打开配置页面。'));
  const id = `usdt-${++seq}`;
  const timer = setTimeout(() => { pending.delete(id); reject(new Error('请求超时，请重试。')); },120000);
  pending.set(id,{resolve,reject,timer,type});
  parent.postMessage({source:'sub2api-plugin-ui',bridge_token:token,type,request_id:id,...extra},'*');
 });
 const message = (text,error=false) => {el('message').textContent=text;el('message').classList.toggle('error',error);if(error)el('message').focus();};
 const numeric = (value, places, min, max) => {
  const text = value.trim();
  if (!new RegExp(`^\\d+(?:\\.\\d{1,${places}})?$`).test(text)) return null;
  const number = Number(text);
  return Number.isFinite(number) && number >= min && number <= max ? number : null;
 };
 function preview(){
  el('fee-rate').disabled=!el('fee-enabled').checked;
  const rate=numeric(el('rate').value,4,0.0001,100000), fee=el('fee-enabled').checked?numeric(el('fee-rate').value,2,0,100):0;
  if(rate===null||fee===null){el('quote-preview').textContent='填写有效汇率与手续费后，将显示计价示例。';return;}
  const total=100+Math.round(fee*100)/100;
  el('quote-preview').textContent=`以 100 CNY 计价基数为例：手续费 ${fee.toFixed(2)} CNY，应付 ${total.toFixed(2)} CNY，约 ${(Math.ceil(total/rate*100)/100).toFixed(2)} USDT，再加识别尾数。实际以订单为准。`;
 }
 function fill(c){
  el('wallet').value=c.wallet_address || ''; el('api-key').value=''; saved=!!c.api_key_set;
  el('rate').value=String(c.rate_cny ?? 6.5);el('fee-enabled').checked=c.fee_enabled===true;el('fee-rate').value=String(c.fee_rate ?? 0);
  el('key-state').textContent=saved?'已安全保存':'';el('api-key').placeholder=saved?'留空保留已保存的 Key':'填写 API Key';preview();
  savedFields=fieldValues();el('saved-wallet').textContent=c.wallet_address || '尚未设置';
 }
 const busy = value => {el('fields').disabled=value;};
 window.addEventListener('message',event => {
  const d=event.data;
  if(event.source!==parent||!d||d.source!=='sub2api-plugin-host'||d.bridge_token!==token)return;
  const p=pending.get(d.request_id);if(!p||d.type!==`${p.type}.result`)return;
  clearTimeout(p.timer);pending.delete(d.request_id);
  if(d.ok)p.resolve(d);else p.reject(new Error(d.error||d.result?.message||'操作失败，请检查配置。'));
 });
 function fieldError(id,errorID,text){el(id).setAttribute('aria-invalid',String(!!text));el(errorID).textContent=text;el(errorID).hidden=!text;}
 async function save(){
  const wallet=el('wallet').value.trim(),key=el('api-key').value.trim();
  const rate=numeric(el('rate').value,4,0.0001,100000), parsedFee=numeric(el('fee-rate').value,2,0,100);
  const fee=el('fee-enabled').checked?parsedFee:(parsedFee??0);
  const errors=[['rate','rate-error',rate===null?'汇率必须为 0.0001–100000，最多 4 位小数。':''],['fee-rate','fee-error',fee===null?'手续费率必须为 0–100%，最多 2 位小数。':''],['wallet','wallet-error',!/^T[1-9A-HJ-NP-Za-km-z]{33}$/.test(wallet)?'请填写有效的 TRON 收款地址。':''],['api-key','key-error',(!saved&&!key)||key&&!/^[A-Za-z0-9_-]{16,256}$/.test(key)?'请填写有效的 TronScan API Key。':'']];
  errors.forEach(([id,errorID,text])=>fieldError(id,errorID,text));
  const invalid=errors.filter(item=>item[2]);
  if(invalid.length){message(invalid.map(item=>item[2]).join(' '),true);return;}
  busy(true);message('正在加密保存配置…');
  try{const r=await request('config.save',{config:{wallet_address:wallet,tronscan_api_key:key,rate_cny:rate,fee_enabled:el('fee-enabled').checked,fee_rate:fee}});fill(r.config);message('已保存。新订单使用下方已生效的收款地址、汇率与手续费；已有订单保留原信息并继续核验。请测试连接。');}
  catch(e){message(e.message,true);}finally{busy(false);}
 }
 el('save').addEventListener('click',save);
 el('test').addEventListener('click',async()=>{if(fieldValues()!==savedFields){message('有未保存的修改。请先点击“保存配置”，再测试连接。',true);return;}busy(true);message('正在测试已保存的配置…');try{const r=await request('config.test');message(r.result.message,!r.result.success);}catch(e){message(e.message,true);}finally{busy(false);}});
 el('form').addEventListener('input',()=>{if(fieldValues()!==savedFields)message('有未保存的修改，付款页仍使用下方已生效的收款地址。请保存后测试连接。');});
 ['rate','fee-rate'].forEach(id=>el(id).addEventListener('input',preview));el('fee-enabled').addEventListener('change',preview);
 el('form').addEventListener('submit',e=>e.preventDefault());
 el('form').addEventListener('keydown',e=>{if(e.key==='Enter'&&!e.isComposing&&e.target instanceof HTMLInputElement){e.preventDefault();save();}});
 parent.postMessage({source:'sub2api-plugin-ui',bridge_token:token,type:'sub2api.plugin.ready'},'*');
 request('config.load').then(r=>{document.documentElement.classList.toggle('dark',r.context?.theme==='dark');fill(r.config||{});busy(false);message(saved?'配置已读取，API Key 已隐藏。':'填写收款信息与计价规则，保存后测试连接。');}).catch(e=>message(e.message,true));
})();
