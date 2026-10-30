// Explicit fixture providers: real repository corpus, synthetic inference and prices.
const {chromium,expect}=require('@playwright/test');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
(async()=>{
 const browser=await chromium.launch({headless:true,args:['--use-mock-keychain']});
 try{
  const page=await browser.newPage({viewport:{width:1440,height:1000}}),errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.goto(process.env.APP_URL);
  const state=()=>page.evaluate(async()=>await(await fetch('/api/game')).json());
  await expect(page.locator('#periods')).toContainText('2026-10-26');
  await expect(page.locator('#fetch')).toHaveText('Load saved news');
  await expect(page.locator('#import-news')).toBeHidden();
  await page.locator('#fetch').click();
  await expect.poll(async()=>{const s=await state();return s.game.status==='ready'&&s.ranked===s.total},{timeout:60000}).toBe(true);
  let s=await state();assert.equal(s.total,161);assert.equal(s.game.week.valuation_mode,'latest');assert.equal(s.game.week.news_end,'2026-10-28');
  await expect(page.locator('#coverage')).toContainText('Repository Techmeme snapshot');
  const corpus=await page.evaluate(async()=>await(await fetch('/api/game/export')).json());assert.equal(corpus.news.length,161);
  const d=s.documents[0],row=page.locator('.story[data-id="'+d.id+'"]');await row.locator('.headline').click();
  await row.getByRole('button',{name:'Find stock opportunities',exact:true}).click();
  await expect.poll(async()=>{s=await state();return s.game.proposals[d.id]?.status},{timeout:30000}).toBe('done');
  assert.deepEqual([...new Set(s.game.proposals[d.id].evidence.map(e=>e.tool))].sort(),['read_archive','search_archive','search_companies']);
  await row.locator('.stake-input').fill('1000');await row.getByRole('button',{name:'Add to portfolio',exact:true}).click();
  await expect(page.locator('#reveal')).toHaveText('Compare with today’s market');
  await page.locator('#reveal').click();
  await expect.poll(async()=>(await state()).game.round.status).toBe('settled');s=await state();
  const round=s.game.round,result=round.positions[0].result;
  assert.equal(result.entry_date,'2026-10-29');assert.ok(result.quote_time);assert.ok(result.price_type.includes('latest available'));
  assert.ok(Math.abs(round.pnl-1000*(result.exit_price/result.entry_price-1))<0.000001);
  await expect(page.locator('#round-note')).toContainText('Latest available market quote');
  const positions=JSON.stringify(round.positions.map(p=>({symbol:p.symbol,amount:p.amount,idea_id:p.idea_id})));
  await page.locator('#update').click();await expect.poll(async()=>(await state()).game.round.status).toBe('settled');
  const updated=(await state()).game.round;assert.equal(JSON.stringify(updated.positions.map(p=>({symbol:p.symbol,amount:p.amount,idea_id:p.idea_id}))),positions);
  await page.setViewportSize({width:390,height:844});assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'Mobile overflow');
  await page.setViewportSize({width:1440,height:1000});assert.deepEqual(errors,[]);
  const output=path.join(__dirname,'results/company-opportunities');fs.mkdirSync(output,{recursive:true});
  await page.screenshot({path:path.join(output,'saved-news-fixture.png'),fullPage:true});
  fs.writeFileSync(path.join(output,'saved-news-fixture.json'),JSON.stringify({passed:true,scope:process.env.TEST_SCOPE,live_models:false,repository_news:s.total,week:s.game.week,checks:['fixed repository corpus','no manual import','model scoring','archive tool loop','timestamped intraday quote','portfolio freeze and arithmetic','quote refresh','mobile layout','mock-keychain','no JavaScript errors']},null,2)+'\n');
  console.log(JSON.stringify({passed:true,scope:'repository snapshot with explicit model/price fixtures',news:s.total}));
 }finally{await browser.close()}
})().catch(e=>{console.error(e.stack);process.exitCode=1});
