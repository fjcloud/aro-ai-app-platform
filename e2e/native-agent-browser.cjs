// Opt-in live test: open the CLI-created OGX backend and call its MaaS model.
const {chromium,expect}=require('@playwright/test');
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
(async()=>{
 const base=process.env.RHOAI_URL?.replace(/\/$/,''),state=process.env.BROWSER_STATE;
 assert.ok(base&&state,'Set RHOAI_URL and a private BROWSER_STATE file');
 const namespace=process.env.APP_NAMESPACE||'signal-desk-dev';
 const browser=await chromium.launch({headless:true,args:['--use-mock-keychain']});
 try{
  const context=await browser.newContext({storageState:state,viewport:{width:1440,height:1000}});
  const page=await context.newPage();
  await page.goto(base+'/gen-ai-studio/playground/'+encodeURIComponent(namespace));
  const skip=page.getByTestId('whats-new-skip-tour');if(await skip.isVisible())await skip.click();
  await expect(page.getByTestId('chatbot-message-bar')).toBeVisible({timeout:60000});
  await expect(page.getByRole('button',{name:'Create playground',exact:true})).toHaveCount(0);
  if(!await page.getByRole('tab',{name:'Model',exact:true}).count())await page.getByTestId('settings-button').click();
  await page.getByTestId('subscription-selector-toggle').click();
  await page.getByText('Card Scout runtime',{exact:true}).click();
  const stream=page.waitForResponse(r=>r.url().includes('/lsd/responses')&&r.request().method()==='POST',{timeout:180000});
  await page.getByTestId('chatbot-message-bar').fill('Reply with exactly CLI_BACKEND_OK.');
  await page.getByTestId('chatbot-message-bar').press('Enter');
  const response=await stream;assert.equal(response.status(),200);
  const raw=await response.text();
  const events=raw.split('\n').filter(s=>s.startsWith('data: {')).map(s=>JSON.parse(s.slice(6)));
  const done=events.find(e=>e.type==='response.completed')?.response;
  assert.equal(done?.status,'completed');
  assert.equal(done.model,'maas-vllm-inference-1/deepseekv4pro');
  const text=done.output.filter(o=>o.type==='message').flatMap(o=>o.content||[]).map(c=>c.text||'').join('');
  assert.ok(text.includes('CLI_BACKEND_OK'),'Missing actual model answer');
  const result={passed:true,project:namespace,backend_created_by:'CLI OGXServer',ui_creation_used:false,
   live_model:true,model:done.model,subscription:'Card Scout runtime',response_status:done.status,
   marker_verified:true,at:new Date().toISOString()};
  const output=path.join(__dirname,'results/native-agent-cli');fs.mkdirSync(output,{recursive:true});
  await expect(page.getByTestId('chatbot')).toContainText('CLI_BACKEND_OK');
  await page.screenshot({path:path.join(output,'playground.png'),fullPage:true});
  fs.writeFileSync(path.join(output,'playground.json'),JSON.stringify(result,null,2)+'\n');
  console.log(JSON.stringify(result));
 }finally{await browser.close();}
})().catch(e=>{console.error(e.message);process.exitCode=1});
