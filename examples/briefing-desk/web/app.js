'use strict';
const $=id=>document.getElementById(id), profileID='tech';
let state={documents:[],profiles:[]},active='',detail=null,memoDirty=false,renderKey='',busy=false;
function el(tag,text,cls){const n=document.createElement(tag);if(text!==undefined)n.textContent=text;if(cls)n.className=cls;return n;}
function notice(text){$('notice').hidden=!text;$('notice').textContent=text;}
async function api(path,body){const r=await fetch('/api/'+path,body?{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)}:{});const data=await r.json();if(!r.ok)throw Error(data.error||'Request failed');return data;}
function memo(){return state.profiles.find(p=>p.id===profileID);}
function scored(d){const c=d.classifications[profileID];return d.status==='ready'&&c?.status==='done'&&!c.stale&&c.profile_version===memo()?.version&&Number.isFinite(c.opportunity_score);}
function band(score){return score>=70?'high':score>=35?'watch':'low';}
function sourceLink(text,raw){const a=el('a',text);try{const u=new URL(raw);if(['http:','https:'].includes(u.protocol)){a.href=u.href;a.target='_blank';a.rel='noopener noreferrer';}}catch{}return a;}
function action(text,fn,cls){const b=el('button',text,cls);b.onclick=async()=>{b.disabled=true;try{notice('');await fn();}catch(e){notice(e.message);}finally{b.disabled=false;}};return b;}
function findings(parent,title,items){if(!items?.length)return;parent.append(el('h3',title));const ul=el('ul');for(const item of items)ul.append(el('li',typeof item==='string'?item:item.text));parent.append(ul);}
async function openStory(id){active=active===id?'':id;detail=active?await api('articles/'+id):null;renderKey='';render();}
function renderDetail(d){
 const box=el('section',undefined,'story-detail'),c=d.classifications[profileID];
 if(c?.memo_reason){box.append(el('h3','Why it matters for your memo'),el('p',c.memo_reason));}
 findings(box,'Key points',d.summary.points);
 box.append(el('p',state.preview?'Synthetic example; no publisher or model was called.':d.source.coverage==='rss_excerpt'?'Based on the Techmeme excerpt; the full publisher article was unavailable.':'Summary based on the retrieved publisher text.','status'));
 const actions=el('div',undefined,'actions');actions.append(sourceLink('Read original ↗',d.source.final_url||d.url));const summary=el('a','Save summary ↓');summary.href='/api/export/'+d.id+'?profile='+profileID;actions.append(summary);
 let r=detail?.id===d.id?detail.research?.[profileID]:null;
 if(r&&(r.profile_version!==memo()?.version||r.summary_hash!==c?.summary_hash||!scored(d)))r=null;
 if(scored(d)){const b=action(r?.status==='running'?'Investigating…':r?.status==='done'?'Investigate again':'Investigate',async()=>{await api('articles/'+d.id+'/investigate',{profile_id:profileID});await refresh();},'primary');b.disabled=state.preview||r?.status==='running';actions.prepend(b);}
 box.append(actions);
 if(r?.status==='running')box.append(el('p','The agent is reading sources and checking the financial implications for your memo.','status'));
 if(r?.error)box.append(el('p',r.error,'error'));
 if(r?.status==='done'){
  box.append(el('h2',r.report.headline));findings(box,'What the sources establish',r.report.facts);findings(box,'Possible implications for the fund',r.report.hypotheses);findings(box,'What still needs checking',r.report.unknowns);
  const download=el('a','Save research note ↓');download.className='research-export';download.href='/api/export/'+d.id+'?profile='+profileID+'&type=research';box.append(download);
 }
 return box;
}
function render(){
 const p=memo();if(p){$('memo-title').textContent=p.name;$('memo-preview').textContent=p.criteria;if(!memoDirty){$('fund-name').value=p.name;$('fund-memo').value=p.criteria;}}
 $('preview').hidden=!(state.preview||state.validation_fixture);if(state.validation_fixture)$('preview').textContent='Validation fixtures · news and model responses are synthetic.';
 const docs=[...state.documents].sort((a,b)=>{const x=scored(a)?a.classifications[profileID].opportunity_score:-1,y=scored(b)?b.classifications[profileID].opportunity_score:-1;return y-x||b.created.localeCompare(a.created);});
 const completed=docs.filter(d=>scored(d)&&(d.classifications[profileID].memo_reason||d.classifications[profileID].reason_error)).length;
 $('progress').textContent=state.scan.status==='running'?'Fetching and ranking news…':completed<docs.length?'Ranking '+completed+' of '+docs.length+' stories against your memo…':docs.length+' stories · highest research priority first';
 $('fetch').disabled=state.preview||state.scan.status==='running';$('memo-form').querySelector('button').disabled=state.preview;
 $('empty').hidden=docs.length>0;
 const key=JSON.stringify([docs,active,detail,p?.version]);if(key===renderKey)return;renderKey=key;$('stories').replaceChildren();
 for(const d of docs){
  const c=d.classifications[profileID],ready=scored(d),row=el('article',undefined,'story');row.dataset.id=d.id;
  const top=el('div',undefined,'story-top'),score=el('div',undefined,'opportunity '+(ready?band(c.opportunity_score):''));score.append(el('strong',ready?String(Math.round(c.opportunity_score)):'—'),el('span','/ 100'));score.setAttribute('aria-label',ready?'Opportunity score '+c.opportunity_score+' out of 100':'Score pending');
  const content=el('div'),title=action(d.summary.title||d.source.title||'Reading article…',()=>openStory(d.id),'headline');title.setAttribute('aria-expanded',String(active===d.id));content.append(title);
  let host='Source';try{host=new URL(d.url).hostname.replace(/^www\./,'');}catch{}content.append(el('div',host,'source'),el('p',d.summary.tldr||'Waiting for the article summary.','summary'));
  if(ready&&c.memo_reason)content.append(el('p',c.memo_reason,'memo-reason'));
  else if(ready)content.append(el('p',c.reason_error||'Matching the news to your memo…','status'));
  if(d.error||c?.error)content.append(el('p',d.error||c.error,'error'));
  if(d.status==='failed')content.append(action('Retry',async()=>{await api('import',{url:d.url,profile_id:profileID});await refresh();}));
  else if(d.status==='ready'&&!ready)content.append(action(c?.status==='running'?'Scoring…':'Score article',async()=>{await api('articles/'+d.id+'/classify',{profile_id:profileID});await refresh();}));
  top.append(score,content);row.append(top);if(active===d.id)row.append(renderDetail(d));$('stories').append(row);
 }
}
async function refresh(){state=await api('state');if(active)detail=await api('articles/'+active);render();}
$('memo-form').oninput=()=>{memoDirty=true;};
$('memo-form').onsubmit=async e=>{e.preventDefault();try{await api('profiles',{id:profileID,name:$('fund-name').value,criteria:$('fund-memo').value});memoDirty=false;$('memo').open=false;notice('Memo saved. Re-ranking your news…');await refresh();}catch(err){notice(err.message);}};
$('fetch').onclick=async()=>{try{notice('');await api('scan',{feed_id:'techmeme',profile_id:profileID});await api('rescore',{profile_id:profileID});await refresh();}catch(err){notice(err.message);}};
$('import').onsubmit=async e=>{e.preventDefault();try{const out=await api('import',{url:$('url').value,profile_id:profileID});$('url').value='';active=out.id;await refresh();}catch(err){notice(err.message);}};
async function tick(){if(busy)return;busy=true;try{await refresh();}catch{notice('Connection interrupted. Retrying…');}finally{busy=false;}}
tick();setInterval(tick,2500);
