/** Diagnostic output contains structure and public copy, never captured credentials or capabilities. */
export function privacy(options) {
 const clean=value=>{
  let text=String(value);
  for(const value of options.privateValues??[])if(value)for(const item of new Set([value,value.toLowerCase(),value.toUpperCase(),value[0].toUpperCase()+value.slice(1)]))text=text.split(item).join('[private]');
  return text.replace(/https?:\/\/[^\s<>"']+/g,'[url]').replace(/\bBearer\s+[^\s<>"']+/gi,'[authorization]').replace(/\b[A-Za-z0-9_-]{32,}\b/g,'[opaque]').replace(/[0-9a-f]{8}-[0-9a-f-]{27,}/gi,'[id]');
 };
 if(options.kind==='text')return clean(options.value);
 let source=document;
 let privateValues=options.privateValues??[];
 let known,unknown;
 if(options.kind==='fixture'){
  source=document.implementation.createHTMLDocument('Diagnostic fixture');
  known='private-'+Math.random().toString(36).slice(2);
  unknown='capability-'+Math.random().toString(36).slice(2);
  source.body.innerHTML='<main id="portico-content" data-viewer="'+unknown+'"><form><input name="password" value="'+known+'"><img src="/v1/artwork/'+unknown+'?token='+unknown+'"><script>'+unknown+'</script><b>'+known+'</b></form></main>';
  privateValues=[known];
  options={...options,privateValues};
 }
 const root=source.documentElement.cloneNode(true);
 root.querySelectorAll('script,style,meta,link,template').forEach(node=>node.remove());
 const allowed=new Set(['class','role','type','name','placeholder','aria-invalid','aria-disabled','aria-expanded','alt']);
 for(const node of root.querySelectorAll('*')){
  for(const attribute of [...node.attributes])if(!allowed.has(attribute.name)&&!(attribute.name==='id'&&['root','portico-content'].includes(attribute.value)))node.removeAttribute(attribute.name);
  if(node.tagName==='TEXTAREA')node.textContent='';
 }
 const html=clean(root.outerHTML);
 if(options.kind==='fixture')return {knownRemoved:!html.includes(known),unknownRemoved:!html.includes(unknown),scriptRemoved:!html.includes('<script'),rootRetained:html.includes('portico-content'),inputRetained:html.includes('name="password"')};
 return html;
}

export function safePath(path) {
 if(path.startsWith('/assets/'))return path;
 const words=new Set(['v1','v2','system','server','networking','identity-proof','direct','sign-in','profiles','select','profile-switch','auth','refresh','me','sign-out','home','preferences','settings','sessions','events','stream','libraries','artwork','media','items','file-info','playback-options','detail','read','reading','queue','songs','albums','capabilities','notifications','portal','admin','console','personal','history','progress','servers','search','live','saved','player','account','help','dashboard','portico-build.json','browser-check.js']);
 return path.split('/').map(part=>!part||words.has(part)?part:':id').join('/');
}
