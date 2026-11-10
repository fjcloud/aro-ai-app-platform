// Refresh identities for the explicitly configured publisher domains; never discover by sentiment.
import fs from 'node:fs';
const file=new URL('../sources.json',import.meta.url),sources=JSON.parse(fs.readFileSync(file));
const results=await Promise.all(sources.map(async source=>{const r=await fetch('https://public.api.bsky.app/xrpc/app.bsky.actor.getProfile?actor='+encodeURIComponent(source.handle),{signal:AbortSignal.timeout(15000)});if(!r.ok)throw Error('Could not resolve '+source.handle);const p=await r.json();if(p.handle!==source.handle||!p.did?.startsWith('did:'))throw Error('Unexpected identity for '+source.handle);return {handle:p.handle,did:p.did,name:p.displayName||source.name}}));
fs.writeFileSync(file,JSON.stringify(results,null,2)+'\n');console.log('Resolved '+results.length+' general-news publishers.');
