// npm install --no-save @playwright/test@1.56.1 && npx playwright install chromium
// APP_URL=https://... AUDIO_FIXTURE=/absolute/mono-16khz.wav node e2e/speech-browser.cjs
const {chromium, expect} = require('@playwright/test');
const path = require('node:path');
const fs = require('node:fs');
const assert = require('node:assert/strict');

(async () => {
  const url = process.env.APP_URL;
  const fixture = process.env.AUDIO_FIXTURE && path.resolve(process.env.AUDIO_FIXTURE);
  if (!url || !fixture || !fs.existsSync(fixture)) throw Error('Set APP_URL and AUDIO_FIXTURE (a spoken WAV fixture).');
  const language = process.env.SPEECH_LANGUAGE || 'fr';
  const expected = new RegExp(process.env.SPEECH_EXPECT || (language === 'fr' ? 'bonjour|transcription|openshift' : 'hello|transcription|openshift'), 'i');
  const args = ['--use-mock-keychain', '--use-fake-device-for-media-stream', `--use-file-for-fake-audio-capture=${fixture}%noloop`];
  const browser = await chromium.launch({headless: true, args: [...args, '--use-fake-ui-for-media-stream']});
  const result = {language, modelRequests: 0, latencyMs: []};
  try {
    const context = await browser.newContext({storageState: process.env.BROWSER_STATE || undefined, permissions: ['microphone', 'clipboard-read', 'clipboard-write'], viewport: {width: 1280, height: 900}});
    await context.addInitScript(() => {
      const get = navigator.mediaDevices.getUserMedia.bind(navigator.mediaDevices);
      navigator.mediaDevices.getUserMedia = async constraints => {
        const stream = await get(constraints);
        window.testMicTracks = stream.getTracks();
        return stream;
      };
    });
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    page.on('response', async response => {
      if (response.url().endsWith('/api/transcribe') && response.ok()) {
        const body = await response.json();
        result.modelRequests++;
        console.error(`Clip ${result.modelRequests}: ${body.latency_ms} ms — ${body.text}`);
        result.latencyMs.push(body.latency_ms);
      }
    });
    await page.goto(url);
    assert.equal(await page.evaluate(() => isSecureContext), true, 'microphone page must be a secure context');
    await page.getByLabel('Spoken language').selectOption(language);
    console.error('Browser loaded; starting microphone');
    const started = Date.now();
    await page.getByRole('button', {name: 'Start microphone', exact: true}).click();
    await expect(page.getByRole('button', {name: 'Stop', exact: true})).toBeEnabled({timeout: 10000});
    await expect(page.getByLabel('Spoken language')).toBeDisabled();
    await expect(page.getByLabel('Transcript', {exact: true})).toContainText(expected, {timeout: 90000});
    result.firstTextMs = Date.now() - started;
    console.error('Text is visible while recording');
    result.textWhileRecording = await page.getByLabel('Transcript', {exact: true}).innerText();
    await expect(page.getByRole('button', {name: 'Stop', exact: true})).toBeEnabled();
    // Let a complete short utterance finish, then exercise tail flushing.
    const recordMs = Number(process.env.SPEECH_RECORD_MS || 9000);
    await page.waitForTimeout(Math.max(0, recordMs - (Date.now() - started)));
    // More than one clip exercises independent WAV framing and ordered requests.
    await expect.poll(() => result.modelRequests, {timeout: 60000}).toBeGreaterThanOrEqual(2);
    await page.getByRole('button', {name: 'Stop', exact: true}).click();
    await expect(page.getByRole('button', {name: 'Start microphone', exact: true})).toBeEnabled({timeout: 90000});
    assert.equal(await page.evaluate(() => window.testMicTracks.every(track => track.readyState === 'ended')), true);
    const downloadPromise = page.waitForEvent('download');
    await page.getByRole('button', {name: 'Download transcript'}).click();
    const download = await downloadPromise;
    const stream = await download.createReadStream();
    let text = '';for await (const chunk of stream) text += chunk.toString();
    assert.match(text, expected);
    result.finalTranscript = text.trim();
    if (process.env.SPEECH_SCREENSHOT) await page.screenshot({path: process.env.SPEECH_SCREENSHOT, fullPage: true});
    await page.getByRole('button', {name: 'Clear', exact: true}).click();
    await expect(page.getByLabel('Transcript', {exact: true})).toBeEmpty();
    // A model outage must surface an error and release the microphone.
    await page.route('**/api/transcribe', route => route.fulfill({status: 502, contentType: 'application/json', body: '{"error":"Speech service is unavailable"}'}));
    await page.getByRole('button', {name: 'Start microphone', exact: true}).click();
    await expect(page.getByRole('alert')).toContainText('unavailable', {timeout: 20000});
    await expect(page.getByRole('button', {name: 'Start microphone', exact: true})).toBeEnabled({timeout: 10000});
    assert.equal(await page.evaluate(() => window.testMicTracks.every(track => track.readyState === 'ended')), true);
    assert.deepEqual(errors, []);
    result.microphoneReleased = true;
    result.outageHandled = true;
    await context.close();
  } finally {await browser.close();}
  const denied = await chromium.launch({headless: true, args: [...args, '--use-fake-ui-for-media-stream']});
  try {
    const context = await denied.newContext({storageState: process.env.BROWSER_STATE || undefined, permissions: ['microphone']});
    // Enforce a real browser denial through Permissions Policy; no OS prompt.
    await context.route(url, async route => {
      const response = await route.fetch();
      await route.fulfill({response, headers: {...response.headers(), 'permissions-policy': 'microphone=()'}});
    });
    const page = await context.newPage();await page.goto(url);
    await page.getByRole('button', {name: 'Start microphone', exact: true}).click();
    await expect(page.getByRole('alert')).toContainText('denied', {timeout: 15000});
    result.permissionDeniedHandled = true;
  } finally {await denied.close();}
  console.log(JSON.stringify(result, null, 2));
})().catch(error => {console.error(error);process.exitCode = 1;});
