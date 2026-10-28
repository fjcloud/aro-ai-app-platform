'use strict';
const $=id=>document.getElementById(id),profileID='tech';
let data=null,active='',dirty=false,busy=false,key='',limit=30,draft=[],draftKey='',amounts=new Map();
function el(tag,text,cls){const n=document.createElement(tag);if(text!==undefined)n.textContent=text;if(cls)n.className=cls;return n;}
const money=n=>new Intl.NumberFormat('en-US',{style:'currency',currency:'USD',maximumFractionDigits:2}).format(n);
const signed=n=>(n>=0?'+':'−')+money(Math.abs(n));
function notice(s){$('notice').hidden=!s;$('notice').textContent=s;}
async function api(path,body){const r=await fetch('/api/'+path,body?{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)}:{});const value=await r.json();if(!r.ok)throw Error(value.error||'Request failed');return value;}
function action(text,fn,cls){const b=el('button',text,cls);b.onclick=async()=>{b.disabled=true;try{notice('');await fn();}catch(e){notice(e.message);}finally{b.disabled=false;}};return b;}
function link(text,url){const a=el('a',text);try{const u=new URL(url);if(['https:','http:'].includes(u.protocol)){a.href=u.href;a.target='_blank';a.rel='noopener noreferrer';}}catch{}return a;}
function scored(d){const c=d.classifications[profileID];return c?.status==='done'&&Number.isFinite(c.opportunity_score)&&c.profile_version===data.memo.version&&!c.stale;}
function saveDraft(){try{localStorage.setItem(draftKey,JSON.stringify(draft));}catch{}}
function loadDraft(){const id=data.game.week.id+':'+data.game.round.id+':'+data.memo.version;if(id===draftKey)return;draftKey=id;try{draft=JSON.parse(localStorage.getItem(id)||'[]');if(!Array.isArray(draft))draft=[];}catch{draft=[];}}
function basketView(){
 const round=data.game.round,total=draft.reduce((n,p)=>n+p.amount,0);$('basket').replaceChildren();$('results').replaceChildren();
 $('wallet').textContent=money(round.status==='settled'?round.value:round.budget);$('profit').textContent=round.status==='settled'?signed(round.pnl)+' ('+(round.return_pct>=0?'+':'')+round.return_pct.toFixed(2)+'%)':'';$('profit').className=round.pnl<0?'loss':'gain';
 $('reveal').hidden=round.status!=='draft';$('reveal').disabled=data.preview||draft.length===0||total>round.budget;$('update').hidden=!['settled','awaiting_prices'].includes(round.status);$('reset').hidden=['draft','settling'].includes(round.status);
 $('round-note').textContent=round.status==='draft'?money(round.budget-total)+' available · choose your investments before revealing any result.':round.status==='settling'?'Portfolio frozen. Fetching observed market prices…':round.status==='awaiting_prices'?round.error:round.week_complete?'Week completed · results through '+round.as_of+' close.':'Week in progress · results through '+round.as_of+' close.';
 const positions=round.status==='draft'?draft:round.positions;
 for(const p of positions){const row=el('div',undefined,'basket-row'),info=el('div');info.append(el('strong',p.symbol+' · '+money(p.amount)),el('p',p.title,'status'));row.append(info);
  if(round.status==='draft')row.append(action('Remove',()=>{draft=draft.filter(x=>x.article_id!==p.article_id);saveDraft();key='';render();}));
  if(p.result){const v=el('div',undefined,'position-result');v.append(el('strong',signed(p.amount*p.result.return_pct/100),p.result.return_pct>=0?'gain':'loss'),el('p',p.result.entry_date+' open '+money(p.result.entry_price)+' → '+p.result.exit_date+' close '+money(p.result.exit_price),'status'),link('Price data ↗',p.result.source_url));row.append(v);}
  $('basket').append(row);
 }
}
function storyDetail(d){
 const box=el('section',undefined,'story-detail');box.append(el('p',d.summary.tldr),link('Read the archived snapshot ↗',d.source.archive_url),el('p','Known by '+d.source.archived_at.slice(0,10)+' · archived Techmeme excerpt, not a current publisher page.','status'));
 let p=data.game.proposals[d.id];if(p&&(p.memo_version!==data.memo.version||p.summary_hash!==d.classifications.tech?.summary_hash))p=null;
 if(scored(d)){const b=action(p?.status==='running'?'Researching…':p?.status==='done'?'Research again':'Find index ideas',async()=>{await api('game/ideas',{article_id:d.id});await refresh();},'primary');b.disabled=data.preview||p?.status==='running';box.append(el('div',undefined,'actions'));box.lastChild.append(b);}
 if(p?.status==='running')box.append(el('p','The agent is comparing archived news and index exposures. It cannot see this week’s prices.','status'));
 if(p?.error)box.append(el('p',p.error,'error'));
 if(p?.status==='done'){
  box.append(el('h3','Possible index investments'),el('p',p.comment));
  if(!p.ideas.length)box.append(el('p','No supported index idea. Try another news story.','status'));
  const form=el('form');form.className='investment-form';
  for(let i=0;i<p.ideas.length;i++){const idea=p.ideas[i],label=el('label',undefined,'idea');const radio=el('input');radio.type='radio';radio.name='idea';radio.value=idea.id;radio.checked=i===0;label.append(radio,el('strong',idea.symbol),el('p',idea.thesis),el('p','Risk: '+idea.risk,'status'));form.append(label);}
  if(p.ideas.length&&data.game.round.status==='draft'){
   const amount=el('input');amount.type='number';amount.min=100;amount.max=10000;amount.step=100;amount.value=amounts.get(d.id)||1000;amount.className='stake-input';amount.setAttribute('aria-label','Paper investment amount');amount.oninput=()=>amounts.set(d.id,amount.value);
   const controls=el('div',undefined,'actions');controls.append(el('label','Investment / USD'),amount);const add=el('button',draft.some(p=>p.article_id===d.id)?'Update investment':'Add to portfolio','primary');add.type='submit';controls.append(add);form.append(controls);
   form.onsubmit=e=>{e.preventDefault();try{const idea=p.ideas.find(v=>v.id===form.querySelector('input[name=idea]:checked')?.value),value=Number(amount.value);if(!idea||!Number.isFinite(value)||value<100)throw Error('Choose an index and an amount of at least $100.');const other=draft.filter(v=>v.article_id!==d.id);if(other.reduce((n,p)=>n+p.amount,0)+value>data.game.round.budget)throw Error('This exceeds the $10,000 paper budget.');draft=[...other,{article_id:d.id,idea_id:idea.id,amount:value,symbol:idea.symbol,title:d.summary.title}];saveDraft();key='';render();notice('Investment added. Review your portfolio above when ready.');}catch(err){notice(err.message);}};
  }
  box.append(form);
  if(p.trace_id)box.append(el('p','MLflow trace: '+p.trace_id,'status'));
 }
 return box;
}
function render(){
 const game=data.game,p=data.memo;loadDraft();$('preview').hidden=!(data.preview||data.validation_fixture);
 $('periods').textContent='News: '+game.week.news_start+' to '+game.week.news_end+' · paper entry: first US session from '+game.week.market_start+'.';
 $('memo-title').textContent=p.name;$('memo-preview').textContent=p.criteria;if(!dirty){$('fund-name').value=p.name;$('fund-memo').value=p.criteria;}
 $('memo-form').querySelector('button').disabled=data.preview||game.round.status!=='draft';
 $('fetch').disabled=data.preview||['loading','ranking'].includes(game.status);$('import-news').disabled=data.preview||game.round.status!=='draft'||['loading','ranking'].includes(game.status);$('fetch').textContent=game.status==='loading'?'Loading archives…':game.status==='ranking'?'Ranking the week…':data.total?'Reload last week':'Load last week’s news';
 $('news-export').hidden=!data.total;$('news-export').href='/api/game/export';$('coverage').textContent=(game.error?.includes('HTTP 403')?'Live collection blocked (HTTP 403). Use Import news with an export for this week.':game.error)||((game.manifest.imported_at?'Manually imported archive · scores computed on the server. ':'')+((game.manifest.unavailable_ids?.length||0)>0?game.manifest.unavailable_ids.length+' archive IDs were unavailable. Export includes the coverage details.':''));
 $('progress').textContent=['loading','ranking'].includes(game.status)?game.message:data.ranked+' / '+data.total+' stories ranked for your memo · highest first';
 basketView();$('empty').hidden=data.total>0;
 if(document.activeElement?.classList.contains('stake-input'))return;
 const next=JSON.stringify([data.documents,game.proposals,active,p.version,limit,draft,game.round.status]);if(next===key)return;key=next;$('stories').replaceChildren();
 for(const d of data.documents.slice(0,limit)){
  const c=d.classifications[profileID],row=el('article',undefined,'story');row.dataset.id=d.id;const top=el('div',undefined,'story-top'),score=el('div',undefined,'opportunity '+(scored(d)&&c.opportunity_score>=70?'high':''));score.append(el('strong',scored(d)?String(Math.round(c.opportunity_score)):'—'),el('span','/ 100'));
  const content=el('div'),title=action(d.summary.title,()=>{active=active===d.id?'':d.id;key='';render();},'headline');title.setAttribute('aria-expanded',String(active===d.id));content.append(title,el('div',d.source.published+' · archived news','source'),el('p',d.summary.tldr,'summary'));if(c?.error)content.append(el('p',c.error,'error'));top.append(score,content);row.append(top);if(active===d.id)row.append(storyDetail(d));$('stories').append(row);
 }
 $('more').hidden=limit>=data.documents.length;
}
async function refresh(){data=await api('game');render();}
$('fetch').onclick=async()=>{try{await api('game/load',{});await refresh();}catch(e){notice(e.message);}};
$('memo-form').oninput=()=>dirty=true;$('memo-form').onsubmit=async e=>{e.preventDefault();try{await api('profiles',{id:profileID,name:$('fund-name').value,criteria:$('fund-memo').value});dirty=false;draft=[];$('memo').open=false;await refresh();notice('Memo saved. Re-ranking the archived news.');}catch(err){notice(err.message);}};
$('reveal').onclick=async()=>{try{await api('game/play',{round_id:data.game.round.id,memo_version:data.memo.version,positions:draft.map(p=>({article_id:p.article_id,idea_id:p.idea_id,amount:p.amount}))});await refresh();}catch(e){notice(e.message);}};
$('update').onclick=async()=>{try{await api('game/settle',{});await refresh();}catch(e){notice(e.message);}};
$('reset').onclick=async()=>{try{await api('game/reset',{});active='';await refresh();}catch(e){notice(e.message);}};
$('more').onclick=()=>{limit+=30;key='';render();};
async function tick(){if(busy)return;busy=true;try{await refresh();}catch{notice('Connection interrupted. Retrying…');}finally{busy=false;}};tick();setInterval(tick,3000);

$('import-news').onclick=()=>$('news-file').click();
$('news-file').onchange=async()=>{const file=$('news-file').files[0];if(!file)return;try{if(file.size>8*1024*1024)throw Error('Choose a JSON news export smaller than 8 MiB.');const body=JSON.parse(await file.text());await api('game/import',body);draft=[];saveDraft();key='';await refresh();notice('News imported. File scores were ignored; the decision model ranks the archived facts.');}catch(e){notice(e.message);}finally{$('news-file').value='';}};
