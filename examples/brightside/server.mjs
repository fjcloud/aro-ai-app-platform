import http from 'node:http';
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {NewsEngine} from './engine.mjs';
import {ResearchAgent} from './agent.mjs';
import {startD3Bridge} from './bridge.mjs';
import {allowedControlOrigin} from './http-security.mjs';
const root=path.dirname(fileURLToPath(import.meta.url));
const bridge=process.env.D3_KUBECONFIG?startD3Bridge({kubeconfig:process.env.D3_KUBECONFIG}):null;
const sources=JSON.parse(fs.readFileSync(path.join(root,'sources.json')));
const agent=new ResearchAgent({base:process.env.LLM_BASE_URL,key:process.env.LLM_API_KEY,model:process.env.LLM_MODEL||'deepseekv4pro'});
const engine=new NewsEngine({sources,agent,d3URL:process.env.D3_URL||'http://127.0.0.1:18123/v1/systemone',stateFile:process.env.STATE_FILE||path.join(root,'data/state.json')});
const clients=new Set();engine.on('state',state=>{const data='data: '+JSON.stringify(state)+'\n\n';for(const client of clients){if(client.writableLength>1024*1024){client.destroy();clients.delete(client)}else client.write(data)}});
const send=(res,status,value)=>{res.writeHead(status,{'Content-Type':'application/json','Cache-Control':'no-store'});res.end(JSON.stringify(value))};
const server=http.createServer(async(req,res)=>{
 try{
 const u=new URL(req.url,'http://localhost');res.setHeader('X-Content-Type-Options','nosniff');
 const routes=['/','/api/state','/api/events','/api/control','/api/trace','/healthz'];if(!routes.includes(u.pathname)){send(res,404,{error:'Not found'});return}
 const method=u.pathname==='/api/control'?'POST':'GET';if(req.method!==method){send(res,405,{error:'Method not allowed'});return}
 if(u.pathname==='/'){res.writeHead(200,{'Content-Type':'text/html; charset=utf-8','Cache-Control':'no-store','Content-Security-Policy':"default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'"});res.end(fs.readFileSync(path.join(root,'index.html')));return}
 if(u.pathname==='/healthz'){send(res,200,{status:'ok'});return}
 if(u.pathname==='/api/state'){send(res,200,engine.snapshot());return}
 if(u.pathname==='/api/trace'){const r=engine.records.get(u.searchParams.get('id'));if(!r){send(res,404,{error:'Unknown story'});return}send(res,200,{id:r.id,status:r.status,classification:r.classification,trace:r.trace,hold_reason:r.hold_reason});return}
 if(u.pathname==='/api/events'){res.writeHead(200,{'Content-Type':'text/event-stream','Cache-Control':'no-cache','Connection':'keep-alive','X-Accel-Buffering':'no'});res.write('data: '+JSON.stringify(engine.snapshot())+'\n\n');clients.add(res);const keep=setInterval(()=>res.write(': heartbeat\n\n'),15000);req.on('close',()=>{clearInterval(keep);clients.delete(res)});return}
 const origin=req.headers.origin;if(!allowedControlOrigin(origin,req.headers.host,process.env.PUBLIC_ORIGIN)){send(res,403,{error:'Unexpected request origin'});return}
 let body='';for await(const chunk of req){body+=chunk;if(body.length>2000){send(res,413,{error:'Request too large'});return}}
 let data;try{data=JSON.parse(body)}catch{send(res,400,{error:'Malformed JSON'});return}
 if(data.action==='play')engine.start();else if(data.action==='pause')engine.pause();else{send(res,400,{error:'Unknown action'});return}send(res,200,{running:engine.running});
 }catch{if(!res.headersSent)send(res,500,{error:'Request failed'});else res.end()}
});
const port=Number(process.env.PORT||18124),host=process.env.HOST||'127.0.0.1';
server.listen(port,host,()=>{console.log(`Brightside for Nora: http://${host}:${port}`);console.log(`Listening to ${sources.length} general-news publisher accounts. No positive-news feeds. No historical bootstrap.`);if(process.env.AUTO_START!=='0')engine.start()});
const shutdown=()=>{engine.close();bridge?.close();for(const client of clients)client.end();server.close(()=>process.exit(0));setTimeout(()=>process.exit(0),4000).unref()};process.on('SIGINT',shutdown);process.on('SIGTERM',shutdown);
