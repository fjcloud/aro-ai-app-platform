'use strict';
const $=id=>document.getElementById(id);
let state,busy=false,automatic=false,timer,controller,previousPhase=-1;
const label=a=>({clear:'Clear',inspect:'Inspect',reject:'Reject'}[a]||'Observer');
const percent=(n,d)=>d?`${Math.round(n/d*100)}%`:'—';
async function api(path,body,signal){const r=await fetch(path,{method:body===undefined?'GET':'POST',headers:body===undefined?{}:{'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body),signal});const data=await r.json();if(!r.ok)throw Error(data.error||`Request failed (${r.status})`);return data;}
function stop(){automatic=false;clearTimeout(timer);if(controller)controller.abort();if(state)render();}
function render(){
 if(!state)return;
 const s=state,result=s.result,resolved=s.history?.length||0;
 $('model').textContent=s.model.split('/').at(-1);
 $('human-score').textContent=percent(s.human_correct,s.human_played);$('human-detail').textContent=s.human_played?`${s.human_correct} of ${s.human_played} stamps correct`:'No stamps yet';
 $('ai-score').textContent=percent(s.model_correct,resolved);$('ai-detail').textContent=resolved?`${s.model_correct} of ${resolved} inspections correct`:'No inspections yet';
 $('points').textContent=s.score.toLocaleString();$('streak').textContent=s.streak?`${s.streak} correct in a row`:'Build a streak';
 $('round').replaceChildren(document.createTextNode(String(Math.min(s.round+1,s.total)).padStart(2,'0')),Object.assign(document.createElement('span'),{textContent:` / ${s.total}`}));
 $('status').textContent=busy?'AI reading documents…':s.over?'Shift complete':automatic?'AI on duty':result?'Inspection complete':'Awaiting your stamp';
 $('directive-name').textContent=s.directive.name;$('directive-text').textContent=s.directive.text;$('phase').textContent=`${Math.min(Math.floor(s.round/4)+1,3).toString().padStart(2,'0')} / 03`;
 const phase=Math.floor(s.round/4);if(previousPhase!==-1&&phase!==previousPhase&&!s.over){const panel=document.querySelector('.directive');panel.classList.remove('changed');void panel.offsetWidth;panel.classList.add('changed');}previousPhase=phase;
 $('orders').textContent=s.standing_orders;
 const image=$('document');if(!s.over&&image.getAttribute('src')!==s.document){image.src=s.document;image.alt=`Passport and cargo permit for courier ${s.round+1}`;}
 image.hidden=s.over;$('loading').hidden=!s.over;$('loading').textContent='GATE CLOSED / SHIFT COMPLETE';
 $('document-wrap').classList.toggle('thinking',busy);$('desk-label').textContent=s.over?'SHIFT ARCHIVED':`COURIER ${String(s.round+1).padStart(2,'0')} / DOCUMENTS ON DESK`;
 $('zoom').disabled=s.over||busy;$('reset').disabled=busy||automatic;$('seed').disabled=busy||automatic;
 $('verdict-controls').hidden=!!result||s.over;$('result').hidden=!result;$('summary').hidden=!s.over;
 document.querySelectorAll('[data-action]').forEach(b=>b.disabled=busy||automatic||!!result||s.over);
 $('ai-one').disabled=busy||automatic||!!result||s.over;$('run').disabled=s.over||(busy&&!automatic);$('run').textContent=automatic?'Ⅱ Pause AI shift':'▶ Watch AI shift';
 $('stamp').hidden=!result;$('stamp').className=`stamp ${result?.expected||''}`;if(result)$('stamp').textContent=label(result.expected).toUpperCase();
 if(result){
  const d=result.decision;$('human-verdict').textContent=label(result.human);$('human-verdict').className=result.human?(result.human_correct?'clear':'reject'):'';
  $('human-mark').textContent=result.human?(result.human_correct?'✓ Correct stamp':'✕ Incorrect stamp'):'AI handled this courier';
  $('ai-verdict').textContent=label(d.choice);$('ai-verdict').className=result.model_correct?'clear':'reject';$('ai-mark').textContent=result.model_correct?'✓ Correct stamp':'✕ Incorrect stamp';
  $('expected').textContent=label(result.expected);$('expected').className=result.expected;$('explanation').textContent=result.explanation;
  $('latency').textContent=`${(d.latency_ms/1000).toFixed(2)} s inference`;
  $('probabilities').replaceChildren(...Object.entries(d.probabilities).sort((a,b)=>b[1]-a[1]).map(([a,p])=>{const row=document.createElement('div');row.className='prob-row';const name=document.createElement('span');name.textContent=label(a);const bar=document.createElement('div');bar.className='bar';const fill=document.createElement('span');fill.style.width=`${p*100}%`;bar.append(fill);const v=document.createElement('small');v.textContent=`${(p*100).toFixed(1)}%`;row.append(name,bar,v);return row;}));
  $('model-detail').textContent=`Image + rulebook · ${d.input_tokens.toLocaleString()} input tokens · 0 generated tokens`;
  $('trace').textContent=d.trace_id?`MLflow trace tr-${d.trace_id}`:'';$('next').disabled=busy||automatic;$('next').textContent=s.round===s.total-1?'Close shift →':'Next courier →';
 }
 if(s.over){$('summary-title').textContent=s.human_played===s.total?(s.human_correct>s.model_correct?'You outread the machine.':s.human_correct<s.model_correct?'The machine takes this shift.':'A tie at the gate.'):'The gate is closed.';$('summary-text').textContent=`Your stamps: ${s.human_correct}/${s.human_played}. AI inspections: ${s.model_correct}/${resolved}. Your score: ${s.score.toLocaleString()}.`;}
 $('log-count').textContent=`${resolved} inspection${resolved===1?'':'s'}`;
 if(resolved)$('history').replaceChildren(...s.history.slice().reverse().map(r=>{const li=document.createElement('li');li.className=r.model_correct?'':'wrong';li.textContent=`${String(r.round).padStart(2,'0')} · AI ${label(r.decision.choice)} ${r.model_correct?'✓':'✕'}`;const detail=document.createElement('span');detail.textContent=`Rulebook: ${label(r.expected)} · ${r.human?`You: ${r.human_correct?'correct':'incorrect'}`:'Observer'}`;li.append(detail);return li;}));
 else{const li=document.createElement('li');li.textContent='Your first courier is waiting.';li.className='muted';$('history').replaceChildren(li);}
}
async function perform(path,body){
 if(busy)return false;busy=true;$('error').hidden=true;controller=new AbortController();render();
 try{state=await api(path,body,controller.signal);return true;}
 catch(e){automatic=false;clearTimeout(timer);if(e.name!=='AbortError'){$('error').textContent=`${e.message}. No new decision was recorded. Try again.`;$('error').hidden=false;}try{state=await api('/api/shift');}catch{}return false;}
 finally{busy=false;controller=undefined;render();}
}
async function decide(action){return perform('/api/shift/decide',{id:state.id,round:state.round,action});}
async function next(){return perform('/api/shift/next',{id:state.id,round:state.round});}
async function tick(){if(!automatic||busy)return;if(state.over){stop();return;}const ok=state.result?await next():await decide('watch');if(ok&&automatic){if(state.over)stop();else timer=setTimeout(tick,state.result?1800:120);}}
document.querySelectorAll('[data-action]').forEach(b=>b.addEventListener('click',()=>decide(b.dataset.action)));
$('ai-one').addEventListener('click',()=>decide('watch'));$('next').addEventListener('click',next);
$('run').addEventListener('click',()=>{if(automatic)stop();else{automatic=true;render();tick();}});
$('reset').addEventListener('click',async()=>{const seed=Number($('seed').value);if(!Number.isSafeInteger(seed)||seed<0||seed>2147483647){$('error').textContent='Choose a whole-number seed from 0 to 2147483647.';$('error').hidden=false;return;}previousPhase=-1;await perform('/api/shift/new',{seed});});
$('zoom').addEventListener('click',()=>{if(automatic)stop();$('zoom-image').src=state.document;$('zoom-dialog').showModal();});$('close-zoom').addEventListener('click',()=>$('zoom-dialog').close());
document.addEventListener('keydown',e=>{if(e.ctrlKey||e.metaKey||e.altKey||busy||automatic||!state||state.over||state.result||$('zoom-dialog').open||/INPUT|TEXTAREA|SELECT/.test(e.target.tagName))return;const action={c:'clear',i:'inspect',r:'reject'}[e.key.toLowerCase()];if(action){e.preventDefault();decide(action);}});
window.addEventListener('pagehide',stop);
(async()=>{try{state=await api('/api/shift');$('seed').value=state.seed;render();}catch(e){$('error').textContent=e.message;$('error').hidden=false;$('loading').textContent='Connection unavailable. Reload to try again.';}})();
