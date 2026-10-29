// Opt-in: real Techmeme archives, D3/Metal, Azure research and market prices.
const {chromium,expect}=require('@playwright/test');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
(async()=>{
 const url=process.env.APP_URL||'http://127.0.0.1:18092';assert.equal(new URL(url).hostname,'127.0.0.1');
 const browser=await chromium.launch({headless:true,args:['--use-mock-keychain']});
 try{
  const page=await browser.newPage({viewport:{width:1440,height:1000}}),errors=[];page.on('pageerror',e=>errors.push(e.message));await page.goto(url);
  const state=()=>page.evaluate(async()=>await(await fetch('/api/game')).json());let s=await state();assert.ok(!s.preview&&!s.validation_fixture);
  const model=(await(await page.request.get('http://127.0.0.1:18093/v1/models')).json()).data[0];assert.equal(model.device,'mps');assert.equal(model.dtype,'float16');
  if(s.game.round.status!=='draft'){await page.locator('#reset').click();await expect.poll(async()=>(await state()).game.round.status).toBe('draft');}
  if(!s.total)await page.locator('#fetch').click();
  await expect.poll(async()=>{s=await state();return s.game.status==='ready'&&s.total>100&&s.total===s.ranked;},{timeout:1800000,intervals:[3000,10000]}).toBe(true);
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
  const dir=path.join(__dirname,'results/weekly-game');fs.mkdirSync(dir,{recursive:true});await page.screenshot({path:path.join(dir,'live.png'),fullPage:true});
  const report={passed:true,at:new Date().toISOString(),live_models:true,news:s.total,ranked:s.ranked,week:s.game.week,unavailable_ids:corpus.unavailable_ids,device:model.device,dtype:model.dtype,article:{title:d.summary.title,score:d.classifications.tech.opportunity_score},research:s.game.proposals[d.id],round:r,checks:['real archive corpus and complete export','actual memo scores on Metal','bounded archived-week LLM research','portfolio frozen before market data','actual historical entry and completed-session close','verified portfolio arithmetic','partial week disclosed','mobile layout','mock-keychain','no JavaScript errors']};fs.writeFileSync(path.join(dir,'live.json'),JSON.stringify(report,null,2)+'\n');
  await page.locator('#reset').click();await expect.poll(async()=>(await state()).game.round.status).toBe('draft');console.log(JSON.stringify({passed:true,news:s.total,ranked:s.ranked,pnl:r.pnl,entry:v.entry_date,exit:v.exit_date,week_complete:r.week_complete}));
 }finally{await browser.close();}
})().catch(e=>{console.error(e.stack);process.exitCode=1;});
