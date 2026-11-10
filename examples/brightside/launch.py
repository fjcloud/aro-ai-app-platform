"""Use the existing personal Foundry deployment; keys stay in process memory."""
import json,os,pathlib,subprocess,sys
root=pathlib.Path(__file__).resolve().parent
env=os.environ.copy()
if not env.get('LLM_API_KEY'):
 account=json.loads(subprocess.check_output(['az','cognitiveservices','account','show','-n','fjasignaldeskai40e9','-g','fja','-o','json']))
 keys=json.loads(subprocess.check_output(['az','cognitiveservices','account','keys','list','-n','fjasignaldeskai40e9','-g','fja','-o','json']))
 env['LLM_API_KEY']=keys['key1']
 env['LLM_BASE_URL']='https://'+account['properties']['customSubDomainName']+'.services.ai.azure.com/openai/v1'
 env.setdefault('LLM_MODEL','deepseekv4pro')
os.chdir(root)
os.execvpe('node',['node',sys.argv[1] if len(sys.argv)>1 else 'server.mjs'],env)
