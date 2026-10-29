// Opt-in: real ARO T4, native OpenShift AI Responses, archives and prices.
const {chromium,expect}=require('@playwright/test');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
(async()=>{
 const url=process.env.APP_URL;assert.ok(url,'APP_URL is required');
 const browser=await chromium.launch({headless:true,args:['--use-mock-keychain']});
 try{
  const context=await browser.newContext({viewport:{width:1440,height:1000},...(process.env.BROWSER_STATE?{storageState:process.env.BROWSER_STATE}:{})});const page=await context.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));await page.goto(url);
  const state=()=>page.evaluate(async()=>await(await fetch('/api/game')).json());let s=await state();assert.ok(!s.preview&&!s.validation_fixture);await expect(page.locator('#news-file')).toBeHidden();
  const model=JSON.parse(fs.readFileSync(process.env.D3_METADATA,'utf8')).data[0];assert.equal(model.device,'cuda:0');assert.equal(model.dtype,'float16');const config=await page.evaluate(async()=>await(await fetch('/api/state')).json());assert.equal(config.agent,'maas-vllm-inference-1/deepseekv4pro');
  if(s.game.round.status!=='draft'){await page.locator('#reset').click();await expect.poll(async()=>(await state()).game.round.status).toBe('draft');}
  let imported=Boolean(s.game.manifest.imported_at),collectionError='';
  if(!s.total){const started=page.waitForResponse(r=>r.url().endsWith('/api/game/load')&&r.request().method()==='POST');await page.locator('#fetch').click();assert.equal((await started).status(),202);}
  await expect.poll(async()=>{s=await state();return ['ready','failed'].includes(s.game.status);},{timeout:1800000,intervals:[3000,10000]}).toBe(true);
  if(s.game.status==='failed'){
   collectionError=s.game.error;
   assert.ok(process.env.NEWS_IMPORT_FILE,'Live collection failed: '+collectionError+'; provide NEWS_IMPORT_FILE to test manual recovery');
   const upload=page.waitForResponse(r=>r.url().endsWith('/api/game/import')&&r.request().method()==='POST');await page.locator('#news-file').setInputFiles(process.env.NEWS_IMPORT_FILE);assert.equal((await upload).status(),202);imported=true;
  }
  await expect.poll(async()=>{s=await state();return ['ready','failed'].includes(s.game.status);},{timeout:1800000,intervals:[3000,10000]}).toBe(true);
  let rankingRetries=0;
  if(s.game.status==='failed'&&process.env.NEWS_IMPORT_FILE&&s.total>0&&s.ranked<s.total){
   const retry=page.waitForResponse(r=>r.url().endsWith('/api/game/import')&&r.request().method()==='POST');await page.locator('#news-file').setInputFiles(process.env.NEWS_IMPORT_FILE);assert.equal((await retry).status(),202);rankingRetries++;
   await expect.poll(async()=>{s=await state();return s.game.status==='ready'&&s.ranked===s.total;},{timeout:1800000,intervals:[3000,10000]}).toBe(true);
  }
  assert.equal(s.game.status,'ready',s.game.error);assert.ok(s.total>100&&s.ranked===s.total);
  if(process.env.EXPECT_UPDATED_APP==='true'){
   const box=page.getByRole('checkbox',{name:'High relevance only',exact:true});await expect(box).toBeVisible();await box.check();
   const count=s.documents.filter(d=>d.classifications.tech.opportunity_score>=70).length;
   await expect(page.locator('.story')).toHaveCount(Math.min(count,30));
   assert.ok((await page.locator('.opportunity strong').allTextContents()).every(x=>Number(x)>=70));
   await box.uncheck();await expect(page.locator('.story')).toHaveCount(30);
  }
  const d=s.documents.find(d=>s.game.proposals[d.id]?.status==='done')||s.documents[0];
  const row=page.locator('.story[data-id="'+d.id+'"]');await row.locator('.headline').click();
  if(!s.game.proposals[d.id]||s.game.proposals[d.id].status!=='done'){await row.getByRole('button',{name:'Find index ideas',exact:true}).click();await expect.poll(async()=>(await state()).game.proposals[d.id]?.status,{timeout:600000,intervals:[1000,3000]}).toBe('done');}
  await expect(row.locator('.idea')).not.toHaveCount(0);await row.locator('.stake-input').fill('1000');await row.getByRole('button',{name:'Add to portfolio',exact:true}).click();await expect(page.locator('#basket .basket-row')).toHaveCount(1);
  s=await state();assert.equal(s.game.round.status,'draft');assert.equal(s.game.round.positions.length,0);assert.equal(s.game.round.pnl,0);
  const corpus=await(await page.request.get(url+'/api/game/export')).json();assert.equal(corpus.news.length,s.total);assert.equal(corpus.week.news_start,s.game.week.news_start);
  await page.locator('#reveal').click();await expect.poll(async()=>(await state()).game.round.status,{timeout:60000,intervals:[1000,3000]}).toBe('settled');s=await state();
  const r=s.game.round,p=r.positions[0],v=p.result;assert.equal(r.invested,1000);assert.ok(v.entry_date>=s.game.week.market_start&&v.exit_date<=s.game.week.market_end);assert.ok(Math.abs(r.pnl-1000*(v.exit_price/v.entry_price-1))<.000001);assert.ok(Math.abs(r.value-(10000+r.pnl))<.000001);assert.ok(v.source_url.startsWith('https://query1.finance.yahoo.com/'));
  const evidence=s.game.proposals[d.id].evidence;assert.deepEqual([...new Set(evidence.map(e=>e.tool))].sort(),['index_universe','read_archive','search_archive']);
  await expect(page.locator('#round-note')).toContainText(r.week_complete?'Week completed':'Week in progress');await expect(page.locator('#reveal')).toBeHidden();
  await page.setViewportSize({width:390,height:844});assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'Mobile overflow');await page.setViewportSize({width:1440,height:1000});assert.deepEqual(errors,[]);
  const dir=path.join(__dirname,'results/signal-desk-aro');fs.mkdirSync(dir,{recursive:true});await page.screenshot({path:path.join(dir,'replay.png'),fullPage:true});
  const report={passed:true,app_url:url,source_revision:process.env.APPLICATION_REVISION||null,agent:config.agent,manual_import:imported,ranking_retries:rankingRetries,live_collection_error:collectionError,at:new Date().toISOString(),live_models:true,news:s.total,ranked:s.ranked,week:s.game.week,unavailable_ids:corpus.unavailable_ids,device:model.device,dtype:model.dtype,article:{title:d.summary.title,score:d.classifications.tech.opportunity_score},research:s.game.proposals[d.id],round:r,checks:['real archive corpus and complete export','actual memo scores on the ARO T4','bounded archived-week LLM research','portfolio frozen before market data','actual historical entry and completed-session close','verified portfolio arithmetic','partial week disclosed','mobile layout','mock-keychain','no JavaScript errors']};fs.writeFileSync(path.join(dir,'replay.json'),JSON.stringify(report,null,2)+'\n');
  await page.locator('#reset').click();await expect.poll(async()=>(await state()).game.round.status).toBe('draft');console.log(JSON.stringify({passed:true,news:s.total,ranked:s.ranked,pnl:r.pnl,entry:v.entry_date,exit:v.exit_date,week_complete:r.week_complete}));
 }finally{await browser.close();}
})().catch(e=>{console.error(e.stack);process.exitCode=1;});
