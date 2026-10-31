const {expect}=require('@playwright/test');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
module.exports=async function runFixtures(browser,base,batch,dir,outputName='app-fixtures.json'){
  // Browser-only fixture data never enters the running platform or its state.
  const fixtureContext=await browser.newContext({viewport:{width:1440,height:1000}});const fixturePage=await fixtureContext.newPage(),common=structuredClone(batch.items.find(i=>i.kind==='pokemon_tcg'&&i.rarity==='common'));
  common.research=null;const fixture={...batch,completed:1,failed:0,running:false,items:[common]};let initialFail=true,priceFail=false;
  await fixturePage.route(base+'/api/batch',async route=>{if(initialFail){initialFail=false;return route.fulfill({status:503,json:{error:'Fixture offline'}})}return route.fulfill({json:fixture})});
  await fixturePage.route(base+'/api/cards/'+common.id+'/research',async route=>route.fulfill(priceFail?{status:502,json:{error:'Fixture price source unavailable'}}:{json:{status:'ok',card:{name:'<img src=x onerror="window.fixtureXss=true">',set:'Fixture set',number:'1',rarity:'Common'},currency:'EUR',trend:0,average_7d:null,source_url:'javascript:alert(1)',trace_url:'data:text/html,test',provider:'Fixture',provider_updated_at:null,note:'Fixture values'}}));
  await fixturePage.goto(base);await expect(fixturePage.locator('#main-error-wrap')).toBeVisible();await fixturePage.locator('#main-error-retry').click();await expect(fixturePage.locator('.card-cell')).toHaveCount(1);await expect(fixturePage.getByRole('button',{name:'Show more',exact:true})).toBeHidden();
  await fixturePage.locator('[data-filter=rare]').click();await expect(fixturePage.locator('.empty-state')).toBeVisible();await expect(fixturePage.locator('#filters')).toBeVisible();
  await fixturePage.reload();await expect(fixturePage.locator('[data-filter=rare]')).toHaveClass(/active/);await expect(fixturePage.locator('#filters')).toBeVisible();
  await fixturePage.locator('[data-filter=all]').click();await fixturePage.getByRole('button',{name:'Check price',exact:true}).click();
  const fp=fixturePage.locator('#research-panel');await expect(fp).toContainText('Unavailable');await expect(fp.locator('img')).toHaveCount(0);await expect(fp.locator('a')).toHaveCount(0);assert.equal(await fixturePage.evaluate(()=>window.fixtureXss),undefined);
  await expect(fp.locator('dt').filter({hasText:'Trend (EUR)'}).locator('xpath=following-sibling::dd[1]')).toHaveText('0');
  priceFail=true;await fixturePage.reload();await fixturePage.getByRole('button',{name:'Check price',exact:true}).click();await expect(fp).toContainText('Fixture price source unavailable');await expect(fp.getByRole('heading')).toContainText(common.name);
  const fixtures={passed:true,checks:['initial failure and Retry','empty filter stays recoverable','empty filter persists on reload','zero is not missing','null is Unavailable','HTML is escaped','unsafe links rejected','price failure preserves selection'],at:new Date().toISOString()};
  fs.writeFileSync(path.join(dir,outputName),JSON.stringify(fixtures,null,2)+'\n');
};
