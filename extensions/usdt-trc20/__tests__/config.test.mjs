import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

const source = readFileSync(new URL('../ui/config.js', import.meta.url), 'utf8');
function page() {
 const nodes = new Map(), sent = [], listeners = new Map();
 const get = id => {
  if (!nodes.has(id)) nodes.set(id, {value:'',checked:false,disabled:id==='fields',hidden:false,textContent:'',events:{},attrs:{},
   classList:{toggle(){}},setAttribute(k,v){this.attrs[k]=v;},focus(){this.focused=true;},
   addEventListener(name,fn){this.events[name]=fn;},
   set innerHTML(_) { throw new Error('Unsafe HTML sink'); },
  });
  return nodes.get(id);
 };
 const parent={postMessage(message){sent.push(message);}};
 const window={addEventListener(name,fn){listeners.set(name,fn);}};
 vm.runInNewContext(source,{document:{getElementById:get,documentElement:{classList:{toggle(){}}}},location:{hash:'#bridge_token=test-session'},parent,window,URLSearchParams,HTMLInputElement:class{},setTimeout(){return 1;},clearTimeout(){}});
 const deliver=(request,extra={},sender=parent,token='test-session')=>listeners.get('message')({source:sender,data:{source:'sub2api-plugin-host',bridge_token:token,type:request.type+'.result',request_id:request.request_id,ok:true,...extra}});
 const load = async () => {deliver(sent.find(m=>m.type==='config.load'),{config:{wallet_address:'TWwECvk3qmaXWvQd6FrGU5rK6AJ78dyFB9',api_key_set:true,rate_cny:6.5,fee_enabled:true,fee_rate:4}});await Promise.resolve();await Promise.resolve();};
 return {get,sent,deliver,load};
}

test('editable rate and independent fee are sent without exposing the saved key',async()=>{
 const p=page();await p.load();assert.equal(p.get('api-key').value,'');
 p.get('rate').value='7.1234';p.get('fee-rate').value='1.25';p.get('fee-enabled').checked=false;
 const saving=p.get('save').events.click();const request=p.sent.at(-1);
 assert.equal(request.type,'config.save');assert.equal(request.config.rate_cny,7.1234);assert.equal(request.config.fee_enabled,false);assert.equal(request.config.fee_rate,1.25);assert.equal(request.config.tronscan_api_key,'');
 p.deliver(request,{config:{...request.config,api_key_set:true}});await saving;assert.match(p.get('message').textContent,/已有订单/);assert.equal(p.get('fee-rate').disabled,true);
});

test('invalid precision, negative values, and HTML never reach config.save',async()=>{
 for(const [field,value] of [['rate','0'],['rate','6.12345'],['rate','<img src=x onerror=alert(1)>'],['fee-rate','-1'],['fee-rate','1.234']]){
  const p=page();await p.load();p.get(field).value=value;await p.get('save').events.click();
  assert.equal(p.sent.filter(m=>m.type==='config.save').length,0);assert.equal(p.get(field).attrs['aria-invalid'],'true');assert.equal(p.get('message').focused,true);
 }
});

test('switching off the fee allows saving after clearing its disabled input',async()=>{
 const p=page();await p.load();p.get('fee-rate').value='';p.get('fee-enabled').checked=false;
 const saving=p.get('save').events.click();const request=p.sent.at(-1);
 assert.equal(request.type,'config.save');assert.equal(request.config.fee_enabled,false);assert.equal(request.config.fee_rate,0);
 p.deliver(request,{config:{...request.config,api_key_set:true}});await saving;
 assert.equal(p.get('fee-rate').value,'0');assert.equal(p.get('fee-rate').disabled,true);
});

test('forged parent or session messages cannot populate the configuration',async()=>{
 const p=page(), request=p.sent.find(m=>m.type==='config.load');
 p.deliver(request,{config:{wallet_address:'attacker'}},{},'test-session');
 p.deliver(request,{config:{wallet_address:'attacker'}},undefined,'wrong-session');
 await Promise.resolve();assert.equal(p.get('wallet').value,'');assert.equal(p.get('fields').disabled,true);
 await p.load();assert.equal(p.get('wallet').value,'TWwECvk3qmaXWvQd6FrGU5rK6AJ78dyFB9');
});

test('server error messages remain text and cannot inject markup',async()=>{
 const p=page();await p.load();const saving=p.get('save').events.click();const request=p.sent.at(-1);
 p.deliver(request,{ok:false,error:'<img src=x onerror=alert(1)>'});await saving;
 assert.equal(p.get('message').textContent,'<img src=x onerror=alert(1)>');
});

test('editing a wallet distinguishes unsaved text from the effective address',async()=>{
 const p=page();await p.load();const original=p.get('saved-wallet').textContent;
 p.get('wallet').value='TNUrv3PEVuuQkJRaSb2tGmFWJYg9PQ5Vx5';p.get('form').events.input();
 await p.get('test').events.click();
 assert.match(p.get('message').textContent,/未保存/);assert.equal(p.get('saved-wallet').textContent,original);
 assert.equal(p.sent.filter(m=>m.type==='config.test').length,0);
 const saving=p.get('save').events.click();const request=p.sent.at(-1);
 p.deliver(request,{config:{...request.config,api_key_set:true}});await saving;
 assert.equal(p.get('saved-wallet').textContent,'TNUrv3PEVuuQkJRaSb2tGmFWJYg9PQ5Vx5');
 const testing=p.get('test').events.click();p.deliver(p.sent.at(-1),{result:{success:false,message:'TronScan 鉴权失败（HTTP 401）'}});await testing;
 assert.match(p.get('message').textContent,/401/);assert.equal(p.get('message').focused,true);
});

test('a rejected wallet save retains the actual saved address',async()=>{
 const p=page();await p.load();const original=p.get('saved-wallet').textContent;
 p.get('wallet').value='TNUrv3PEVuuQkJRaSb2tGmFWJYg9PQ5Vx5';
 const saving=p.get('save').events.click();p.deliver(p.sent.at(-1),{ok:false,error:'地址未保存'});await saving;
 assert.equal(p.get('saved-wallet').textContent,original);assert.match(p.get('message').textContent,/未保存/);
});
