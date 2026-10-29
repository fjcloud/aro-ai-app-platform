// Real browser + deployed local model. Never starts a hosted inference client.
// APP_URL=https://... BROWSER_STATE=/private/login-state.json node e2e/decision-tetris-browser.cjs
const {chromium,expect}=require('@playwright/test');
const assert=require('node:assert/strict');
(async()=>{
 const base=process.env.APP_URL||'http://127.0.0.1:18083';
 const browser=await chromium.launch({headless:true,args:['--use-mock-keychain']});
 try{
  const context=await browser.newContext(process.env.BROWSER_STATE?{storageState:process.env.BROWSER_STATE}:{});
  const page=await context.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.goto(base,{waitUntil:'domcontentloaded'});
  for(let i=0;i<20;i++){
   if(await page.getByRole('heading',{name:'Decision Tetris.',exact:true}).count())break;
   const login=page.getByRole('button',{name:'Log in with OpenShift',exact:true});if(await login.count())await login.click();
   const allow=page.getByRole('button',{name:/Allow selected permissions|Allow/i});if(await allow.count())await allow.first().click();
   await page.waitForTimeout(500);
  }
  await expect(page.getByRole('heading',{name:'Decision Tetris.',exact:true})).toBeVisible({timeout:30000});
  await expect(page.locator('#status')).toHaveText('Paused');
  await page.locator('#seed').fill('42');await page.locator('#reset').click();await expect(page.locator('#step')).toBeEnabled();
  await page.locator('#step').click();await expect(page.locator('#turn')).toHaveText('1',{timeout:45000});
  await expect(page.locator('#choice')).toContainText('d3-mini chose');
  await expect(page.locator('.prob-row')).toHaveCount(5);await expect(page.locator('#tokens')).toContainText('0 generated tokens');
  const latency=await page.locator('#latency').innerText();
  await page.locator('#run').click();await expect.poll(async()=>Number(await page.locator('#turn').textContent()),{timeout:45000}).toBeGreaterThanOrEqual(4);
  await page.locator('#run').click();await expect(page.locator('#status')).toHaveText('Paused',{timeout:45000});
  const paused=await page.locator('#turn').innerText();await page.waitForTimeout(1500);assert.equal(await page.locator('#turn').innerText(),paused);
  await page.locator('#mode').selectOption('manual');
  const options=await page.locator('#placement option').evaluateAll(es=>es.map(e=>e.value));const selected=options.at(-1);
  await page.locator('#placement').selectOption(selected);await page.locator('#step').click();await expect(page.locator('#choice')).toHaveText(`manual chose ${selected}`);
  await page.locator('#mode').selectOption('model');const beforeFailure=await page.locator('#turn').innerText();
  await page.route('**/api/game/step',route=>route.fulfill({status:502,contentType:'application/json',body:JSON.stringify({error:'Injected local model outage'})}));
  await page.locator('#run').click();await expect(page.locator('#error')).toHaveText('Injected local model outage');
  assert.equal(await page.locator('#turn').innerText(),beforeFailure,'A failed model call must not apply a fallback move');
  await page.unroute('**/api/game/step');await page.locator('#reset').click();await expect(page.locator('#step')).toBeEnabled();await expect(page.locator('#turn')).toHaveText('0');
  await page.locator('#step').click();await expect(page.locator('#turn')).toHaveText('1',{timeout:45000});
  await page.screenshot({path:process.env.SCREENSHOT||'/private/tmp/cloud-terminal-tetris-live.png',fullPage:true});
  await page.setViewportSize({width:390,height:844});assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'Mobile layout overflows');
  assert.deepEqual(errors,[]);
  console.log(JSON.stringify({passed:true,scope:'Real browser, Go game and local d3-mini; injected HTTP outage for failure handling',firstDecisionLatency:latency,checks:['real model move','probability bars','autoplay','pause and cancellation','manual placement','no fallback on error','reset','mobile layout','no browser errors']},null,2));
 }finally{await browser.close();}
})().catch(e=>{console.error(e.message);process.exitCode=1;});
