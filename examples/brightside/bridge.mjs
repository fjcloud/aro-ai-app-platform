import net from 'node:net';
import {spawn} from 'node:child_process';

// Local development transport only. The app owns and restarts this child, not
// the cluster service. Existing externally managed port-forwards are preserved.
export function startD3Bridge({kubeconfig,port=18123,spawnProcess=spawn,checkPort=portOpen,interval=5000}){
 let child=null,stopped=false,checking=false;
 const tick=async()=>{
  if(stopped||checking||child)return;
  checking=true;
  try{
   if(await checkPort(port)||stopped)return;
   const proc=spawnProcess('oc',['--kubeconfig='+kubeconfig,'-n','decision-serving','port-forward','svc/d3-mini',`${port}:8080`,'--address=127.0.0.1'],{stdio:['ignore','ignore','pipe'],env:{PATH:process.env.PATH,HOME:process.env.HOME}});
   child=proc;proc.stderr?.on('data',()=>{});
   const clear=()=>{if(child===proc)child=null};proc.once('exit',clear);proc.once('error',clear);
  }finally{checking=false}
 };
 const timer=setInterval(tick,interval);timer.unref();void tick();
 return {close(){stopped=true;clearInterval(timer);child?.kill('SIGTERM');child=null},tick};
}
function portOpen(port){return new Promise(resolve=>{const socket=net.createConnection({host:'127.0.0.1',port});let done=false;const finish=value=>{if(done)return;done=true;socket.destroy();resolve(value)};socket.setTimeout(800);socket.once('connect',()=>finish(true));socket.once('error',()=>finish(false));socket.once('timeout',()=>finish(false))})}
