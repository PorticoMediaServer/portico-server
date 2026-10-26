import {signedOutLaunch,signIn} from './auth.mjs';
import {publication,probePublicationLinks} from './reading.mjs';

async function home(page,input){
 await page.goto(input.origin+'/',{waitUntil:'domcontentloaded'});
 await page.locator('[data-home-document]').waitFor();
 await page.locator('[data-home-row] [data-card-kind]').first().waitFor();
 return page.locator('[data-home-row]').count();
}

async function countVisible(locator){
 return locator.evaluateAll(nodes=>nodes.filter(node=>{const rect=node.getBoundingClientRect();return rect.width>40&&rect.height>40&&rect.right>0&&rect.bottom>0&&rect.left<innerWidth&&rect.top<innerHeight;}).length);
}

/** Web and mobile-web core flows. Each flow proves its own real document: Home
 * rows, a grid and detail from the fixture library, actual media progress, the
 * paginator's own publication geometry, Settings and the sign-out form. */
export async function runWeb(session){
 const {page,input}=session;
 await signedOutLaunch(session);
 const authenticated=await signIn(session);
 if(!authenticated){session.unavailable('authenticated-flows','sign-in failed or dropped');return;}

 await session.flow('home-real-rows',async()=>{
  const rows=await home(page,input);
  const visibleCards=await countVisible(page.locator('[data-home-row] [data-card-kind]'));
  if(rows<1||visibleCards<1)throw new Error('home rows or cards absent');
  return {rows,visibleCards};
 });

 await session.flow('movies-browse-detail',async()=>{
  await home(page,input);
  await page.getByRole('link',{name:'Movies',exact:true}).click();
  const cards=page.locator('[data-card-kind]');
  await cards.first().waitFor();
  const browseCards=await cards.count();
  const button=cards.first().locator(':scope > button').first();
  const label=(await button.getAttribute('aria-label'))?.trim();
  if(!label)throw new Error('browse card has no accessible label');
  await button.click();
  await page.locator('[data-title-id]').waitFor();
  const heading=page.locator('[data-title-id] h1').first();
  await heading.waitFor();
  if(!(await heading.innerText()).trim())throw new Error('detail heading empty');
  const play=page.locator('[data-title-id]').getByRole('button').filter({hasText:/^(Play|Resume)/}).first();
  await play.waitFor();
  return {browseCards,detailTitle:true,playControl:true};
 });

 await session.flow('home-see-all',async()=>{
  await home(page,input);
  const row=page.locator('[data-home-row]:not([data-home-row="continue"]):not([data-home-row="continue_books"])')
   .filter({has:page.getByRole('button',{name:'See all'})}).first();
  await row.waitFor();
  const rowId=await row.getAttribute('data-home-row');
  await row.getByRole('button',{name:'See all'}).first().click();
  const expanded=page.locator('[data-home-row]').filter({has:page.getByRole('button',{name:'Home',exact:true})}).first();
  await expanded.waitFor();
  if(rowId&&(await expanded.getAttribute('data-home-row'))!==rowId)throw new Error('See all changed the row identity');
  const heading=expanded.getByRole('heading').first();
  await heading.waitFor();
  if(!(await heading.innerText()).trim())throw new Error('See all heading empty');
  const cards=expanded.locator('[data-card-kind]');
  await cards.first().waitFor();
  const visibleCards=await countVisible(cards);
  if(visibleCards<1)throw new Error('See all rendered no visible cards');
  await expanded.getByRole('button',{name:'Home',exact:true}).first().click();
  await page.locator('[data-home-document]').waitFor();
  return {rowIdentityPreserved:Boolean(rowId),visibleCards};
 });

 await session.flow('video-start-progress',async()=>{
  const play=page.locator('[data-title-id]').getByRole('button').filter({hasText:/^(Play|Resume)/}).first();
  await play.click();
  await page.waitForFunction(()=>[...document.querySelectorAll('video')].some(video=>video.currentTime>1.5&&!video.paused),{},{timeout:35000});
  const player=await page.evaluateHandle(()=>{
   const area=element=>{const rect=element.getBoundingClientRect();return rect.width*rect.height;};
   const videos=[...document.querySelectorAll('video')],playing=videos.filter(element=>!element.paused&&element.currentTime>0);
   return (playing.length?playing:videos).reduce((best,element)=>!best||area(element)>area(best)?element:best,undefined)??null;
  });
  const sample=()=>player.evaluate(element=>{
   if(!element||!element.isConnected)return null;
   const rect=element.getBoundingClientRect();
   return {currentTime:element.currentTime,duration:Number.isFinite(element.duration)?element.duration:0,readyState:element.readyState,error:Boolean(element.error),paused:element.paused,width:Math.round(rect.width),height:Math.round(rect.height)};
  });
  const first=await sample();
  if(!first)throw new Error('no video element');
  await page.waitForTimeout(2000);
  const second=await sample();
  if(!second||second.error||second.paused)throw new Error('video did not stay playing');
  if(!(second.readyState>=2)||!(second.duration>0))throw new Error('video has no decodable duration');
  if(!(second.width>=320&&second.height>=180))throw new Error('video surface has no media geometry');
  if(!(second.currentTime>first.currentTime+0.25))throw new Error('video progress did not advance');
  return {advancedSeconds:Math.round((second.currentTime-first.currentTime)*10)/10,durationSeconds:Math.round(second.duration*10)/10,readyState:second.readyState};
 });

 await session.flow('music-browse',async()=>{
  await home(page,input);
  await page.getByRole('link',{name:'Music',exact:true}).click();
  await page.locator('[data-card-kind]').first().waitFor();
  const visibleCards=await countVisible(page.locator('[data-card-kind]'));
  if(visibleCards<1)throw new Error('music browse has no visible cards');
  return {visibleCards};
 });

 await session.flow('reading-nonblank-no-egress',async()=>{
  session.external.length=0;
  await home(page,input);
  await page.getByRole('link',{name:'Books',exact:true}).click();
  await page.locator('[data-card-kind]').first().waitFor();
  await page.locator('[data-card-kind]').first().locator(':scope > button').first().click();
  await page.getByRole('button',{name:/^(Read|Continue reading|Start reading)/}).first().click();
  await page.waitForURL(/\/read\//);
  const result=await publication(page);
  session.setReading(result);
  if(!result.found)throw new Error('no paginator publication with visible text geometry');
  if(session.external.length)throw new Error('publication requested a foreign origin');
  if(await result.frameRef.evaluate(probePublicationLinks))throw new Error('publication frame exposes remote or responsive asset links');
  if(!/\/read\//.test(new URL(page.url()).pathname))throw new Error('reading route left');
  return {publication:{chars:result.publication.chars,lines:result.publication.lines,images:result.publication.images,textRects:result.publication.textRects},externalRequests:session.external.length};
 });

 await session.flow('settings-open',async()=>{
  await page.goto(input.origin+'/settings',{waitUntil:'domcontentloaded'});
  const heading=page.getByRole('heading').first();
  await heading.waitFor();
  if(!(await heading.innerText()).trim())throw new Error('settings heading empty');
  const controls=await page.locator('[role="switch"],select,input[type="range"],input[type="search"],input[type="text"]').evaluateAll(nodes=>nodes.filter(node=>{const rect=node.getBoundingClientRect();return rect.width>1&&rect.height>1&&rect.right>0&&rect.bottom>0&&rect.left<innerWidth&&rect.top<innerHeight;}).length);
  if(controls<1)throw new Error('settings has no visible control');
  return {controls};
 });

 await session.flow('profile-switch',async()=>{
  await page.goto(input.origin+'/profiles',{waitUntil:'domcontentloaded'});
  const profile=page.getByRole('button').filter({hasText:input.reviewProfile}).first();
  await profile.waitFor();await profile.click();
  await page.locator('[data-home-document]').waitFor();
  await page.locator('[data-home-row] [data-card-kind]').first().waitFor();
  return {home:true};
 });

 await session.flow('sign-out',async()=>{
  await home(page,input);
  await page.getByRole('button',{name:/^Switch profile or server(?:,|$)|, account menu$/}).click();
  await page.getByRole('menuitem',{name:/^Sign out of /}).click();
  const password=page.locator('input[name="password"]');
  await password.waitFor();
  if(await password.inputValue())throw new Error('sign-out left a populated password field');
  if(await page.locator('[data-home-document]').count())throw new Error('sign-out left the signed-in document');
  await page.goto(input.origin+'/',{waitUntil:'domcontentloaded'});
  await password.waitFor();
  return {signedOut:true,homeKeptRequiringSignIn:true};
 });
}
