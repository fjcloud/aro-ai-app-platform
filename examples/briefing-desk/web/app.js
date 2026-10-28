'use strict';
const $ = id => document.getElementById(id);
const labels = {buy:'BUY',short:'SHORT',no_signal:'NO SIGNAL',running:'RUNNING',queued:'QUEUED',summarizing:'READING',failed:'FAILED',ready:'READY'};
const terminalProfile = 'tech';
let state = {documents:[],profiles:[]}, market = null, active = null, filter = 'all', detailKey = '', rowKey = '';
const edits = new Map();
function el(tag,text,cls) {const n=document.createElement(tag);if(text!==undefined)n.textContent=text;if(cls)n.className=cls;return n;}
function notice(text) {$('notice').hidden=!text;$('notice').textContent=text;}
function link(text,raw) {const a=el('a',text);try{const u=new URL(raw);if(['http:','https:'].includes(u.protocol)){a.href=u.href;a.target='_blank';a.rel='noopener noreferrer';}}catch{}return a;}
function money(v) {return Number.isFinite(v)?new Intl.NumberFormat('en-US',{style:'currency',currency:'USD',maximumFractionDigits:2}).format(v):'—';}
function utc(raw) {const d=new Date(raw);return Number.isNaN(d.getTime())?'—':d.toISOString().slice(11,16);}
function fullDate(raw) {const d=new Date(raw);return Number.isNaN(d.getTime())?'Unavailable':d.toISOString().replace('T',' ').slice(0,19)+' UTC';}
async function api(path,body) {
 const r=await fetch('/api/'+path,body?{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)}:{});
 if(!r.ok){const out=await r.json().catch(()=>({error:'Request failed'}));throw Error(out.error||'Request failed');}return r.json();
}
function button(text,fn,cls='') {
 const b=el('button',text,cls);b.onclick=async e=>{e.stopPropagation();b.disabled=true;try{notice('');await fn();}catch(err){notice(err.message);}finally{b.disabled=false;}};return b;
}
function currentProfile() {return state.profiles.find(p=>p.id===terminalProfile);}
function fresh(d) {const c=d.classifications[terminalProfile];return c?.status==='done'&&!c.stale&&c.profile_version===currentProfile()?.version&&['buy','short','no_signal'].includes(c.priority);}
function directional(d) {return fresh(d)&&['buy','short'].includes(d.classifications[terminalProfile].priority);}
function docs() {return state.documents.filter(d=>d.source.feed_url?.includes('techmeme.com')||d.source.coverage==='synthetic_demo'||d.classifications[terminalProfile]);}
async function select(id) {active=id;detailKey='';rowKey='';renderRows();await renderDetail();}
async function investigate(d) {active=d.id;detailKey='';if(state.preview){await select(d.id);return;}await api('articles/'+d.id+'/investigate',{profile_id:terminalProfile});await refresh();}
async function rank(d) {await api('articles/'+d.id+'/classify',{profile_id:terminalProfile});await refresh();}
async function retry(d) {await api('import',{url:d.url,profile_id:terminalProfile});await refresh();}
function renderRows() {
 const list=docs(), key=JSON.stringify([list,active,filter]);if(key===rowKey)return;rowKey=key;
 const totals={all:list.length,buy:0,short:0,no_signal:0};for(const d of list){if(fresh(d))totals[d.classifications[terminalProfile].priority]++;}for(const b of document.querySelectorAll('[data-filter]')){const f=b.dataset.filter;b.textContent=(f==='all'?'ALL':labels[f])+' '+totals[f];}
 const rows=$('rows');rows.replaceChildren();let count=0;
 for(const d of list){const c=d.classifications[terminalProfile];if(filter!=='all'&&(!fresh(d)||c.priority!==filter))continue;count++;
  const tr=el('tr',undefined,'story'+(d.id===active?' selected':''));tr.dataset.id=d.id;tr.tabIndex=0;tr.setAttribute('aria-selected',String(d.id===active));tr.onclick=()=>select(d.id).catch(e=>notice(e.message));tr.onkeydown=e=>{if(e.key==='Enter'){e.preventDefault();select(d.id).catch(err=>notice(err.message));}};
  const time=el('td',utc(d.source.published||d.created),'time'),story=el('td');let host='Source';try{host=new URL(d.url).hostname.replace(/^www\./,'');}catch{}
  story.append(el('div',state.preview?'SYNTHETIC DEMO':host,'source'),el('div',d.summary.title||d.source.title||'Reading article…','headline'),el('p',d.summary.tldr||'Waiting for the retrieval agent.','summary'));
  if(d.summary.market_context){const context=el('p',d.summary.market_context,'market-context');context.title='Financial interpretation / hypothesis; verified during investigation';story.append(context);}
  if(d.error||c?.error)story.append(el('p',d.error||c.error,'error'));
  const code=d.status!=='ready'?d.status:fresh(d)?c.priority:c?.status==='failed'?'failed':'queued';const signal=el('td');signal.append(el('span',labels[code]||'RANK NEEDED','tag '+code));const score=el('td',fresh(d)?Math.round(c.confidence*100)+'%':'—','score'),action=el('td');
  if(d.status==='failed'){action.append(button('RETRY',()=>retry(d),'action'));}
  else if(directional(d)){const r=d.research?.[terminalProfile],b=button(state.preview?'OPEN DEMO':r?.status==='running'?'WORKING…':'INVESTIGATE',()=>investigate(d),'action '+c.priority);b.disabled=r?.status==='running';action.append(b);}
  else if(d.status==='ready'&&!fresh(d)){const b=button('RANK',()=>rank(d),'action');b.disabled=state.preview||['running','queued'].includes(c?.status);action.append(b);}
  else if(d.status==='ready'){action.append(button('READ',()=>select(d.id),'action'));}
  tr.append(time,story,signal,score,action);rows.append(tr);
 }
 $('count').textContent=String(count);$('empty').hidden=count>0;
}
function activity(items) {const list=el('ul',undefined,'activity');for(const e of items||[]){const li=el('li',undefined,e.stage);li.append(el('time',utc(e.at)),document.createTextNode(e.message+(e.duration_ms?' · '+(e.duration_ms/1000).toFixed(1)+'s':'')));list.append(li);}return list;}
function addFindings(box,title,items,cls='') {if(!items?.length)return;box.append(el('h4',title,'section-label'));const ul=el('ul',undefined,'insights '+cls);for(const item of items){ul.append(el('li',typeof item==='string'?item:item.text+' ['+item.sources.join(', ')+']'));}box.append(ul);}
function exportURL(d,r) {const u=new URL('/api/export/'+d.id,location.origin);u.searchParams.set('profile','tech');u.searchParams.set('type','research');const v=edits.get(d.id+':'+r.at);if(v){u.searchParams.set('notional',v.notional);u.searchParams.set('target',v.target);u.searchParams.set('stop',v.stop);}return u.pathname+u.search;}
function renderPlan(d,r) {
 const p=r.report.plan;if(!p)return null;const box=el('section',undefined,'paper'),head=el('div',undefined,'paper-head');head.append(el('b',p.symbol||'NO TRADE'),el('small','ETF / PAPER SCENARIO'),el('span',labels[p.direction]||p.direction,'tag '+p.direction));box.append(head);
 const body=el('div',undefined,'paper-body');body.append(el('p',p.thesis,'thesis'));const q=p.quote;
 if(p.direction==='no_signal'||p.pricing_error||!q||q.error||q.stale||!q.as_of||Date.now()-new Date(q.as_of).getTime()>72*3600000){body.append(el('p',p.pricing_error||'No sourced reference quote is available.','error'),el('p','No entry price or P&L is generated without a valid market reference.','fine'));box.append(body);return box;}
 body.append(link(q.instrument.index+' · issuer details ↗',q.instrument.issuer_url));
 const key=d.id+':'+r.at;let values=edits.get(key)||{notional:10000,target:p.target_move_pct,stop:p.stop_move_pct};edits.set(key,values);
 const assumptions=el('div',undefined,'assumptions');const inputs={};
 for(const [name,label,min,max,step]of[['notional','NOTIONAL / USD',100,1000000,100],['target','TARGET MOVE / %',.1,25,.1],['stop','STOP MOVE / %',.1,15,.1]]){const wrap=el('label',label),input=el('input');input.type='number';input.min=min;input.max=max;input.step=step;input.value=values[name];input.dataset.field=name;wrap.append(input);assumptions.append(wrap);inputs[name]=input;}
 body.append(assumptions);const grid=el('div',undefined,'price-grid');const fields={};for(const [key,title,cls]of[['entry','REFERENCE ENTRY',''],['target','TARGET CASE','green'],['stop','STOP CASE','red']]){const cell=el('div'),price=el('strong','—',cls),pnl=el('em','',cls);cell.append(el('small',title),price,pnl);grid.append(cell);fields[key]={price,pnl};}body.append(grid);
 function calculate(){for(const key of Object.keys(inputs))values[key]=Number(inputs[key].value);const valid=Object.values(inputs).every(n=>n.checkValidity())&&Object.values(values).every(Number.isFinite);const direction=p.direction==='short'?-1:1,entry=q.price;
  fields.entry.price.textContent=money(entry);fields.entry.pnl.textContent=(values.notional/entry).toFixed(2)+' units';
  fields.target.price.textContent=valid?money(entry*(1+direction*values.target/100)):'—';fields.stop.price.textContent=valid?money(entry*(1-direction*values.stop/100)):'—';
  const download=$('research-export');if(download){if(valid){download.href=exportURL(d,r);download.removeAttribute('aria-disabled');}else{download.removeAttribute('href');download.setAttribute('aria-disabled','true');}}
  fields.target.pnl.textContent=valid?'+'+money(values.notional*values.target/100)+' gross P&L':'Check inputs';fields.stop.pnl.textContent=valid?'−'+money(values.notional*values.stop/100)+' gross P&L':'Check inputs';
 }
 for(const n of Object.values(inputs))n.oninput=calculate;calculate();
 body.append(el('p',(state.preview?'SYNTHETIC REFERENCE':'REFERENCE QUOTE')+' · '+fullDate(q.as_of)+' · '+p.horizon_days+' sessions assumed','fine'),link('Quote source ↗',q.url),el('p',p.assumption,'fine'));box.append(body);return box;
}
async function renderDetail() {
 if(!active)return;const d=await api('articles/'+active);if(d.id!==active)return;
 const key=JSON.stringify([d,state.preview]);if(key===detailKey)return;detailKey=key;const box=$('detail'),open=new Set([...box.querySelectorAll('details[open]')].map(n=>n.dataset.key));box.replaceChildren();
 const c=d.classifications[terminalProfile],r=d.research?.[terminalProfile],sub=el('div',undefined,'subline');sub.append(el('span',labels[c?.priority]||'SIGNAL PENDING','tag '+(c?.priority||'')),link('Publisher ↗',d.source.final_url||d.url));if(d.source.coverage==='rss_excerpt')sub.append(el('span','RSS EXCERPT ONLY','coverage'));if(state.preview)sub.append(el('span','SYNTHETIC FIXTURE','coverage'));box.append(sub,el('h3',d.summary.title||d.source.title||'Reading article…'),el('p',d.summary.tldr||'Waiting for an English summary.','lead'));
 if(d.summary.market_context){box.append(el('div','MARKET READ / HYPOTHESIS','source'),el('p',d.summary.market_context,'market-context'));}
 const actions=el('div',undefined,'detail-actions');if(directional(d)){const b=button(state.preview?'DEMO ANALYSIS':r?.status==='running'?'INVESTIGATING…':r?.status==='done'?'INVESTIGATE AGAIN ↻':'INVESTIGATE ↗',()=>investigate(d),state.preview?'':'orange');b.disabled=state.preview||r?.status==='running';actions.append(b);}if(d.status==='ready'){const a=el('a','SUMMARY ↓');a.href='/api/export/'+d.id+'?profile=tech';actions.append(a);}box.append(actions);if(d.error)box.append(el('p',d.error,'error'));
 if(r){if(r.error)box.append(el('p',r.error,'error'));if(r.status==='running'){const log=el('details');log.open=true;log.dataset.key='activity';log.append(el('summary','AGENT ACTIVITY / LIVE'),activity(r.events));box.append(log);}
  if(r.status==='done'){const plan=renderPlan(d,r);if(plan)box.append(plan);addFindings(box,'SOURCE FACTS',r.report.facts?.slice(0,3));addFindings(box,'FINANCIAL THESIS / CONDITIONAL',r.report.hypotheses?.slice(0,2),'hypothesis');
   const full=el('details');full.dataset.key='research';full.open=open.has('research');full.append(el('summary','FULL RESEARCH / SOURCES / ACTIVITY'));addFindings(full,'UNKNOWN / UNCONFIRMED',r.report.unknowns);addFindings(full,'NEXT CHECKS',r.report.next_checks);
   for(const e of r.evidence||[]){const source=el('details',undefined,'evidence-item');source.dataset.key=e.id;source.open=open.has(e.id);source.append(el('summary',e.id+' / '+e.tool),link('Open source ↗',e.url),el('pre',JSON.stringify(e.data,null,2)));full.append(source);}full.append(activity(r.events));if(r.trace_id)full.append(el('p','TRACE / '+r.trace_id,'fine'));box.append(full);
   const a=el('a','RESEARCH NOTE / MARKDOWN ↓');a.id='research-export';a.href=exportURL(d,r);box.append(a);
  }
 }else{if(c?.priority==='no_signal')box.append(el('p','No clear directional company or sector catalyst in the available source. Neutral stories remain in the feed.','fine'));else box.append(el('p','Investigate to map this signal to an index proxy, verify market references and examine a conditional return scenario.','fine'));const summary=el('details');summary.dataset.key='summary';summary.open=open.has('summary');summary.append(el('summary','ARTICLE NOTES'));addFindings(summary,'KEY POINTS',d.summary.points);summary.append(el('p',d.summary.overview||'','lead'),activity(d.events));box.append(summary);}
}
async function refresh() {
 state=await api('state');$('preview').hidden=!(state.preview||state.validation_fixture);if(state.validation_fixture)$('preview').textContent='LOCAL VALIDATION · MODEL, NEWS AND PRICE FIXTURES · NO LIVE INFERENCE';$('fetch').disabled=state.preview||state.scan.status==='running';$('fetch').textContent=state.scan.status==='running'?'FETCHING…':'FETCH 12 STORIES ↻';$('url').disabled=state.preview;$('import').querySelector('button').disabled=state.preview;$('connection').textContent=state.preview?'LOCAL PREVIEW / MODELS DISCONNECTED':state.inference?.agent_backend==='azure'?('d3-mini · '+(state.inference.decision_device||'local')+' / Azure Foundry cloud · DeepSeek-V4-Pro'):state.inference?.agent_backend==='groq'?('d3-mini · '+(state.inference.decision_device||'local')+' / Groq cloud · '+state.agent):'OpenShift AI / d3-mini · T4';
 const list=docs();if(!active&&list.length)active=(state.preview?list.find(d=>directional(d)&&d.research?.tech?.status==='done'):null)?.id||list[0].id;renderRows();const status=$('scan-status');status.hidden=!state.scan.status||state.scan.status==='done';status.textContent=state.scan.error||state.scan.events?.at(-1)?.message||'Retrieving Techmeme…';await renderDetail();
}
async function refreshMarket(){try{market=await api('market');$('market').replaceChildren(...market.quotes.map(q=>{const node=el('div',undefined,'quote');node.append(el('b',q.instrument.symbol),el('strong',q.error?'—':money(q.price)),link(q.instrument.index,q.instrument.issuer_url),el('small',state.preview?'DEMO':q.error?'UNAVAILABLE':(q.stale?'STALE · ':'REF · ')+(q.as_of||'').slice(5,10)+' '+utc(q.as_of)+'Z','asof'));node.title=q.error||q.basis+' · '+fullDate(q.as_of);return node;}));}catch{$('market').textContent='Reference quotes unavailable';}}
$('fetch').onclick=async()=>{try{notice('');await api('scan',{feed_id:'techmeme',profile_id:terminalProfile});await refresh();}catch(e){notice(e.message);}};
$('import').onsubmit=async e=>{e.preventDefault();try{notice('');const out=await api('import',{url:$('url').value,profile_id:terminalProfile});$('url').value='';await refresh();await select(out.id);}catch(err){notice(err.message);}};
for(const b of document.querySelectorAll('[data-filter]'))b.onclick=()=>{filter=b.dataset.filter;for(const n of document.querySelectorAll('[data-filter]'))n.setAttribute('aria-pressed',String(n===b));renderRows();};
let pending=false;async function tick(){if(pending)return;pending=true;try{await refresh();}catch(e){notice('Connection interrupted. Retrying automatically.');}finally{pending=false;}}
function clock(){$('clock').textContent=new Date().toISOString().slice(11,19)+' UTC';}clock();setInterval(clock,1000);
(async()=>{await tick();await refreshMarket();})();setInterval(tick,2500);setInterval(refreshMarket,60000);
