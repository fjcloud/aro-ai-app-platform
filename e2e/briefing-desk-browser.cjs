// Explicit archived-news, D3, LLM and historical-price fixtures.
const {chromium,expect}=require('@playwright/test');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
(async()=>{
 const browser=await chromium.launch({headless:true,args:['--use-mock-keychain']});
 try{
  const page=await browser.newPage({viewport:{width:1440,height:1000}}),errors=[];page.on('pageerror',e=>errors.push(e.message));await page.goto(process.env.APP_URL);
  const state=()=>page.evaluate(async()=>await(await fetch('/api/game')).json());let s=await state();assert.equal(s.validation_fixture,true);await expect(page.locator('#news-file')).toBeHidden();
  await page.locator('#fetch').click();await expect.poll(async()=>{s=await state();return s.game.status==='ready'&&s.total===21&&s.ranked===21;},{timeout:30000}).toBe(true);
  await expect(page.locator('.story')).toHaveCount(3);await expect(page.locator('.opportunity strong').first()).toHaveText('90');await expect(page.locator('#preview')).toBeVisible();
  // Editing the memo re-ranks the saved archive without re-importing live news.
  const original=s.memo,custom='Retail-only fixture memo. Research consumer marketplaces and merchant revenues; semiconductor headlines have low relevance.';
  await page.locator('#memo>summary').click();await page.locator('#fund-memo').fill(custom);await page.waitForTimeout(3100);await expect(page.locator('#fund-memo')).toHaveValue(custom);
  await page.locator('#memo-form button').click();await expect.poll(async()=>{const x=await state();return x.memo.criteria===custom&&x.ranked===21&&x.documents.every(d=>d.classifications.tech.opportunity_score===10);},{timeout:30000}).toBe(true);
  await page.locator('#memo>summary').click();await page.locator('#fund-memo').fill(original.criteria);await page.locator('#memo-form button').click();await expect.poll(async()=>{s=await state();return s.memo.criteria===original.criteria&&s.ranked===21&&s.documents.some(d=>d.classifications.tech.opportunity_score===90);},{timeout:30000}).toBe(true);
  // The file contributes archived facts only; forged scores are ignored.
  await page.locator('#data-tools').evaluate(e=>e.open=true);
  const uploaded=await(await page.request.get(process.env.APP_URL+'/api/game/export')).json();
  uploaded.news[0].summary.tldr+=' Additional imported source fact.';
  uploaded.news.forEach(d=>d.classifications.tech.opportunity_score=99);
  await page.locator('#news-file').setInputFiles({name:'archived-news.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify(uploaded))});
  await expect.poll(async()=>{s=await state();return s.game.status==='ready'&&s.ranked===21&&Boolean(s.game.manifest.imported_at);},{timeout:30000}).toBe(true);
  assert.ok(s.documents.every(d=>d.classifications.tech.opportunity_score!==99));
  await expect(page.locator('#coverage')).toContainText('Manually imported archive');
  const choices=s.documents.filter(d=>d.classifications.tech.opportunity_score===90).slice(0,2);
  for(let i=0;i<choices.length;i++){
   const d=choices[i],row=page.locator('.story[data-id="'+d.id+'"]');await row.locator('.headline').click();await row.getByRole('button',{name:'Find stock opportunities',exact:true}).click();
   await expect.poll(async()=>(await state()).game.proposals[d.id]?.status,{timeout:30000}).toBe('done');
   const form=row.locator('.investment-form');await expect(form.locator('.idea')).toHaveCount(2);
   await form.locator('input[name=idea]').nth(i).check();await form.locator('.stake-input').fill('1000');await form.getByRole('button',{name:'Add to portfolio',exact:true}).click();
   await expect(page.locator('#basket .basket-row')).toHaveCount(i+1);
  }
  const before=await state();assert.equal(before.game.round.status,'draft');assert.equal(before.game.round.positions.length,0);assert.equal(before.game.round.pnl,0);
  const exported=await page.request.get(process.env.APP_URL+'/api/game/export');const corpus=await exported.json();assert.equal(corpus.news.length,21);assert.equal(corpus.week.news_start,'2026-10-19');assert.equal(corpus.unavailable_ids.length,0);
  await page.locator('#reveal').click();await expect.poll(async()=>(await state()).game.round.status,{timeout:30000}).toBe('settled');s=await state();
  assert.equal(s.game.round.invested,2000);assert.ok(Math.abs(s.game.round.pnl-50)<.001);assert.ok(Math.abs(s.game.round.value-10050)<.001);assert.equal(s.game.round.positions[0].result.exit_date,'2026-10-27');assert.equal(s.game.round.week_complete,false);
  await expect(page.locator('#wallet')).toHaveText('$10,050.00');await expect(page.locator('#profit')).toContainText('+$50.00');await expect(page.locator('#round-note')).toContainText('Week in progress');await expect(page.locator('#reveal')).toBeHidden();
  const again=await page.request.post(process.env.APP_URL+'/api/game/play',{data:{round_id:s.game.round.id,memo_version:s.memo.version,positions:[]}});assert.equal(again.status(),409);
  await page.setViewportSize({width:390,height:844});assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'Mobile overflow');await page.setViewportSize({width:1440,height:1000});assert.deepEqual(errors,[]);
  const dir=path.join(__dirname,'results/simple-desk');fs.mkdirSync(dir,{recursive:true});await page.screenshot({path:path.join(dir,'archived-week-fixture.png'),fullPage:true});const report={passed:true,live_models:false,scope:'Explicit fixture providers through real Go handlers and browser',news:21,checks:['all seven archive days','manual file import with model results ignored','memo edit re-scoring','historical company research','portfolio choices hidden from market results before commit','two investments','positive and negative stock returns','budget and immutable round','no intraday/future valuation','week-in-progress label','full news export','mobile layout','mock-keychain','no JavaScript errors']};fs.writeFileSync(path.join(dir,'archived-week-fixture.json'),JSON.stringify(report,null,2)+'\n');console.log(JSON.stringify(report));
 }finally{await browser.close();}
})().catch(e=>{console.error(e.message);process.exitCode=1;});
