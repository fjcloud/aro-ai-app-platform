// Opt-in live local inference check. No fixture responses or forced signals.
const {chromium, expect} = require('@playwright/test');
const assert = require('node:assert/strict'), fs = require('node:fs'), path = require('node:path');
(async () => {
  const url = process.env.APP_URL || 'http://127.0.0.1:18092';
  assert.equal(new URL(url).hostname, '127.0.0.1', 'This check only targets the local development app');
  const browser = await chromium.launch({headless:true, args:['--use-mock-keychain']});
  try {
    const page = await browser.newPage({viewport:{width:1512,height:1050}}), errors=[];
    page.on('pageerror', e => errors.push(e.message));
    await page.goto(url, {waitUntil:'domcontentloaded'});
    const state = () => page.evaluate(async () => await (await fetch('/api/state')).json());
    let s = await state();
    assert.equal(s.preview, false); assert.ok(!s.validation_fixture);
    assert.ok(['groq','azure'].includes(s.inference.agent_backend));
    assert.equal(s.agent,s.inference.agent_backend==='azure'?'deepseekv4pro':'qwen/qwen3.8-27b');
    assert.equal(s.inference.decision_device, 'mps'); assert.equal(s.classifier, 'vllm-sr/d3-mini');
    const model = (await (await page.request.get('http://127.0.0.1:18093/v1/models')).json()).data[0];
    assert.equal(model.device, 'mps'); assert.equal(model.dtype, 'float16'); assert.equal(model.ready, true);
    if (!s.documents.length) await page.locator('#fetch').click();
    await expect.poll(async () => {
      s = await state();
	  const failed=s.documents.find(d=>d.status==='failed'||d.classifications.tech?.status==='failed');
	  if(failed)throw new Error('Live import failed: '+(failed.error||failed.classifications.tech?.error));
      return s.documents.length >= 3 && s.documents.every(d => d.status==='ready' && d.classifications.tech?.status==='done');
    }, {timeout:600000, intervals:[1000,3000,5000]}).toBe(true);
    await expect(page.locator('html')).toHaveAttribute('lang','en');
    await expect(page.locator('#preview')).toBeHidden();
    await expect(page.locator('#connection')).toContainText(s.inference.agent_backend==='azure'?'Azure Foundry cloud':'Groq cloud');
    await expect(page.locator('.story')).toHaveCount(s.documents.length);
    for (const d of s.documents) {
      assert.ok(d.summary.title && d.summary.tldr);
      assert.ok(['buy','short','no_signal'].includes(d.classifications.tech.priority));
      assert.ok(d.classifications.tech.input_tokens > 0);
    }
    const directional = s.documents.find(d => ['buy','short'].includes(d.classifications.tech.priority));
    let investigation = 'Not triggered: no directional signal in the real sample';
    if (directional) {
      await page.locator('.story[data-id="'+directional.id+'"]').getByRole('button',{name:'INVESTIGATE',exact:true}).click();
      await expect.poll(async () => (await state()).documents.find(d=>d.id===directional.id).research?.tech?.status,
        {timeout:600000,intervals:[1000,3000,5000]}).toBe('done');
      investigation = 'Completed on a real directional classification';
    }
    const neutral = s.documents.find(d=>d.classifications.tech.priority==='no_signal');
    if (neutral) {
      const res = await page.request.post(url+'/api/articles/'+neutral.id+'/investigate', {data:{profile_id:'tech'}});
      assert.equal(res.status(), 409, 'NO SIGNAL must remain gated');
    }
    await page.locator('.story').first().click();
    await page.setViewportSize({width:390,height:844});
    assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
    await page.setViewportSize({width:1512,height:1050}); assert.deepEqual(errors,[]);
    const dir = path.join(__dirname,'results/techmeme-terminal'); fs.mkdirSync(dir,{recursive:true});
    const prefix=s.inference.agent_backend==='azure'?'azure-live':'local-live';
    await page.screenshot({path:path.join(dir,prefix+'.png'),fullPage:true});
    const report = {passed:true,at:new Date().toISOString(),scope:'Actual local Go app, public Techmeme data, hosted research LLM and local d3-mini Metal; no inference fixtures',
      backend:s.inference.agent_backend,agent:s.agent,classifier:s.classifier,device:model.device,dtype:model.dtype,upstream_accelerator_validated:model.accelerator_validated,
      articles:s.documents.map(d=>({url:d.url,title:d.summary.title,coverage:d.source.coverage,signal:d.classifications.tech.priority,input_tokens:d.classifications.tech.input_tokens,latency_ms:d.classifications.tech.latency_ms})),
      investigation,checks:['English UI','live article summaries','actual Metal classifications','NO SIGNAL gate','mobile layout','no JavaScript errors','mock-keychain']};
    fs.writeFileSync(path.join(dir,prefix+'.json'),JSON.stringify(report,null,2)+'\n');
    console.log(JSON.stringify(report));
  } finally {await browser.close();}
})().catch(e=>{console.error(e.message);process.exitCode=1;});
