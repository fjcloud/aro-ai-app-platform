// Local protocol E2E: real Go server + Chromium microphone; simulated model/metrics.
// NODE_PATH=/path/to/node_modules AUDIO_FIXTURE=/path/to/spoken.wav node e2e/cluster-voice-browser.cjs
const {chromium,expect}=require('@playwright/test');
const assert=require('node:assert/strict');
const http=require('node:http');
const fs=require('node:fs');
const os=require('node:os');
const path=require('node:path');
const {spawn,spawnSync}=require('node:child_process');
const root=path.resolve(__dirname,'..');
const temp=fs.mkdtempSync(path.join(os.tmpdir(),'cluster-voice-e2e-'));
let app,browser,upstream;let calls={speech:0,model:0,queries:0};let failModel=false;
const listen=server=>new Promise(resolve=>server.listen(0,'127.0.0.1',()=>resolve(server.address().port)));
const json=(res,status,data)=>{res.writeHead(status,{'Content-Type':'application/json'});res.end(JSON.stringify(data));};
(async()=>{
 const fixture=process.env.AUDIO_FIXTURE;if(!fixture||!fs.existsSync(fixture))throw Error('Set AUDIO_FIXTURE to a spoken WAV fixture');
 const built=spawnSync('go',['build','-o',path.join(temp,'app'),'.'],{cwd:path.join(root,'examples/live-transcript'),encoding:'utf8'});assert.equal(built.status,0,built.stderr);
 fs.writeFileSync(path.join(temp,'key'),'runtime-key',{mode:0o600});
 upstream=http.createServer(async(req,res)=>{
  const chunks=[];for await(const chunk of req)chunks.push(chunk);const body=Buffer.concat(chunks);
  if(req.url==='/v1/audio/transcriptions'){
   calls.speech++;assert.ok(body.includes(Buffer.from('RIFF')));return json(res,200,{text:'What is the cluster CPU utilization?'});
  }
  if(req.url.startsWith('/api/v1/query')){
   calls.queries++;const q=new URL(req.url,'http://mock').searchParams;assert.match(q.get('query'),/node_cpu_seconds_total/);assert.equal(q.get('timeout'),'8s');
   const range=req.url.startsWith('/api/v1/query_range');
   return json(res,200,{status:'success',data:{resultType:range?'matrix':'vector',result:[{metric:{instance:'test-node'},...(range?{values:[[Math.floor(Date.now()/1000),'42']]}:{value:[Math.floor(Date.now()/1000),'42']})}]}});
  }
  if(req.url==='/v1/chat/completions'){
   calls.model++;assert.equal(req.headers.authorization,'Bearer runtime-key');if(failModel)return json(res,503,{error:'private upstream diagnostic'});
   const input=JSON.parse(body);const last=input.messages.at(-1);
   if(last.role==='tool'){
    assert.match(last.content,/'42'|"42"/);return json(res,200,{choices:[{message:{role:'assistant',content:'CPU is 42% (cpu_percent), based on the returned sample. <img src=x onerror=alert(1)>'}}],usage:{prompt_tokens:20,completion_tokens:10}});
   }
   assert.equal(input.tool_choice,'required');return json(res,200,{choices:[{message:{role:'assistant',tool_calls:[{id:'m1',type:'function',function:{name:'query_metric',arguments:JSON.stringify({metric:'cpu_percent',window:last.content.includes('hour')?'1h':''})}}]}}]});
  }
  json(res,404,{error:'unexpected fixture path'});
 });
 const upstreamPort=await listen(upstream),reserved=http.createServer(),appPort=await listen(reserved);await new Promise(resolve=>reserved.close(resolve));
 app=spawn(path.join(temp,'app'),[],{env:{...process.env,LISTEN_ADDR:`127.0.0.1:${appPort}`,WHISPER_URL:`http://127.0.0.1:${upstreamPort}/v1`,AGENT_BASE_URL:`http://127.0.0.1:${upstreamPort}/v1`,AGENT_KEY_FILE:path.join(temp,'key'),PROMETHEUS_URL:`http://127.0.0.1:${upstreamPort}`,SERVICE_ACCOUNT_TOKEN_FILE:path.join(temp,'key'),SERVICE_CA_FILE:'',OTEL_EXPORTER_OTLP_TRACES_ENDPOINT:''},stdio:['ignore','ignore','pipe']});
 const url=`http://127.0.0.1:${appPort}`;
 for(let i=0;i<50;i++){try{if((await fetch(url+'/healthz')).ok)break;}catch{};await new Promise(resolve=>setTimeout(resolve,100));}
 browser=await chromium.launch({headless:true,args:['--use-mock-keychain','--use-fake-ui-for-media-stream','--use-fake-device-for-media-stream',`--use-file-for-fake-audio-capture=${path.resolve(fixture)}%noloop`]});
 const context=await browser.newContext({permissions:['microphone']});
 await context.addInitScript(()=>{const original=navigator.mediaDevices.getUserMedia.bind(navigator.mediaDevices);navigator.mediaDevices.getUserMedia=async options=>{const stream=await original(options);window.testTracks=stream.getTracks();return stream;};});
 const page=await context.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto(url);await expect(page.getByRole('heading',{name:'Talk to your cluster.'})).toBeVisible();
 await page.getByLabel('Spoken language').selectOption('en');await page.getByRole('button',{name:'Start microphone',exact:true}).click();
 await expect(page.getByLabel('Transcript',{exact:true})).toContainText('CPU utilization',{timeout:30000});assert.equal(calls.model,0,'transcription must not auto-submit questions');
 await expect(page.getByRole('button',{name:'Ask the cluster',exact:true})).toBeDisabled();
 await page.getByRole('button',{name:'Stop',exact:true}).click();await expect(page.getByRole('button',{name:'Ask the cluster',exact:true})).toBeEnabled({timeout:15000});
 assert.ok(await page.evaluate(()=>window.testTracks.every(track=>track.readyState==='ended')));
 await page.getByRole('button',{name:'Ask the cluster',exact:true}).click();await expect(page.getByLabel('Conversation',{exact:true})).toContainText('CPU is 42%',{timeout:15000});
 await page.getByText('Monitoring evidence (1 queries)',{exact:true}).click();await expect(page.locator('#conversation pre')).toContainText('node_cpu_seconds_total');await expect(page.locator('#conversation pre')).toContainText('queried_at');
 assert.equal(await page.locator('#conversation img').count(),0,'model output must remain plain text');
 assert.match(await page.locator('#conversation .trace').innerText(),/Trace: [0-9a-f]{32}/);
 await page.getByLabel('Your question',{exact:false}).fill('Show CPU over the last hour');await page.getByRole('button',{name:'Ask the cluster',exact:true}).click();await expect(page.locator('#conversation article')).toHaveCount(2);await expect(page.getByRole('button',{name:'Ask the cluster',exact:true})).toBeEnabled();
 failModel=true;await page.getByLabel('Your question',{exact:false}).fill('Another CPU question');await page.getByRole('button',{name:'Ask the cluster',exact:true}).click();await expect(page.getByRole('alert')).toContainText('Model request rejected');assert.doesNotMatch(await page.getByRole('alert').innerText(),/private upstream diagnostic/);
 await page.getByRole('button',{name:'New conversation'}).click();await expect(page.getByLabel('Conversation',{exact:true})).toBeEmpty();
 assert.deepEqual(errors,[]);assert.ok(calls.speech>=1&&calls.model>=5&&calls.queries===2);
 console.log(JSON.stringify({passed:true,scope:'LOCAL: real browser and Go; simulated Whisper, MaaS and Thanos',calls,checks:['microphone capture','explicit submission','model tool loop','monitoring evidence','text follow-up','trace ID','safe DOM output','error redaction','new conversation']},null,2));
})().catch(error=>{console.error(error);process.exitCode=1;}).finally(async()=>{if(browser)await browser.close();if(app)app.kill('SIGTERM');if(upstream)await new Promise(resolve=>upstream.close(resolve));fs.rmSync(temp,{recursive:true,force:true});});
