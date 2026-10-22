"""Run generation and tool-call probes inside the model pod.

Set BENCH_BASE for the internal API and BENCH_LONG=1 for both 12k-token probes.
"""
import json, ssl, time, urllib.request, urllib.parse, os
base=os.environ.get('BENCH_BASE','https://127.0.0.1:8000/v1')
# vLLM uses an internal self-signed certificate on the AWS reference pod.
context=(ssl._create_unverified_context() if urllib.parse.urlparse(base).hostname in {"127.0.0.1", "localhost"}
         else ssl.create_default_context())
def post(payload):
    r=urllib.request.Request(base+'/chat/completions',data=json.dumps(payload).encode(),headers={'Content-Type':'application/json'})
    start=time.monotonic()
    with urllib.request.urlopen(r,context=context,timeout=180) as response:
        result=json.load(response)
    return result,round(time.monotonic()-start,3)
prompt={'model':'qwen3','messages':[{'role':'user','content':'Write exactly five short numbered tips for building a reliable Go HTTP service.'}],'temperature':0,'max_tokens':256,'chat_template_kwargs':{'enable_thinking':False}}
for i in range(3):
    d,secs=post(prompt)
    assert d["choices"][0]["message"].get("content"), d
    assert d["choices"][0]["finish_reason"] == "stop", d
    print(json.dumps({'case':'five-go-tips','run':i+1,'seconds':secs,'usage':d.get('usage'),'finish_reason':d['choices'][0]['finish_reason'],'content':d['choices'][0]['message'].get('content')}),flush=True)
tool={'model':'qwen3','messages':[{'role':'user','content':'Use the multiply tool to calculate 7 times 8.'}],'temperature':0,'max_tokens':256,'chat_template_kwargs':{'enable_thinking':False},'tools':[{'type':'function','function':{'name':'multiply','description':'Multiply two integers','parameters':{'type':'object','properties':{'a':{'type':'integer'},'b':{'type':'integer'}},'required':['a','b']}}}],'tool_choice':'auto'}
d,secs=post(tool)
calls=d['choices'][0]['message'].get('tool_calls',[])
assert calls, d
args=json.loads(calls[0]['function']['arguments'])
assert calls[0]['function']['name']=='multiply' and args['a']*args['b']==56,calls
print(json.dumps({'case':'tool-call','seconds':secs,'usage':d.get('usage'),'tool_calls':calls,'passed':True}),flush=True)
followup=dict(tool)
followup['messages']=tool['messages']+[d['choices'][0]['message'], {'role':'tool','tool_call_id':calls[0]['id'],'content':'56'}]
followup['tool_choice']='none'
d,secs=post(followup)
assert '56' in d['choices'][0]['message'].get('content',''),d
print(json.dumps({'case':'tool-result','seconds':secs,'usage':d.get('usage'),'content':d['choices'][0]['message']['content'],'passed':True}),flush=True)
if os.environ.get('BENCH_LONG')=='1':
    for verb,case in [('Read','long-prompt'),('Scan','long-prompt-warm-kernels-new-prefix')]:
        long_prompt=dict(prompt)
        # Changing the first token prevents reuse of the previous prompt's prefix cache.
        long_prompt['messages']=[{'role':'user','content':verb+' this filler:\n'+('red hat cloud workshop. '*2500)+'\nNow answer exactly GPU-CONTEXT-OK.'}]
        long_prompt['max_tokens']=64
        d,secs=post(long_prompt)
        assert 'GPU-CONTEXT-OK' in d['choices'][0]['message'].get('content',''),d
        print(json.dumps({'case':case,'seconds':secs,'usage':d.get('usage'),'passed':True}),flush=True)
