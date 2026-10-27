// Actual Go game + local visual model. The injected outage is a separate UI check.
const {chromium,expect}=require('@playwright/test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
(async()=>{
 const browser=await chromium.launch({headless:true,args:['--use-mock-keychain']});
 try{
  const context=await browser.newContext(process.env.BROWSER_STATE?{storageState:process.env.BROWSER_STATE}:{});
  const page=await context.newPage(),errors=[];page.on('pageerror',e=>{errors.push(e.message);console.error('Browser error:',e.message);});
  if(process.env.DEBUG_GAME)page.on('response',async r=>{if(r.url().includes('/api/shift/')){let d;try{d=await r.json();}catch{}console.log(JSON.stringify({path:new URL(r.url()).pathname,status:r.status(),round:d?.round,result:!!d?.result,error:d?.error}));}});
  await page.goto(process.env.APP_URL||'http://127.0.0.1:18084',{waitUntil:'domcontentloaded'});
  for(let i=0;i<20;i++){
   if(await page.getByRole('heading',{name:'Orbit Checkpoint.',exact:true}).count())break;
   const login=page.getByRole('button',{name:'Log in with OpenShift',exact:true});if(await login.count())await login.click();
   const allow=page.getByRole('button',{name:/Allow selected permissions|Allow/i});if(await allow.count())await allow.first().click();
   await page.waitForTimeout(500);
  }
  await expect(page.getByRole('heading',{name:'Orbit Checkpoint.',exact:true})).toBeVisible({timeout:30000});
  async function waitForDecision(){await expect(page.locator('#result:not([hidden]), #error:not([hidden])')).toBeVisible({timeout:90000});if(await page.locator('#error').isVisible())throw Error(await page.locator('#error').innerText());}
  await expect(page.locator('#clear')).toBeEnabled();
  await page.locator('#seed').fill('42');await page.locator('#reset').click();await expect(page.locator('#clear')).toBeEnabled();
  await expect(page.locator('#directive-name')).toHaveText('Medical fast lane');
  await expect.poll(()=>page.locator('#document').evaluate(e=>e.complete&&e.naturalWidth)).toBe(1000);
  const initial=await page.evaluate(async()=>await(await fetch('/api/shift')).json());
  assert.equal(initial.result,null);assert.ok(!('cases' in initial));assert.ok(!JSON.stringify(initial).includes('ZURI VOSS'));
  await page.locator('#zoom').click();await expect(page.locator('#zoom-dialog')).toBeVisible();await page.keyboard.press('Escape');await expect(page.locator('#zoom-dialog')).not.toBeVisible();
  await page.locator('#clear').click();await waitForDecision();
  await expect(page.locator('#human-mark')).toHaveText('✓ Correct stamp');await expect(page.locator('#ai-mark')).toHaveText('✓ Correct stamp');
  await expect(page.locator('#expected')).toHaveText('Clear');await expect(page.locator('.prob-row')).toHaveCount(3);
  await expect(page.locator('#model-detail')).toContainText('0 generated tokens');
  const firstLatency=await page.locator('#latency').innerText();
  await page.screenshot({path:process.env.SCREENSHOT||'/private/tmp/cloud-terminal-orbit-local.png',fullPage:true});
  await page.locator('#next').click();await expect(page.locator('#clear')).toBeEnabled();
  await page.locator('#clear').click();await waitForDecision();
  await expect(page.locator('#human-mark')).toHaveText('✕ Incorrect stamp');await expect(page.locator('#expected')).toHaveText('Reject');
  // Duplicate submission must be rejected without another decision.
  assert.equal(await page.evaluate(async()=>{const s=await(await fetch('/api/shift')).json();return(await fetch('/api/shift/decide',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:s.id,round:s.round,action:'clear'})})).status;}),409);
  await page.locator('#next').click();await expect(page.locator('#clear')).toBeEnabled();
  await page.route('**/api/shift/decide',route=>route.fulfill({status:502,contentType:'application/json',body:JSON.stringify({error:'Injected local model outage'})}));
  const before=await page.locator('#log-count').innerText();await page.locator('#ai-one').click();await expect(page.locator('#error')).toContainText('Injected local model outage');
  assert.equal(await page.locator('#log-count').innerText(),before);await expect(page.locator('#result')).not.toBeVisible();
  await page.unroute('**/api/shift/decide');
  await page.locator('#run').click();await expect.poll(()=>page.locator('#history li').count(),{timeout:90000}).toBeGreaterThanOrEqual(4);
  await expect(page.locator('#directive-name')).toHaveText('Relic quarantine',{timeout:90000});
  await page.locator('#run').click();await expect(page.locator('#run')).toHaveText('▶ Watch AI shift');
  await expect(page.locator('#reset')).toBeEnabled({timeout:90000});const paused=await page.locator('#log-count').innerText();await page.waitForTimeout(2200);assert.equal(await page.locator('#log-count').innerText(),paused);
  await page.locator('#run').click();await expect(page.locator('#summary')).toBeVisible({timeout:180000});
  await expect(page.locator('#history li')).toHaveCount(12);await expect(page.locator('#ai-detail')).toContainText('of 12 inspections');
  const end=await page.evaluate(async()=>await(await fetch('/api/shift')).json());assert.equal(end.over,true);assert.equal(end.human_played,2);
  assert.equal(end.history.length,12);assert.equal(end.history[1].human_correct,false);
  await page.setViewportSize({width:390,height:844});assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'Mobile page overflows');
  await page.locator('#reset').click();await expect(page.locator('#clear')).toBeEnabled();await expect(page.locator('#log-count')).toHaveText('0 inspections');
  await page.locator('#zoom').click();await expect(page.locator('#zoom-dialog')).toBeVisible();await expect.poll(()=>page.locator('.zoom-scroll').evaluate(e=>e.scrollWidth>e.clientWidth),{timeout:5000,message:'Full-size mobile document cannot be inspected'}).toBe(true);await page.locator('#close-zoom').click();
  assert.deepEqual(errors,[]);
  const report={passed:true,scope:'Real browser, Go game and local d3-mini visual inference; separately injected HTTP outage',firstInference:firstLatency,fullShift:{couriers:end.history.length,modelCorrect:end.model_correct,humanPlayed:end.human_played,humanCorrect:end.human_correct},checks:['image evidence without extracted fields','document zoom','human correct and incorrect stamps','typed model probabilities','duplicate rejection','no fallback on error','autoplay','changing directives','pause and cancellation','12-courier completion','reset','mobile layout and full-size zoom','no browser errors']};
  if(process.env.RESULT_FILE)fs.writeFileSync(process.env.RESULT_FILE,JSON.stringify(report,null,2)+'\n');console.log(JSON.stringify(report,null,2));
 }finally{await browser.close();}
})().catch(e=>{console.error(e.message);process.exitCode=1;});
