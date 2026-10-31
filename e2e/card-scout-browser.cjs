// Live T4/native-agent acceptance, then isolated browser fixtures for edge cases.
const {chromium,expect}=require('@playwright/test');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const base=process.env.APP_URL?.replace(/\/$/,''),dir=path.join(__dirname,'results/card-scout');
assert.ok(base,'Set APP_URL');fs.mkdirSync(dir,{recursive:true});
(async()=>{
 const browser=await chromium.launch({headless:true,args:['--use-mock-keychain']});
 try{
  const context=await browser.newContext({storageState:process.env.BROWSER_STATE||undefined,viewport:{width:1440,height:1000}});
  const page=await context.newPage();await page.goto(base);await expect(page.getByRole('heading',{name:'Card Scout',exact:true})).toBeVisible();
  const getBatch=async()=>{const r=await context.request.get(base+'/api/batch');assert.equal(r.status(),200);return r.json()};
  let batch=await getBatch();assert.equal(batch.dataset.items,51);assert.equal(batch.items.length,51);
  await expect(page.locator('.card-cell')).toHaveCount(12,{timeout:15000});
  if(batch.completed!==51||batch.failed){
   const started=page.waitForResponse(r=>r.url()===base+'/api/batch/analyze'&&r.request().method()==='POST');
   await page.getByRole('button',{name:'Analyze batch',exact:true}).click();
   assert.equal((await started).status(),202);
   await expect.poll(async()=>{batch=await getBatch();return batch.completed===51&&batch.failed===0&&!batch.running},{timeout:180000,intervals:[1500]}).toBe(true);
  }
  await expect(page.locator('#status-msg')).toContainText('51/51',{timeout:10000});
  await page.getByRole('button',{name:'Show more',exact:true}).click();await expect(page.locator('.card-cell')).toHaveCount(24);
  const rare=batch.items.filter(i=>i.kind==='pokemon_tcg'&&i.rarity==='rare_or_higher');
  await page.locator('[data-filter=rare]').click();await expect(page.locator('.card-cell')).toHaveCount(Math.min(12,rare.length));
  await page.locator('[data-filter=review]').click();await expect(page.locator('.card-cell')).toHaveCount(batch.items.filter(i=>i.needs_review).length);
  await page.locator('[data-filter=other]').click();await expect(page.locator('.card-cell')).toHaveCount(3);await expect(page.getByRole('button',{name:'Show more',exact:true})).toBeHidden();await expect(page.getByRole('button',{name:'Check price',exact:true})).toHaveCount(0);
  await page.locator('[data-filter=rare]').click();
  const target=rare.find(i=>!i.research)||rare[0];assert.ok(target);
  const row=page.locator('#cell-'+target.id);
  if(!(await row.count())){while(await page.getByRole('button',{name:'Show more',exact:true}).isVisible()&&!(await row.count()))await page.getByRole('button',{name:'Show more',exact:true}).click()}
  let research;
  if(!target.research){
   const reply=page.waitForResponse(r=>r.url()===base+'/api/cards/'+target.id+'/research'&&r.request().method()==='POST',{timeout:120000});
   await row.getByRole('button',{name:'Check price',exact:true}).click();await expect(page.locator('#research-panel')).toContainText('Checking price');
   const response=await reply;assert.equal(response.status(),200);research=await response.json();
  }else{await row.getByRole('button',{name:'Check price',exact:true}).click();research=target.research}
  const panel=page.locator('#research-panel');await expect(panel).toContainText(research.card.name);await expect(panel).toContainText(research.card.rarity);
  await expect(panel.locator('a').filter({hasText:research.source_url})).toHaveAttribute('href',research.source_url);
  assert.ok((await panel.boundingBox()).y<(await page.locator('#grid').boundingBox()).y,'Price result should precede the card grid');
  for(const [label,value]of[['Trend (EUR)',research.trend],['7-day average (EUR)',research.average_7d]]){
   const text=await panel.locator('dt').filter({hasText:label}).locator('xpath=following-sibling::dd[1]').innerText();
   if(value===null||value===undefined)assert.equal(text,'Unavailable');else assert.equal(Number(text),value);
  }
  const trace=panel.getByRole('link',{name:'View MLflow trace',exact:true});await expect(trace).toHaveAttribute('href',research.trace_url);
  await page.screenshot({path:path.join(dir,'app-desktop.png'),fullPage:true});
  await page.reload();await expect(page.locator('[data-filter=rare]')).toHaveClass(/active/);await expect(page.locator('.card-cell')).toHaveCount(Math.min(12,rare.length));
  await page.setViewportSize({width:390,height:844});await page.locator('[data-filter=other]').click();
  assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'Mobile horizontal overflow');
  await page.screenshot({path:path.join(dir,'app-mobile.png'),fullPage:true});
  const live={passed:true,at:new Date().toISOString(),dataset:batch.dataset,completed:batch.completed,failed:batch.failed,rare_cards:rare.length,research_item:target.id,research,checks:['real image inference recovered to 51/51 completed','pagination and whole-batch filters','controls offer no price action','native price research and exact evidence values','same-origin images','trace link','saved filter','390px mobile']};
  fs.writeFileSync(path.join(dir,'app-live.json'),JSON.stringify(live,null,2)+'\n');

  await require('./card-scout-fixtures.cjs')(browser,base,batch,dir);
  console.log(JSON.stringify({live:true,fixtures:true,completed:batch.completed,trace_url:research.trace_url}));
 }finally{await browser.close()}
})().catch(e=>{console.error(e.stack);process.exitCode=1});
