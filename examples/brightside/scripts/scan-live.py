import argparse,datetime,json,pathlib,time,urllib.request
root=pathlib.Path(__file__).resolve().parents[1]
parser=argparse.ArgumentParser();parser.add_argument('--seconds',type=int,default=150);args=parser.parse_args()
if not 1<=args.seconds<=3600:parser.error('Use a duration between 1 and 3600 seconds')
start=datetime.datetime.now(datetime.timezone.utc).isoformat().replace('+00:00','Z');deadline=time.monotonic()+args.seconds
api='http://127.0.0.1:18124/api/state'
baseline=json.load(urllib.request.urlopen(api,timeout=8));seen={};last_report=0;observations=[]
print(json.dumps({'scan_started_at':start,'connection':baseline['connection'],'baseline_received':baseline['stats']['received'],'baseline_classified':baseline['stats']['classified']}),flush=True)
while time.monotonic()<deadline:
 state=json.load(urllib.request.urlopen(api,timeout=8));disk=json.loads((root/'data/state.json').read_text())
 posts=[r for r in disk['records'] if r['received_at']>=start]
 for r in posts:
  key=(r['id'],r['cid']);status=r['status'];choice=r.get('classification',{}).get('choice')
  marker=(status,choice)
  if seen.get(key)!=marker:
   item={'id':r['id'],'source':r['source'],'received_at':r['received_at'],'status':status,'choice':choice,'classification_ms':r.get('classification',{}).get('http_ms')}
   observations.append(item);print(json.dumps(item,ensure_ascii=False),flush=True);seen[key]=marker
 if time.monotonic()-last_report>=30:
  print(json.dumps({'progress':True,'new_posts':len(posts),'classified':sum('classification' in r for r in posts),'positive':sum(r.get('classification',{}).get('choice')=='positive' for r in posts),'connection':state['connection'],'last_error':state['last_error']}),flush=True);last_report=time.monotonic()
 time.sleep(2)
state=json.load(urllib.request.urlopen(api,timeout=8));disk=json.loads((root/'data/state.json').read_text());posts=[r for r in disk['records'] if r['received_at']>=start]
result={'scan_started_at':start,'scan_finished_at':datetime.datetime.now(datetime.timezone.utc).isoformat().replace('+00:00','Z'),'mode':'Only new actual WebSocket arrivals observed after scan start; no historical records requeued','source_count':len(state['sources']),'received':len(posts),'classified':sum('classification' in r for r in posts),'positive':sum(r.get('classification',{}).get('choice')=='positive' for r in posts),'published':sum(r['status']=='published' for r in posts),'classifier_errors':sum(r['status']=='classifier_error' for r in posts),'pending':sum(r['status'] in ['queued','classifying','positive','researching'] for r in posts),'final_connection':state['connection'],'last_error':state['last_error'],'observations':observations,'posts':posts}
(root/'evidence/realtime-scan.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
print('SCAN_RESULT '+json.dumps({k:v for k,v in result.items() if k not in ['posts','observations']},ensure_ascii=False),flush=True)
