// An explicit public origin is used behind an HTTPS OpenShift route. Forwarded
// headers are not trusted to decide which origins can issue control requests.
export function allowedControlOrigin(origin,host,publicOrigin){
 if(!origin)return true;
 try{return origin===(publicOrigin?new URL(publicOrigin).origin:`http://${host}`)}catch{return false}
}
