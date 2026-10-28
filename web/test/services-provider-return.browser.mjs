import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import path from 'node:path';

// Full built Services entry and canonical browser coordinator, with a fixture
// HTTP service. Provider exchange is not a real Google/Apple qualification.
export async function checkExistingProviderReturns({browser, webRoot, origin = 'https://web.getportico.tv'}) {
  const results=[];
  for (const provider of ['google','apple']) for (const status of ['verified','onboarding']) {
    const context=await browser.newContext(), page=await context.newPage();
    const transactionId='oid_'+'A'.repeat(43), familyId='provider-fixture-family';
    const transaction={transactionId,provider,mode:'login',status,expiresAt:new Date(Date.now()+60000).toISOString(),contactVerified:true,needsUsername:false,mfaRequired:false};
    const session={account:{id:'existing-account',username:'existing_user',displayName:'Existing user'},familyId,accessToken:'synthetic-access-token',expiresAt:new Date(Date.now()+900000).toISOString(),refreshExpiresAt:new Date(Date.now()+90*86400000).toISOString()};
    let completed=0;const errors=[];page.on('pageerror',e=>errors.push(e.message));
    try {
      await context.addInitScript(() => {
        const pictures = new Set(), paints = [];
        window.__providerReturnPaints = paints;
        const sample = () => {
          const boot = document.getElementById('boot');if (boot) pictures.add(boot);
          const paint = {boot: Boolean(boot), bootLeaving: boot?.hasAttribute('data-leaving') ?? false, signIn: Boolean(document.querySelector('input[name=password]')), pictures: pictures.size, loadingPictures: document.querySelectorAll('#boot, [data-provider-loading]').length, home: Boolean(document.querySelector('#join'))};
          if (JSON.stringify(paints.at(-1)) !== JSON.stringify(paint)) paints.push(paint);
        };
        new MutationObserver(sample).observe(document, {subtree:true,childList:true,attributes:true});sample();
      });
      await context.route(origin+'/**',async route=>{
        const request=route.request(),url=new URL(request.url());
        const json=body=>route.fulfill({contentType:'application/json',body:JSON.stringify(body),headers:{'X-Portico-CSRF-Cookie':'__Host-portico_refresh_csrf'}});
        if(url.pathname==='/v1/identity/options')return json({password:true,recovery:true,registration:true,verifiedContactRequired:true,accountSecurity:true,providers:[{id:provider,enabled:true,native:false,nativeClientIds:[]}]});
        if(url.pathname==='/v1/identity/transactions/'+transactionId+'/complete'){
          assert.equal(request.method(),'POST');assert.deepEqual(request.postDataJSON(),{});completed++;return json(session);
        }
        if(url.pathname==='/v1/identity/current'||url.pathname==='/v1/identity/transactions/'+transactionId)return json(transaction);
        if(url.pathname==='/v1/servers')return json({items:[],version:'0',watch:'fixture'});
        if(url.pathname.startsWith('/v1/'))return json(null);
        const name=path.extname(url.pathname)?url.pathname.slice(1):'index.html';
        try{return route.fulfill({body:await readFile(path.join(webRoot,name)),contentType:name.endsWith('.js')?'text/javascript':name.endsWith('.css')?'text/css':name.endsWith('.html')?'text/html':undefined});}
        catch{return route.fulfill({status:404,body:''});}
      });
      await page.goto(origin+'/account?identityTransaction='+transactionId);
      await page.waitForFunction(({origin,familyId})=>{
        const record=JSON.parse(localStorage.getItem('portico.account.v1:'+origin)||'null');return record?.active?.session?.familyId===familyId;
      },{origin,familyId},{timeout:10000});
      await page.locator('#join').waitFor({state:'visible'});
      const paints = await page.evaluate(() => window.__providerReturnPaints);
      assert.equal(paints.some(p => p.signIn), false, 'successful callback must never commit the signed-out form');
      assert.equal(Math.max(...paints.map(p => p.pictures)),1,'one static boot node for the whole return');
      assert.equal(paints.some(p => p.loadingPictures > 1),false,'no second loading picture behind the static boot');
      assert.equal(completed,1);assert.equal(new URL(page.url()).pathname,'/');assert.equal(new URL(page.url()).search,'');
      assert.equal(await page.locator('input[name=username]').count(),0);assert.deepEqual(errors,[]);
      results.push({provider,status,completed,canonicalFamily:true,paints});
    }finally{await context.close();}
  }
  return results;
}
