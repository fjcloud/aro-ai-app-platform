// Real wall-clock Go simulation + real local T4 model; no simulated AI replies.
const {chromium,expect}=require('@playwright/test');const assert=require('node:assert/strict');const fs=require('node:fs');
(async()=>{const browser=await chromium.launch({headless:true,args:['--use-mock-keychain']});try{
 const context=await browser.newContext(process.env.BROWSER_STATE?{storageState:process.env.BROWSER_STATE}:{}),page=await context.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto(process.env.APP_URL||'http://127.0.0.1:18086',{waitUntil:'domcontentloaded'});
 for(let i=0;i<20;i++){if(await page.getByRole('heading',{name:'Sky Control.',exact:true}).count())break;const login=page.getByRole('button',{name:'Log in with OpenShift',exact:true});if(await login.count())await login.click();const allow=page.getByRole('button',{name:/Allow selected permissions|Allow/i});if(await allow.count())await allow.first().click();await page.waitForTimeout(500);}
 await expect(page.getByRole('heading',{name:'Sky Control.',exact:true})).toBeVisible({timeout:30000});await expect(page.locator('#play')).toBeEnabled();
 const get=()=>page.evaluate(async()=>await(await fetch('/api/state')).json());
 const control=(action,extra={})=>page.evaluate(async({action,extra})=>{const s=await(await fetch('/api/state')).json();const r=await fetch('/api/control',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:s.id,action,...extra})});if(!r.ok)throw Error('Sector control failed: '+r.status);return await r.json();},{action,extra});
 await control('reset',{seed:42,config:{aircraft:6,arrivals_per_minute:0,speed:12}});await expect.poll(()=>page.locator('#speed').inputValue()).toBe('12');
 await expect(page.locator('#play')).toHaveText('▶ Start AI');await page.locator('#play').click();
 await expect.poll(async()=>(await get()).pending,{timeout:30000}).toBe(true);const a=await get();await page.waitForTimeout(350);const b=await get();assert.ok(b.world.seconds>a.world.seconds,'The simulation stopped while inference was in flight');
 await expect.poll(async()=>(await get()).world.stats.accepted,{timeout:45000}).toBeGreaterThanOrEqual(3);
 await page.locator('.flight-strip').first().click();await expect(page.locator('#selected')).toBeVisible();
 await page.screenshot({path:process.env.SCREENSHOT||'/private/tmp/cloud-terminal-sky-local.png',fullPage:true});
 // Let real cleared trajectories reach the runway and the departure exit.
 await expect.poll(async()=>(await get()).world.stats.landed,{timeout:180000,intervals:[500,1000,1500]}).toBeGreaterThanOrEqual(1);
 await expect.poll(async()=>(await get()).world.stats.departed,{timeout:180000,intervals:[500,1000,1500]}).toBeGreaterThanOrEqual(1);
 const nominal=await get();assert.equal(nominal.world.stats.crashes,0);assert.equal(nominal.world.stats.safety_stops,0);assert.equal(nominal.world.stats.errors,0);
 await page.locator('#play').click();await expect(page.locator('#play')).toHaveText('▶ Start AI');const paused=await get();await page.waitForTimeout(450);assert.equal((await get()).world.seconds,paused.world.seconds,'Pause did not freeze the physics clock');
 await control('reset',{seed:42,config:{aircraft:6,arrivals_per_minute:0,speed:12}});await expect(page.locator('#decision-count')).toHaveText('0 cycles');
 await page.locator('#runway-list .runway-row').last().getByRole('button',{name:'Close',exact:true}).click();await expect(page.locator('#runway-list .runway-row').last().getByRole('button')).toHaveText('Reopen');assert.equal((await get()).world.runways[1].closed,true);
 await page.locator('#emergency').click();assert.ok((await get()).world.flights.some(p=>p.note.includes('EMERGENCY')),'Priority request was not added');
 await control('reset',{seed:42,config:{aircraft:6,arrivals_per_minute:0,speed:8}});await expect.poll(()=>page.locator('#speed').inputValue()).toBe('8');
 await page.getByRole('button',{name:'Push limit',exact:true}).click();await expect.poll(async()=>(await get()).world.config.aircraft).toBe(24);assert.equal((await get()).world.config.speed,60);
 await page.locator('#play').click();await expect.poll(async()=>(await get()).world.stats.late,{timeout:60000,intervals:[500,1000]}).toBeGreaterThanOrEqual(3);
 const stressed=await get();assert.equal(stressed.state,'critical');assert.equal(stressed.world.stats.crashes,0);assert.equal(stressed.world.stats.accepted,0,'An overdue decision was applied');assert.equal(stressed.world.stats.errors,0);
 assert.ok(stressed.history.filter(e=>e.status==='late').every(e=>e.total_ms>e.budget_ms));
 if(process.env.STRESS_SCREENSHOT)await page.screenshot({path:process.env.STRESS_SCREENSHOT,fullPage:true});
 await page.locator('#play').click();await expect(page.locator('#play')).toHaveText('▶ Start AI');
 await page.setViewportSize({width:390,height:844});assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'Mobile page overflows');await page.locator('.flight-strip').first().click();await expect(page.locator('#selected')).toBeVisible();assert.deepEqual(errors,[]);
 const summary=s=>({config:s.world.config,simSeconds:s.world.seconds,stats:s.world.stats,cycles:s.history.filter(e=>e.status!=='cancelled').map(e=>({choice:e.choice,status:e.status,total_ms:e.total_ms,inference_ms:e.inference_ms,budget_ms:e.budget_ms,input_tokens:e.input_tokens,trace_id:e.trace_id}))});
 const report={passed:true,scope:'Real browser, asynchronous Go physics and local d3-mini on one T4',nominal:summary(nominal),stressed:summary(stressed),checks:['physics advances during inference','real landing and departure','flight inspection','pause freezes simulation','runway closure','medical priority request','difficulty sliders and preset','overdue results discarded','critical inference state shown separately from backlog','zero crashes','no safety stops in nominal run','mobile layout','no browser errors']};
 if(process.env.RESULT_FILE)fs.writeFileSync(process.env.RESULT_FILE,JSON.stringify(report,null,2)+'\n');console.log(JSON.stringify({passed:true,nominal:report.nominal.stats,stressed:report.stressed.stats,checks:report.checks},null,2));
 }finally{await browser.close();}})().catch(e=>{console.error(e.message);process.exitCode=1;});
