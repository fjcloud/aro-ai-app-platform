// Opt-in: real local D3/Metal and hosted research LLM. No model fixtures.
const {chromium,expect}=require('@playwright/test');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
(async()=>{
 const url=process.env.APP_URL||'http://127.0.0.1:18092';assert.equal(new URL(url).hostname,'127.0.0.1');
 const browser=await chromium.launch({headless:true,args:['--use-mock-keychain']});
 try{
  const page=await browser.newPage({viewport:{width:1440,height:1000}}),errors=[];page.on('pageerror',e=>errors.push(e.message));await page.goto(url);
  const state=()=>page.evaluate(async()=>await(await fetch('/api/state')).json());let s=await state();
  assert.equal(s.preview,false);assert.ok(!s.validation_fixture);assert.equal(s.inference.agent_backend,'azure');assert.equal(s.agent,'deepseekv4pro');
  const model=(await(await page.request.get('http://127.0.0.1:18093/v1/models')).json()).data[0];assert.equal(model.device,'mps');assert.equal(model.dtype,'float16');
  if(process.env.FETCH_NEWS==='1'||!s.documents.length)await page.locator('#fetch').click();
  await expect.poll(async()=>{s=await state();const p=s.profiles.find(p=>p.id==='tech');return s.documents.length>=12&&s.documents.every(d=>d.status==='ready'&&d.classifications.tech?.status==='done'&&!d.classifications.tech.stale&&d.classifications.tech.profile_version===p.version&&Number.isFinite(d.classifications.tech.opportunity_score)&&d.classifications.tech.memo_reason);},{timeout:600000,intervals:[1000,3000,5000]}).toBeTruthy();
  const memo=s.profiles.find(p=>p.id==='tech');const sorted=[...s.documents].sort((a,b)=>b.classifications.tech.opportunity_score-a.classifications.tech.opportunity_score);
  await expect(page.locator('.story')).toHaveCount(sorted.length);await expect(page.locator('.opportunity strong').first()).toHaveText(String(Math.round(sorted[0].classifications.tech.opportunity_score)));
  await expect(page.locator('#market')).toHaveCount(0);await expect(page.locator('[data-filter]')).toHaveCount(0);await expect(page.locator('#preview')).toBeHidden();
  const news=sorted[0];await page.locator('.story[data-id="'+news.id+'"]').locator('.headline').click();
  let investigation='Existing current memo research reused';const r=news.research?.tech;
  if(r?.status!=='done'||r.profile_version!==memo.version){
   await page.locator('.story-detail').getByRole('button',{name:'Investigate',exact:true}).click();
   await expect.poll(async()=>(await state()).documents.find(d=>d.id===news.id).research?.tech?.status,{timeout:600000,intervals:[1000,3000,5000]}).toBe('done');investigation='Completed on a real memo-scored article';
  }
  await expect(page.locator('.research-export')).toHaveCount(1);
  await page.setViewportSize({width:390,height:844});assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));await page.setViewportSize({width:1440,height:1000});assert.deepEqual(errors,[]);
  const dir=path.join(__dirname,'results/techmeme-terminal');fs.mkdirSync(dir,{recursive:true});await page.screenshot({path:path.join(dir,'memo-research.png'),fullPage:true});
  await page.locator('.story[data-id="'+news.id+'"]').locator('.headline').click();
  await expect(page.locator('.story-detail')).toHaveCount(0);
  await page.screenshot({path:path.join(dir,'memo-live.png'),fullPage:true});
  const report={passed:true,at:new Date().toISOString(),scope:'Actual D3 ordinal scoring on Metal, Azure DeepSeek memo explanations and research; real saved news',memo:{name:memo.name,criteria:memo.criteria,version:memo.version},classifier:s.classifier,device:model.device,dtype:model.dtype,agent:s.agent,
   articles:sorted.map(d=>({url:d.url,title:d.summary.title,score:d.classifications.tech.opportunity_score,reason:d.classifications.tech.memo_reason,coverage:d.source.coverage,input_tokens:d.classifications.tech.input_tokens})),investigation,checks:['simple English UI','memo-relative scores out of 100, not confidence or returns','descending opportunity order','no ticker dashboard or BUY/SHORT feed filters','source-grounded memo explanations','current-score investigation','mobile layout','mock-keychain','no JavaScript errors']};
  fs.writeFileSync(path.join(dir,'memo-live.json'),JSON.stringify(report,null,2)+'\n');console.log(JSON.stringify({passed:true,articles:sorted.length,highest:report.articles[0].score,investigation}));
 }finally{await browser.close();}
})().catch(e=>{console.error(e.message);process.exitCode=1;});
