// Component test only. These recent REST posts are NEVER sent to the live feed.
import fs from 'node:fs';
import {classifyPost} from '../providers.mjs';
import {postFromEvent} from '../engine.mjs';
import {ResearchAgent} from '../agent.mjs';
fs.mkdirSync(new URL('../evidence/components/',import.meta.url),{recursive:true});
const sources=JSON.parse(fs.readFileSync(new URL('../sources.json',import.meta.url)));
const source=sources.find(s=>s.handle==='euronews.com');
const url='https://public.api.bsky.app/xrpc/app.bsky.feed.getAuthorFeed?actor=euronews.com&limit=80&filter=posts_no_replies';
const data=await(await fetch(url)).json(),scanned=[];
const agent=new ResearchAgent({base:process.env.LLM_BASE_URL,key:process.env.LLM_API_KEY,model:'deepseekv4pro'});
for(const v of data.feed||[]){if(scanned.length>=80)break;const p=v.post;if(p.author.did!==source.did)continue;const post=postFromEvent({kind:'commit',did:source.did,commit:{operation:'create',collection:'app.bsky.feed.post',rkey:p.uri.split('/').at(-1),cid:p.cid,record:p.record}},source,'component-test-recent-rest');if(!post)continue;
 const classification=await classifyPost(post,'http://127.0.0.1:18123/v1/systemone',new AbortController().signal);scanned.push({text:post.text,post_url:post.post_url,published_at:post.published_at,classification});
 console.log(JSON.stringify({stage:'classify',scanned:scanned.length,choice:classification.choice,text:post.text.slice(0,130)}));
 if(classification.choice!=='positive'||classification.probabilities.positive<.65)continue;
 try{const result=await agent.investigate(post,AbortSignal.timeout(180000),t=>console.log(JSON.stringify({stage:t.type,tool:t.name,ms:t.http_ms,error:t.output?.error})));fs.writeFileSync(new URL('../evidence/components/component-agent.json',import.meta.url),JSON.stringify({kind:'Component test on a recent real general-news post. NOT a WebSocket event and NOT published in live feed.',post,classification,result,scanned},null,2)+'\n');console.log(JSON.stringify({component_result:result.summary?'source-supported summary':'held',reason:result.reason,summary:result.summary}));if(result.summary)break;}catch(e){console.log('Component investigation:',e.message);fs.writeFileSync(new URL('../evidence/components/component-agent-error.json',import.meta.url),JSON.stringify({post,classification,error:e.message,scanned},null,2)+'\n')}

}
fs.writeFileSync(new URL('../evidence/components/component-classification.json',import.meta.url),JSON.stringify({kind:'Isolated component test; recent REST posts from a general-news publisher; never injected into live feed',scanned},null,2)+'\n');
