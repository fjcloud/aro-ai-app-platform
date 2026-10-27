const $ = id => document.getElementById(id);
let stream, context, source, capture, recording = false, starting = false;
let quietSamples = 0, asking = false, history = [], speechTrace = '', speechSpan = '';
let frames = [], samples = 0, queue = [], sending = false, failed = false;
let activeLanguage = 'fr', rate = 16000, generation = 0;

function controls() {
  $('start').disabled = asking || recording || starting || sending || queue.length > 0;
  $('ask').disabled = $('new-conversation').disabled = $('question').disabled = $('start').disabled;
  $('stop').disabled = !recording;
  $('language').disabled = $('clear').disabled = $('start').disabled;
}

function wav(data, inputRate) {
  const ratio = inputRate / 16000, n = Math.floor(data.length / ratio);
  const buffer = new ArrayBuffer(44 + n * 2), view = new DataView(buffer);
  const text = (at, value) => [...value].forEach((c,i) => view.setUint8(at+i,c.charCodeAt(0)));
  text(0,'RIFF');view.setUint32(4,36+n*2,true);text(8,'WAVE');text(12,'fmt ');
  view.setUint32(16,16,true);view.setUint16(20,1,true);view.setUint16(22,1,true);
  view.setUint32(24,16000,true);view.setUint32(28,32000,true);view.setUint16(32,2,true);view.setUint16(34,16,true);
  text(36,'data');view.setUint32(40,n*2,true);
  for(let i=0;i<n;i++) {
    const start=Math.floor(i*ratio), end=Math.min(data.length,Math.floor((i+1)*ratio));
    let sum=0; for(let j=start;j<end;j++) sum+=data[j];
    const x=Math.max(-1,Math.min(1,sum/Math.max(1,end-start)));
    view.setInt16(44+i*2,x<0?x*32768:x*32767,true);
  }
  return new Blob([buffer],{type:'audio/wav'});
}

function append(text) {
  if (!text) return;
  const node=document.createElement('span');node.textContent=($('transcript').textContent?' ':'')+text;
  $('transcript').append(node);
  $('question').value = ($('question').value + ' ' + text).trim().slice(0,1000);
}

async function sendQueue() {
  if(sending) return;
  sending=true;controls();
  try {
    while(queue.length) {
      const clip=queue.shift(), form=new FormData();form.append('file',clip,'speech.wav');form.append('language',activeLanguage);
      const response=await fetch('/api/transcribe',{method:'POST',body:form,signal:AbortSignal.timeout(45000)});
      const result=await response.json();
      if(!response.ok) throw Error(result.error || 'Transcription failed');
      append(result.text);
      speechTrace=result.trace_id || '';speechSpan=result.span_id || '';
      $('status').textContent=`${recording?'Listening':'Finishing'} · last clip processed in ${(result.latency_ms/1000).toFixed(1)} s`;
    }
  } catch(error) {
    failed=true;queue=[];$('error').textContent=error.message || 'Could not reach the speech service';
    await stop(false);
  } finally {
    sending=false;
    if(!recording) $('status').textContent=failed?'Stopped. Check the error and try again.':'Stopped. Your microphone is off.';
    controls();
  }
}

function flush() {
  if(samples<rate*.15) {frames=[];samples=0;return;}
  const data=new Float32Array(samples);let at=0;
  for(const frame of frames) {data.set(frame,at);at+=frame.length;}
  frames=[];samples=0;
  // Skip quiet clips: Whisper can invent words when transcribing only silence.
  let energy=0;for(const value of data) energy+=value*value;
  if(Math.sqrt(energy/data.length)<.006) return;
  if(queue.length>=3) {failed=true;$('error').textContent='Speech service is falling behind. Stop and try again.';void stop(false);return;}
  queue.push(wav(data,rate));void sendQueue();
}

async function stop(flushTail=true) {
  recording=false;++generation;
  capture?.disconnect();source?.disconnect();stream?.getTracks().forEach(track=>track.stop());
  if(context && context.state!=='closed') await context.close();
  context=null;capture=null;source=null;stream=null;
  if(flushTail) flush();else {frames=[];samples=0;}
  $('status').textContent=sending||queue.length?'Finishing the last audio clips…':'Stopped. Your microphone is off.';controls();
}

$('start').onclick=async()=>{
  starting=true;failed=false;$('error').textContent='';controls();
  const current=++generation;
  try {
    if(!navigator.mediaDevices?.getUserMedia || !window.AudioWorkletNode) throw Error('Use an HTTPS page in a current Chrome, Edge, Firefox or Safari browser.');
    stream=await navigator.mediaDevices.getUserMedia({audio:{channelCount:1,echoCancellation:true,noiseSuppression:true}});
    if(generation!==current) {stream.getTracks().forEach(t=>t.stop());return;}
    context=new AudioContext({sampleRate:16000});rate=context.sampleRate;
    await context.audioWorklet.addModule('/capture.js');await context.resume();
    source=context.createMediaStreamSource(stream);capture=new AudioWorkletNode(context,'capture');
    activeLanguage=$('language').value;frames=[];samples=0;quietSamples=0;recording=true;
    capture.port.onmessage=({data})=>{
      if(!recording) return;
      frames.push(data);samples+=data.length;
      // Prefer a pause after 1 second; force a clip at 4 seconds.
      const quiet=data.reduce((total,value)=>total+value*value,0)/data.length < .000064;
      quietSamples=quiet?quietSamples+data.length:0;
      if(samples>=rate*4 || (samples>=rate && quietSamples>=rate*.2)) flush();
    };
    source.connect(capture);capture.connect(context.destination);
    $('status').textContent='Listening… speak into your microphone.';
  } catch(error) {
    failed=true;
    $('error').textContent=error.name==='NotAllowedError'?'Microphone access was denied. Allow it in your browser and try again.':error.message;
    await stop(false);
  } finally {starting=false;controls();}
};
$('stop').onclick=()=>void stop();
$('clear').onclick=()=>{$('transcript').replaceChildren();$('question').value='';speechTrace='';speechSpan='';$('error').textContent='';};
$('download').onclick=()=>{
  const url=URL.createObjectURL(new Blob([$('transcript').textContent+'\n'],{type:'text/plain;charset=utf-8'}));
  const a=document.createElement('a');a.href=url;a.download='transcript.txt';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);
};
window.addEventListener('pagehide',()=>{stream?.getTracks().forEach(t=>t.stop());void context?.close();});

$('new-conversation').onclick=()=>{
  history=[];speechTrace='';speechSpan='';$('conversation').replaceChildren();$('transcript').replaceChildren();$('question').value='';$('error').textContent='';
};
$('question-form').onsubmit=async event=>{
  event.preventDefault();
  const question=$('question').value.trim();
  if(!question || asking || recording || starting || sending || queue.length) return;
  asking=true;controls();$('error').textContent='';$('status').textContent='Consulting monitoring tools…';
  try {
    const response=await fetch('/api/chat',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({question,history,speech_trace:speechTrace,speech_span:speechSpan}),signal:AbortSignal.timeout(105000)});
    const result=await response.json();if(!response.ok) throw Error(result.error || 'The agent request failed');
    const article=document.createElement('article'), title=document.createElement('h2'), answer=document.createElement('p');
    title.textContent=question;answer.textContent=result.answer;article.append(title,answer);
    const details=document.createElement('details'), summary=document.createElement('summary'), evidence=document.createElement('pre');
    summary.textContent=`Monitoring evidence (${result.evidence.length} queries)`;evidence.textContent=JSON.stringify(result.evidence,null,2);details.append(summary,evidence);article.append(details);
    const trace=document.createElement('p');trace.className='trace';trace.textContent=`Trace: ${result.trace_id} · Prompt: ${result.prompt_version}`;article.append(trace);
    $('conversation').append(article);history.push({role:'user',content:question},{role:'assistant',content:result.answer.slice(0,1000)});history=history.slice(-8);while(new TextEncoder().encode(JSON.stringify(history)).length>16000) history.splice(0,2);
    $('transcript').replaceChildren();$('question').value='';speechTrace='';speechSpan='';$('status').textContent='Answer ready. Your microphone is off.';
  }catch(error){$('error').textContent=error.message || 'The cluster agent is unavailable';$('status').textContent='Question failed. Your microphone is off.';}
  finally {asking=false;controls();}
};
