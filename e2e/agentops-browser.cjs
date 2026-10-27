// Live native Playground/MLflow checks. Credentials stay in private files.
// RHOAI_URL=https://rh-ai... BROWSER_STATE=/private/state.json MCP_TOKEN_FILE=/private/token \
// MLFLOW_TRACE_ID=<Go trace ID> node e2e/agentops-browser.cjs
const {chromium,expect}=require('@playwright/test');
const fs=require('node:fs');const assert=require('node:assert/strict');
(async()=>{
 const base=process.env.RHOAI_URL?.replace(/\/$/,''),state=process.env.BROWSER_STATE,tokenFile=process.env.MCP_TOKEN_FILE;
 const namespace=process.env.APP_NAMESPACE||'live-transcript-dev',server=process.env.MCP_SERVER_NAME||'OpenShift-Health';
 const traceId=process.env.MLFLOW_TRACE_ID?.replace(/^tr-/,'');
 if(!base||!state||!tokenFile||!traceId)throw Error('Set RHOAI_URL, BROWSER_STATE, MCP_TOKEN_FILE and MLFLOW_TRACE_ID');
 const browser=await chromium.launch({headless:true,args:['--use-mock-keychain']});
 try{
  const context=await browser.newContext({storageState:state}),page=await context.newPage();
  await page.goto(`${base}/gen-ai-studio/playground/${encodeURIComponent(namespace)}`,{waitUntil:'domcontentloaded'});
  await expect(page.getByTestId('chatbot-message-bar')).toBeVisible({timeout:60000});
  if(!await page.getByRole('tab',{name:'MCP',exact:true}).count())await page.getByTestId('settings-button').click();
  await page.getByRole('tab',{name:'MCP',exact:true}).click();
  const row=page.getByRole('row').filter({hasText:server});await row.getByRole('checkbox').check();
  await page.getByTestId('mcp-token-input').fill(fs.readFileSync(tokenFile,'utf8').trim());
  await page.getByTestId('mcp-token-authorize-button').click();
  await expect(page.getByText('Connection successful',{exact:true})).toBeVisible({timeout:30000});
  await page.getByTestId('modal-submit-button').click();
  await expect(row.getByRole('button',{name:`View tools for ${server}`,exact:true})).toContainText('3 active');
  await page.getByRole('tab',{name:'Model',exact:true}).click();
  await page.getByTestId('subscription-selector-toggle').click();await page.getByText('Cluster Voice runtime',{exact:true}).click();
  await page.getByTestId('chatbot-message-bar').fill('Use the MCP query_metric tool to query cpu_percent over 1h. Summarize the latest CPU value and range per node in at most 8 lines. Include percent units and the latest sample timestamp; keep raw samples in the tool result.');
  await page.getByTestId('chatbot-message-bar').press('Enter');
  await expect(page.locator('body')).toContainText('Tool response: query_metric',{timeout:110000});
  assert.equal(await page.getByRole('button',{name:'Danger alert details',exact:true}).count(),0,'Playground inference failed');
  await page.getByRole('button',{name:'Application launcher',exact:true}).click();
  const pagesBefore=context.pages().length;
  await page.getByText('MLflow',{exact:true}).click();
  await expect.poll(()=>context.pages().length,{timeout:15000}).toBeGreaterThan(pagesBefore);
  const mlflow=context.pages().at(-1);await mlflow.waitForLoadState('domcontentloaded');
  await expect(mlflow.locator('body')).toContainText(/Select workspace|cluster-voice/,{timeout:60000});
  if(await mlflow.getByText('Select workspace',{exact:true}).count())await mlflow.getByText(namespace,{exact:true}).click();
  await mlflow.getByText('cluster-voice',{exact:true}).first().click();
  await expect(mlflow.getByText('Token usage',{exact:true})).toBeVisible({timeout:30000});
  const response=await context.request.get(`${base}/mlflow/api/3.0/mlflow/traces/get?trace_id=tr-${traceId}`,{headers:{'X-MLFLOW-WORKSPACE':namespace}});
  assert.equal(response.status(),200,'Native MLflow trace read must succeed');
  const data=await response.json(),spans=data.trace.spans;
  const names=spans.map(s=>s.name);
  assert.ok(names.includes('cluster.question')&&names.includes('llm.chat')&&names.some(n=>n.startsWith('tool.')));
  const root=spans.find(s=>s.name==='cluster.question');assert.ok(root.links?.length,'Voice question must link to the speech trace');
  assert.ok(!JSON.stringify(data).includes('Authorization: Bearer'),'Credential must not appear in traces');
  console.log(JSON.stringify({passed:true,scope:'LIVE native Playground MCP + MaaS + MLflow UI and persisted Go trace',tools:3,playgroundMetric:'cpu_percent',mlflowTraceId:`tr-${traceId}`,spans:names,speechLink:true},null,2));
 }finally{await browser.close();}
})().catch(error=>{console.error(error.message);process.exitCode=1;});
