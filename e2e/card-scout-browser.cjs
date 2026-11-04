// Live T4/native-agent acceptance, then isolated browser fixtures for edge cases.
const {chromium,expect}=require('@playwright/test');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const base=process.env.APP_URL?.replace(/\/$/,''),dir=process.env.CARD_SCOUT_RESULTS_DIR||path.join(__dirname,'results/card-scout');
assert.ok(base,'Set APP_URL');fs.mkdirSync(dir,{recursive:true});
(async()=>{
 const browser=await chromium.launch({headless:true,args:['--use-mock-keychain']});
 try{
  const context=await browser.newContext({storageState:process.env.BROWSER_STATE||undefined,viewport:{width:1440,height:1000}});
  const page=await context.newPage();await page.goto(base);await expect(page.getByRole('heading',{name:'Card Scout',exact:true})).toBeVisible();
  const getBatch=async()=>{const r=await context.request.get(base+'/api/batch');assert.equal(r.status(),200);return r.json()};
  const checkWholeCard=async()=>{
   const img=page.locator('.card img').first();
   await expect.poll(()=>img.evaluate(i=>i.complete&&i.naturalWidth>0),{timeout:15000}).toBe(true);
   const geometry=await img.evaluate(i=>{const b=i.getBoundingClientRect();return {fit:getComputedStyle(i).objectFit,ratio:b.width/b.height,natural:i.naturalWidth/i.naturalHeight}});
   assert.notEqual(geometry.fit,'cover','The complete card must be visible, not cropped to fill');
   assert.ok(Math.abs(geometry.ratio/geometry.natural-1)<0.03,'Card frame must preserve the source portrait proportions');
  };
  let batch=await getBatch();assert.equal(batch.dataset.items,105);assert.equal(batch.items.length,105);
  await expect(page.locator('.card')).toHaveCount(12,{timeout:15000});
  await checkWholeCard();
  if(batch.completed!==105||batch.failed){
   const started=page.waitForResponse(r=>r.url()===base+'/api/batch/analyze'&&r.request().method()==='POST');
   await page.getByRole('button',{name:'Analyze batch',exact:true}).click();
   assert.equal((await started).status(),202);
   await expect.poll(async()=>{batch=await getBatch();return batch.completed===105&&batch.failed===0&&!batch.running},{timeout:300000,intervals:[1500]}).toBe(true);
  }
  await expect(page.locator('#progress-msg')).toContainText('105 completed',{timeout:10000});
  for(const [id,kind]of[['item-203','not_a_card'],['item-204','other_card'],['item-205','other_card']]){
   const item=batch.items.find(i=>i.id===id);assert.ok(item);
   assert.equal(item.kind,kind);assert.equal(item.rarity,'not_applicable');
   const denied=await context.request.post(base+'/api/cards/'+id+'/research');
   assert.equal(denied.status(),409,'Negative controls must not invoke Pokemon price research');
  }
  await page.getByRole('button',{name:'Show more',exact:true}).click();await expect(page.locator('.card')).toHaveCount(24);
  const rare=batch.items.filter(i=>i.kind==='pokemon_tcg'&&i.rarity==='rare_or_higher');
  await page.locator('[data-filter=rare]').click();await expect(page.locator('.card')).toHaveCount(Math.min(12,rare.length));
  await page.locator('[data-filter=review]').click();await expect(page.locator('.card')).toHaveCount(Math.min(12,batch.items.filter(i=>i.needs_review).length));
  const other=batch.items.filter(i=>['other_card','not_a_card'].includes(i.kind));
  await page.locator('[data-filter=other]').click();await expect(page.locator('.card')).toHaveCount(Math.min(12,other.length));await expect(page.getByRole('button',{name:'Show more',exact:true})).toBeHidden();await expect(page.getByRole('button',{name:'Check price',exact:true,disabled:false})).toHaveCount(0);
  await page.locator('[data-filter=rare]').click();
  const target=rare.find(i=>!i.research)||rare[0];assert.ok(target);
  const row=page.locator('img[src="/assets/'+target.id+'"]').locator('xpath=ancestor::*[.//button][1]');
  if(!(await row.count())){while(await page.getByRole('button',{name:'Show more',exact:true}).isVisible()&&!(await row.count()))await page.getByRole('button',{name:'Show more',exact:true}).click()}
  let research;
  if(!target.research){
   const reply=page.waitForResponse(r=>r.url()===base+'/api/cards/'+target.id+'/research'&&r.request().method()==='POST',{timeout:120000});
   await row.getByRole('button',{name:'Check price',exact:true}).click();await expect(page.locator('#price-panel')).toContainText('Checking price');
   const response=await reply;assert.equal(response.status(),200);research=await response.json();
  }else{await row.getByRole('button',{name:'Check price',exact:true}).click();research=target.research}
  const panel=page.locator('#price-panel');await expect(panel).toContainText(research.card.name);await expect(panel).toContainText(research.card.rarity);
  if(research.note)await expect(panel).toContainText(research.note);
  await expect(panel.getByRole('link',{name:'Source',exact:true})).toHaveAttribute('href',research.source_url);
  assert.ok((await panel.boundingBox()).y<(await page.locator('#grid').boundingBox()).y,'Price result should precede the card grid');
  for(const [label,value]of[['Trend (EUR)',research.trend],['7-day average (EUR)',research.average_7d]]){
   const field=panel.locator('.price-item').filter({has:page.locator('strong').filter({hasText:label.replace(' (EUR)','')})});const text=(await field.innerText()).replace(await field.locator('strong').innerText(),'').trim();
   if(value===null||value===undefined)assert.equal(text,'Unavailable');else assert.equal(Number(text),value);
  }
  const trace=panel.getByRole('link',{name:'View MLflow trace',exact:true});await expect(trace).toHaveAttribute('href',research.trace_url);
  await page.screenshot({path:path.join(dir,'app-desktop.png'),fullPage:true});
  await page.reload();await expect(page.locator('[data-filter=rare]')).toHaveClass(/active/);await expect(page.locator('.card')).toHaveCount(Math.min(12,rare.length));
  await page.setViewportSize({width:390,height:844});await checkWholeCard();await page.locator('[data-filter=other]').click();
  assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'Mobile horizontal overflow');
  await page.locator('[data-filter=rare]').click();await checkWholeCard();
  await page.screenshot({path:path.join(dir,'app-mobile.png'),fullPage:true});
  const live={passed:true,at:new Date().toISOString(),dataset:batch.dataset,completed:batch.completed,failed:batch.failed,rare_cards:rare.length,research_item:target.id,research,checks:['real image inference recovered to 105/105 completed','pagination and whole-batch filters','Other filter agrees with model choices','native price research and exact evidence values','same-origin images','trace link','saved filter','complete card and portrait proportions on desktop and mobile','390px mobile']};
  fs.writeFileSync(path.join(dir,'app-live.json'),JSON.stringify(live,null,2)+'\n');

  await require('./card-scout-fixtures.cjs')(browser,base,batch,dir);
  console.log(JSON.stringify({live:true,fixtures:true,completed:batch.completed,trace_url:research.trace_url}));
 }finally{await browser.close()}
})().catch(e=>{console.error(e.stack);process.exitCode=1});
