// Live Signal Desk Playground: MaaS model, system prompt and actual MCP calls.
// CAPTURE_GUIDE=true refreshes the illustrated guide using the actual UI.
const{chromium,expect}=require('@playwright/test');
const fs=require('fs'),path=require('path'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..'),images=path.join(root,'content/modules/ROOT/assets/images');
const base=process.env.RHOAI_URL?.replace(/\/$/,''),app=process.env.APP_URL?.replace(/\/$/,''),state=process.env.BROWSER_STATE;
const namespace=process.env.APP_NAMESPACE||'signal-desk-dev';
assert.ok(base&&app&&state,'Set RHOAI_URL, APP_URL and BROWSER_STATE');
const screenshot=async(page,name)=>{if(process.env.CAPTURE_GUIDE==='true')await page.screenshot({path:path.join(images,'signal-desk-playground-'+name+'.png'),fullPage:true})};
(async()=>{const browser=await chromium.launch({headless:true,args:['--use-mock-keychain']});
try{const context=await browser.newContext({storageState:state,viewport:{width:1440,height:1000}}),page=await context.newPage();
await page.goto(base+'/gen-ai-studio/playground/'+encodeURIComponent(namespace));
await expect(page.getByTestId('chatbot-message-bar')).toBeVisible({timeout:60000});
if(await page.getByTestId('whats-new-skip-tour').isVisible())await page.getByTestId('whats-new-skip-tour').click();
if(!await page.getByRole('tab',{name:'Model',exact:true}).count())await page.getByTestId('settings-button').click();
await page.getByTestId('subscription-selector-toggle').click();await page.getByText('Signal Desk runtime',{exact:true}).click();
await screenshot(page,'model');
await page.getByRole('tab',{name:'Prompt',exact:true}).click();
await page.getByTestId('system-instructions-input').fill(fs.readFileSync(root+'/content/modules/ROOT/assets/attachments/signal-desk-agent-prompt.txt','utf8'));
await page.getByTestId('system-instructions-input').evaluate(el=>{el.scrollTop=0});await screenshot(page,'prompt');
await expect.poll(async()=>{try{return(await context.request.get(app+'/healthz',{timeout:8000})).status()}catch{return 0}},{timeout:600000,intervals:[3000]}).toBe(200);
await page.getByRole('tab',{name:'MCP',exact:true}).click();
await expect(page.getByRole('row').filter({hasText:'Signal-Desk'})).toBeVisible({timeout:30000});
await screenshot(page,'mcp');
await page.getByRole('checkbox').check();
const connection=page.getByTestId('mcp-server-success-modal');
await expect(connection).toBeVisible({timeout:60000});
await expect(connection).toContainText('6 out of 6 tools are active.');
await expect(page.getByTestId('mcp-token-input')).toBeHidden();
await screenshot(page,'connected');
await connection.getByTestId('modal-submit-button').click();
await expect(page.getByRole('button',{name:'View tools for Signal-Desk',exact:true})).toContainText('6 active',{timeout:30000});
await expect(page.getByTestId('mcp-token-input')).toBeHidden();
console.log('Public MCP connected without a token.');

const tools=page.getByRole('button',{name:/View tools/i});
if(await tools.count()){
 await tools.click();for(const name of ['list_news','read_archive','search_archive','search_web','read_web','search_companies'])await expect(page.getByRole('dialog').getByText(name,{exact:true})).toBeVisible({timeout:15000});await screenshot(page,'tools');

 const dialog=page.getByRole('dialog');if(await dialog.count())await dialog.getByRole('button',{name:'Close',exact:true}).first().click();else await page.keyboard.press('Escape');
}
const probes=[{tool:'search_web',question:'Use the search_web tool exactly once with query "SK Hynix memory demand". Summarize two returned eligible headlines and their dates. Stop after this one tool call.'}];
const names=[],responses=[];let actualModel='';
for(const probe of probes){
 const stream=page.waitForResponse(r=>r.url().includes('/lsd/responses')&&r.request().method()==='POST',{timeout:180000});
 await page.getByTestId('chatbot-message-bar').fill(probe.question);await page.getByTestId('chatbot-message-bar').press('Enter');
 const response=await stream;assert.equal(response.status(),200);const raw=await response.text();if(process.env.RESPONSE_DEBUG_FILE)fs.writeFileSync(process.env.RESPONSE_DEBUG_FILE,raw,{mode:0o600});
 const events=raw.split('\n').filter(s=>s.startsWith('data: {')).map(s=>JSON.parse(s.slice(6))),done=events.findLast(e=>e.type==='response.completed')?.response;
 if(!done&&process.env.RESPONSE_DEBUG_FILE)fs.writeFileSync(process.env.RESPONSE_DEBUG_FILE,raw,{mode:0o600});
 assert.equal(done?.status,'completed');assert.equal(done.model,'maas-vllm-inference-1/deepseekv4pro');actualModel=done.model;const calls=done.output.filter(x=>x.type==='mcp_call');
 assert.equal(calls.length,1,'Component probe must execute exactly one tool');assert.equal(calls[0].name,probe.tool);assert.ok(!calls[0].error);
 names.push(probe.tool);responses.push(done.id);
}
await page.waitForTimeout(1000);await page.getByRole('tab',{name:'Model',exact:true}).click();
await screenshot(page,'answer');

const result={passed:true,project:namespace,model:actualModel,status:'completed',response_ids:responses,tools:names,mcp_authentication:'none',at:new Date().toISOString()};
fs.writeFileSync(root+'/e2e/results/simple-desk/playground-current.json',JSON.stringify(result,null,2)+'\n');console.log(JSON.stringify(result));

}finally{await browser.close()}})().catch(e=>{console.error(e.stack);process.exitCode=1});
