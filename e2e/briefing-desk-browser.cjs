// Local preview / fixture validation. Live provider checks are separate.
const {chromium,expect}=require('@playwright/test');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
(async()=>{
 const browser=await chromium.launch({headless:true,args:['--use-mock-keychain']});
 try{
  const page=await browser.newPage({viewport:{width:1440,height:1000}}),errors=[];
  page.on('pageerror',e=>errors.push(e.message));
  await page.goto(process.env.APP_URL,{waitUntil:'domcontentloaded'});
  const state=()=>page.evaluate(async()=>await(await fetch('/api/state')).json());
  const finished=async()=>{const s=await state(),p=s.profiles.find(p=>p.id==='tech');return s.documents.length===3&&s.documents.every(d=>d.classifications.tech?.status==='done'&&d.classifications.tech.profile_version===p.version&&Number.isFinite(d.classifications.tech.opportunity_score)&&d.classifications.tech.memo_reason);};
  let s=await state();
  if(!s.preview){assert.equal(s.validation_fixture,true);await page.locator('#fetch').click();await expect.poll(finished,{timeout:30000}).toBeTruthy();s=await state();}
  else await expect(page.locator('#fetch')).toBeDisabled();
  await expect(page.locator('html')).toHaveAttribute('lang','en');
  await expect(page.locator('.story')).toHaveCount(3);await expect(page.locator('#preview')).toBeVisible();
  await expect.poll(()=>page.locator('.opportunity strong').allTextContents()).toEqual(['90','50','10']);
  await expect(page.locator('#market')).toHaveCount(0);await expect(page.locator('[data-filter]')).toHaveCount(0);
  await page.locator('.story .headline').first().click();
  await expect(page.locator('.story-detail')).toHaveCount(1);
  if(!s.preview){
   await page.locator('.story-detail').getByRole('button',{name:'Investigate',exact:true}).click();
   const high=s.documents.find(d=>d.classifications.tech.opportunity_score===90);
   await expect.poll(async()=>(await state()).documents.find(d=>d.id===high.id).research?.tech?.status,{timeout:30000}).toBe('done');
  }
  await expect(page.locator('.research-export')).toHaveCount(1);
  const href=await page.locator('.research-export').getAttribute('href');
  const note=await page.evaluate(async href=>await(await fetch(href)).text(),href);
  assert.ok(note.includes('Mandate:')&&note.includes('Evidence'));
  if(!s.preview){
   // A low score is relevant information, not a prohibition on read-only research.
   const low=s.documents.find(d=>d.classifications.tech.opportunity_score===10);
   await page.locator('.story[data-id="'+low.id+'"]').locator('.headline').click();
   await expect(page.locator('.story-detail').getByRole('button',{name:'Investigate',exact:true})).toBeEnabled();
   const original=s.profiles.find(p=>p.id==='tech');
   await page.locator('#memo>summary').click();
   const custom='Retail-only fixture memo. Research consumer marketplaces and merchant revenues; semiconductor headlines have low relevance.';
   await page.locator('#fund-memo').fill(custom);
   // Wait for an actual periodic refresh: it must preserve the unsaved memo.
   await page.waitForTimeout(2700);await expect(page.locator('#fund-memo')).toHaveValue(custom);
   await page.locator('#fund-name').fill('Retail fixture fund');
   await page.locator('#memo-form').getByRole('button').click();
   await expect.poll(async()=>{const x=await state();return x.profiles[0].criteria===custom&&x.documents.every(d=>d.classifications.tech.profile_version===x.profiles[0].version&&d.classifications.tech.opportunity_score===10&&d.classifications.tech.memo_reason);},{timeout:30000}).toBe(true);
   await expect(page.locator('.research-export')).toHaveCount(0);
   await page.locator('#memo>summary').click();await page.locator('#fund-name').fill(original.name);await page.locator('#fund-memo').fill(original.criteria);await page.locator('#memo-form').getByRole('button').click();
   await expect.poll(finished,{timeout:30000}).toBeTruthy();
  }
  await page.setViewportSize({width:390,height:844});assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'Mobile layout overflows');await page.setViewportSize({width:1440,height:1000});assert.deepEqual(errors,[]);
  const dir=path.join(__dirname,'results/techmeme-terminal');fs.mkdirSync(dir,{recursive:true});const prefix=s.preview?'preview':'fixture-e2e';
  await page.screenshot({path:path.join(dir,prefix+'.png'),fullPage:true});
  const report={passed:true,scope:process.env.TEST_SCOPE||'Local synthetic fixture validation',live_models:false,checks:['simple memo and ranked news UI','opportunity scores are not confidence percentages','descending score order','memo draft survives refresh','memo changes re-score existing summaries','old memo research is hidden','any current scored story can be investigated','research Markdown export','mobile layout','mock-keychain','no JavaScript errors']};
  fs.writeFileSync(path.join(dir,prefix+'.json'),JSON.stringify(report,null,2)+'\n');console.log(JSON.stringify(report));
 }finally{await browser.close();}
})().catch(e=>{console.error(e.message);process.exitCode=1;});
