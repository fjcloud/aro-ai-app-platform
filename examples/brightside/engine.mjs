import {EventEmitter} from 'node:events';
import fs from 'node:fs';
import path from 'node:path';
import {canonicalURL,classifyPost} from './providers.mjs';

export function postFromEvent(event,source,origin='live'){
 const c=event.commit,r=c?.record;if(event.kind!=='commit'||c.collection!=='app.bsky.feed.post'||!['create','update'].includes(c.operation)||!r||r.reply||typeof r.text!=='string'||!r.text.trim()||!source)return null;
 if(!/^[a-zA-Z0-9._~:-]{1,512}$/.test(c.rkey))return null;
 const date=Date.parse(r.createdAt);if(!Number.isFinite(date)||date>Date.now()+300000||date<Date.now()-48*3600000)return null;
 const external=r.embed?.external||r.embed?.media?.external;
 const raw=[external?.uri,...(r.facets||[]).flatMap(f=>(f.features||[]).filter(v=>v.$type==='app.bsky.richtext.facet#link').map(v=>v.uri))];
 const urls=[...new Set(raw.map(canonicalURL).filter(Boolean))];
 return {id:`at://${event.did}/app.bsky.feed.post/${c.rkey}`,cid:c.cid||JSON.stringify(r),did:event.did,source:source.name,handle:source.handle,text:r.text.slice(0,4000),headline:external?.title?.slice(0,400)||'',urls,published_at:new Date(date).toISOString(),received_at:new Date().toISOString(),ingestion_ms:Math.max(0,Date.now()-date),post_url:`https://bsky.app/profile/${source.handle}/post/${c.rkey}`,origin,trace:[],status:'queued'};
}

export class NewsEngine extends EventEmitter{
 constructor({sources,agent,d3URL,stateFile,jetstream='wss://jetstream2.us-east.bsky.network/subscribe',classify=classifyPost,socketFactory=u=>new WebSocket(u)}){
  super();Object.assign(this,{sources,agent,d3URL,stateFile,jetstream,classify,socketFactory});this.byDID=new Map(sources.map(s=>[s.did,s]));this.records=new Map();this.running=false;this.controller=new AbortController();this.classifying=false;this.researching=0;this.socket=null;this.reconnect=null;this.generation=0;this.connection='paused';this.lastError='';this.cursor=null;this.revision=0;this.stats={received:0,classified:0,positive:0,rejected:0,published:0,held:0,errors:0,dropped:0,reconnects:0};this.startedAt=new Date().toISOString();
  if(stateFile&&fs.existsSync(stateFile)){try{const saved=JSON.parse(fs.readFileSync(stateFile));this.stats={...this.stats,...saved.stats};this.cursor=saved.cursor;for(const r of saved.records||[]){if(r.ingestion_ms===undefined)r.ingestion_ms=Math.max(0,Date.parse(r.received_at)-Date.parse(r.published_at));if(['classifying','researching'].includes(r.status))r.status=r.classification?.choice==='positive'?'positive':'queued';this.records.set(r.id,r)}}catch{this.lastError='Saved state could not be restored'}}
 }
 save(){if(!this.stateFile)return;fs.mkdirSync(path.dirname(this.stateFile),{recursive:true});const temp=this.stateFile+'.tmp';fs.writeFileSync(temp,JSON.stringify({stats:this.stats,cursor:this.cursor,records:[...this.records.values()]}),{mode:0o600});fs.renameSync(temp,this.stateFile)}
 changed(){this.revision++;this.emit('state',this.snapshot());clearTimeout(this.saveTimer);this.saveTimer=setTimeout(()=>this.save(),300);this.saveTimer.unref()}
 snapshot(){const all=[...this.records.values()];return {revision:this.revision,persona:{name:'Nora',description:'Only genuinely positive world news. Short summaries, checked sources.'},running:this.running,connection:this.connection,started_at:this.startedAt,last_event_at:this.lastEventAt||null,last_error:this.lastError,stats:{...this.stats,pending:all.filter(r=>['queued','classifying','positive','researching'].includes(r.status)).length,held:all.filter(r=>r.status==='held').length,researching:this.researching},sources:this.sources,filtered_items:all.filter(r=>r.status==='rejected'&&!r.deleted&&r.classification).sort((a,b)=>b.received_at.localeCompare(a.received_at)).slice(0,100).map(r=>({id:r.id,cid:r.cid,source:r.source,handle:r.handle,text:r.text,headline:r.headline,urls:r.urls,published_at:r.published_at,received_at:r.received_at,post_url:r.post_url,classification:{choice:r.classification.choice,probabilities:r.classification.probabilities,http_ms:r.classification.http_ms,model:r.classification.model},filter_reason:r.classification.choice==='positive'?'Positive score below the 65% publication threshold':'Classified as '+r.classification.choice})),items:all.filter(r=>r.status==='published'&&!r.deleted).sort((a,b)=>b.received_at.localeCompare(a.received_at)).slice(0,100).map(({trace,...r})=>r)}}
 ingest(event,origin='live'){
  const source=this.byDID.get(event.did);if(!source)return false;
  if(event.cursor!==undefined)this.cursor=event.cursor;else if(event.time_us)this.cursor=event.time_us;
  this.lastEventAt=new Date().toISOString();
  if(event.kind==='account'&&event.active===false){for(const r of this.records.values())if(r.did===event.did){r.deleted=true;r.status='deleted'}this.changed();return false}
  if(event.kind!=='commit'||event.commit?.collection!=='app.bsky.feed.post')return false;
  const id=`at://${event.did}/app.bsky.feed.post/${event.commit.rkey}`;
  if(event.commit.operation==='delete'){const r=this.records.get(id);if(r){r.deleted=true;r.status='deleted';this.changed()}return false}
  const post=postFromEvent(event,source,origin);if(!post)return false;
  const existing=this.records.get(post.id);if(existing?.cid===post.cid)return false;
  if([...this.records.values()].filter(r=>['queued','classifying','positive','researching'].includes(r.status)).length>=120){this.stats.dropped++;this.changed();return false}
  // An updated version invalidates a previously published summary immediately.
  if(existing)existing.deleted=true;
  this.records.set(post.id,post);this.stats.received++;
  for(const [key,r]of this.records){if(this.records.size<=500)break;if(!['queued','classifying','positive','researching'].includes(r.status))this.records.delete(key)}
  this.changed();this.pump();return true;
 }
 connect(){
  if(!this.running)return;const generation=this.generation;
  const url=new URL(this.jetstream);url.searchParams.set('wantedCollections','app.bsky.feed.post');for(const did of this.byDID.keys())url.searchParams.append('wantedDids',did);if(this.cursor!==null)url.searchParams.set('cursor',String(this.cursor));
  this.connection='connecting';this.changed();
  let socket;try{socket=this.socketFactory(url.href);this.socket=socket}catch{this.scheduleReconnect(generation);return}
  socket.onopen=()=>{if(generation!==this.generation)return;this.connection='live';this.changed()};
  socket.onmessage=e=>{if(!this.running||generation!==this.generation||typeof e.data!=='string'||e.data.length>2*1024*1024)return;try{this.ingest(JSON.parse(e.data))}catch{this.lastError='An unreadable stream event was ignored';this.changed()}};
  socket.onerror=()=>{if(generation===this.generation){this.connection='reconnecting';this.changed()}};
  socket.onclose=()=>{if(generation===this.generation&&this.running)this.scheduleReconnect(generation)};
 }
 scheduleReconnect(generation){if(!this.running||generation!==this.generation)return;this.connection='reconnecting';this.stats.reconnects++;this.changed();clearTimeout(this.reconnect);this.reconnect=setTimeout(()=>this.connect(),3000)}
 start(){if(this.running)return;this.running=true;this.controller=new AbortController();this.generation++;this.connect();this.retryTimer=setInterval(()=>this.retryDue(),2000);this.retryTimer.unref();this.pump();this.changed()}
 pause(){if(!this.running)return;this.running=false;this.generation++;this.controller.abort();clearTimeout(this.reconnect);clearInterval(this.retryTimer);this.socket?.close();this.socket=null;this.connection='paused';this.changed();this.save()}
 retryDue(){if(!this.running)return;let changed=false;for(const post of this.records.values()){if(post.status==='classifier_error'&&post.classifier_retry_at&&Date.parse(post.classifier_retry_at)<=Date.now()&&!post.deleted){post.status='queued';delete post.classifier_retry_at;post.trace.push({type:'classifier_retry',at:new Date().toISOString(),reason:'Retry after a temporary decision-model connection failure.'});changed=true}if(post.status==='held'&&post.retry_at&&Date.parse(post.retry_at)<=Date.now()&&!post.deleted){post.status='positive';delete post.retry_at;post.trace.push({type:'retry',at:new Date().toISOString(),reason:'Recheck previously unconfirmed live news after allowing corroboration to appear.'});changed=true}}if(changed){this.changed();this.pump()}}
 scheduleRetry(post){if((post.attempts||0)<3)post.retry_at=new Date(Date.now()+(post.attempts===1?60000:180000)).toISOString()}
 async pump(){
  if(!this.running)return;
  if(!this.classifying){const post=[...this.records.values()].find(r=>r.status==='queued'&&!r.deleted);if(post){this.classifying=true;post.status='classifying';const signal=this.controller.signal,generation=this.generation;post.classification_attempts=(post.classification_attempts||0)+1;this.changed();(async()=>{
   try{
    const result=await this.classify(post,this.d3URL,signal);
    if(post.deleted)return;
    if(generation!==this.generation){post.status='queued';return}
    this.lastError='';delete post.error;delete post.classifier_retry_at;post.classification=result;post.trace.push({type:'classification',at:new Date().toISOString(),...result});this.stats.classified++;
    if(result.choice==='positive'&&result.probabilities.positive>=.65){post.status='positive';this.stats.positive++}else{post.status='rejected';this.stats.rejected++}
   }catch(e){if(!post.deleted){if(signal.aborted)post.status='queued';else{post.status='classifier_error';post.error=e.message;this.stats.errors++;this.lastError=e.message;if(post.classification_attempts<3)post.classifier_retry_at=new Date(Date.now()+10000*post.classification_attempts).toISOString()}}}
   finally{this.classifying=false;this.changed();this.pump()}
  })()}}
  while(this.researching<2){const post=[...this.records.values()].find(r=>r.status==='positive'&&!r.deleted);if(!post)break;
   // Repeated posts linking the exact same article do not trigger another paid investigation.
   const duplicate=post.urls[0]&&[...this.records.values()].find(r=>r!==post&&['researching','published'].includes(r.status)&&r.urls[0]===post.urls[0]);
   if(duplicate){post.status='duplicate';this.changed();continue}
   if(!post.urls.length){post.status='held';post.hold_reason='No linked article to check';this.stats.held++;this.changed();continue}
   this.researching++;post.attempts=(post.attempts||0)+1;post.status='researching';const signal=this.controller.signal,generation=this.generation;this.changed();(async()=>{
    try{const result=await this.agent.investigate(post,signal,t=>{post.trace.push(t);this.emit('trace',{id:post.id,event:t})});
     if(post.deleted)return;
     if(generation!==this.generation){post.status='positive';return}
     post.research_ms=result.http_ms;
     if(result.summary){post.summary=result.summary;post.status='published';post.published_to_feed_at=new Date().toISOString();post.processing_ms=Date.now()-Date.parse(post.received_at);this.stats.published++}else{post.status='held';post.hold_reason=result.reason;this.stats.held++;if(result.retryable)this.scheduleRetry(post)}
    }catch(e){if(!post.deleted){if(signal.aborted)post.status='positive';else{post.status='held';post.hold_reason=e.message;this.stats.held++;this.stats.errors++;this.lastError=e.message;this.scheduleRetry(post)}}}
    finally{this.researching--;this.changed();this.pump()}
   })();
  }
 }
 close(){this.pause();clearTimeout(this.saveTimer);this.save()}
}
