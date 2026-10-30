// Live acceptance: the app link must open the exact native MLflow trace graph.
const {chromium,expect}=require('@playwright/test');
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
(async()=>{
 assert.ok(process.env.APP_URL&&process.env.BROWSER_STATE,'Set APP_URL and BROWSER_STATE');
 const browser=await chromium.launch({headless:true,args:['--use-mock-keychain']});
 try{
  const context=await browser.newContext({storageState:process.env.BROWSER_STATE,viewport:{width:1440,height:1000}}),page=await context.newPage();
  await page.goto(process.env.APP_URL);await expect(page.locator('.story')).toHaveCount(3);
  const dir=path.join(__dirname,'results/simple-desk');fs.mkdirSync(dir,{recursive:true});await page.screenshot({path:path.join(dir,'landing.png'),fullPage:true});
  const state=await page.evaluate(async()=>await(await fetch('/api/game')).json());
  const d=state.documents.slice(0,3).find(d=>state.game.proposals[d.id]?.skill);
  assert.ok(d,'Run a fresh skill-assisted research on a visible story first');
  const proposal=state.game.proposals[d.id],row=page.locator('.story[data-id="'+d.id+'"]');
  await row.locator('.headline').click();await expect(row.locator('.trace-link')).toHaveAttribute('href',proposal.trace_url);
  const opened=context.waitForEvent('page');await row.locator('.trace-link').click();const trace=await opened;
  await trace.waitForLoadState();await expect(trace.getByText('Trace breakdown',{exact:true})).toBeVisible({timeout:60000});
  await expect(trace.getByText('game.research',{exact:true}).first()).toBeVisible();
  for(const name of ['tool.read_archive','tool.search_archive','tool.search_web','tool.read_web','tool.search_companies'])await expect(trace.getByText(name,{exact:true}).first()).toBeVisible();
  const u=new URL(trace.url()),query=new URLSearchParams(u.hash.split('?')[1]);assert.equal(query.get('selectedEvaluationId'),proposal.trace_id);assert.equal(query.get('workspace'),'signal-desk-dev');
  await trace.getByText('game.research',{exact:true}).first().click();await expect(trace.getByText('research.skill.name',{exact:true})).toBeVisible();
  await trace.screenshot({path:path.join(dir,'mlflow-graph.png'),fullPage:true});
  fs.writeFileSync(path.join(dir,'trace-link.json'),JSON.stringify({passed:true,at:new Date().toISOString(),trace_id:proposal.trace_id,trace_url:trace.url(),skill:proposal.skill,checks:['actual app link opens a new tab','exact trace and workspace','native MLflow graph with agent and tools','mock-keychain']},null,2)+'\n');
  console.log(JSON.stringify({passed:true,trace_url:trace.url(),skill:proposal.skill.name}));
 }finally{await browser.close();}
})().catch(e=>{console.error(e.stack);process.exitCode=1});
