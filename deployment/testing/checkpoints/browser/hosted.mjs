import {signedOutLaunch,signIn} from './auth.mjs';

const SECTIONS='Portico Account sections';

/** The drawn signed-in start surface for a fresh account: the invitation field
 * and its heading, never a server or library control. */
async function startSurface(page){
 await page.getByRole('heading',{name:'Join with an invitation'}).waitFor();
 await page.getByRole('textbox',{name:'Invitation link or code'}).waitFor();
}

/** Active section from the workspace navigation, or null. */
async function currentSection(page){
 const active=page.getByRole('navigation',{name:SECTIONS}).first().locator('[aria-current="page"]');
 if((await active.count())!==1)return null;
 return (await active.innerText()).trim();
}

/** Ordinary Hosted Services flows: the signed-in directory, the account page,
 * the security and device views, then sign-out. Ordinary account only; no
 * owner claim, approval or device-code path is exercised here. */
export async function runHosted(session){
 const {page,input}=session;
 await signedOutLaunch(session);
 const authenticated=await signIn(session);
 if(!authenticated){session.unavailable('authenticated-flows','sign-in failed or dropped');return;}

 await session.flow('account-directory',async()=>{
  await page.goto(input.origin+'/',{waitUntil:'domcontentloaded'});
  await page.locator('input[name="password"]').waitFor({state:'hidden'});
  await startSurface(page);
  if(await page.locator('a[href^="/library/"]').count())throw new Error('Hosted directory exposed a server library');
  return {startSurface:true,join:true,libraryLinks:0};
 });

 await session.flow('account-management',async()=>{
  await page.goto(input.origin+'/account',{waitUntil:'domcontentloaded'});
  await page.getByRole('heading',{name:'Portico Account'}).first().waitFor();
  await page.getByRole('navigation',{name:SECTIONS}).waitFor();
  await page.getByRole('heading',{name:'Devices'}).first().waitFor();
  if((await currentSection(page))!=='Devices')throw new Error('account page did not open its default section');
  if(await page.locator('input[name="password"]').count())throw new Error('account page exposed a password control');
  if(await page.getByRole('heading',{name:'Join with an invitation'}).count())throw new Error('account page fell back to the signed-in start surface');
  return {workspace:true,defaultSection:'Devices'};
 });

 await session.flow('security-view',async()=>{
  await page.goto(input.origin+'/account?tab=security',{waitUntil:'domcontentloaded'});
  await page.getByRole('heading',{name:'Security'}).first().waitFor();
  if((await currentSection(page))!=='Security')throw new Error('security section is not the active account section');
  await page.getByText('Password',{exact:true}).first().waitFor();
  await page.getByText('Two-step verification',{exact:true}).first().waitFor();
  const change=await page.getByRole('button',{name:'Change'}).count();
  if(change<1)throw new Error('security view has no change action');
  return {current:'Security',changeActions:change};
 });

 await session.flow('devices-view',async()=>{
  await page.goto(input.origin+'/account?tab=devices',{waitUntil:'domcontentloaded'});
  await page.getByRole('heading',{name:'Devices'}).first().waitFor();
  if((await currentSection(page))!=='Devices')throw new Error('devices section is not the active account section');
  await page.getByText('This browser',{exact:true}).first().waitFor();
  const others=page.getByRole('button',{name:'Sign out all other devices'});
  await others.waitFor();
  return {current:'Devices',thisBrowser:true,signOutOthers:await others.count()};
 });

 await session.flow('hosted-sign-out',async()=>{
  await page.goto(input.origin+'/account?tab=devices',{waitUntil:'domcontentloaded'});
  await page.getByRole('navigation',{name:SECTIONS}).getByRole('button',{name:'Sign out'}).click();
  const password=page.locator('input[name="password"]');
  await password.waitFor();
  if(await password.inputValue())throw new Error('sign-out left a populated password field');
  await page.goto(input.origin+'/account',{waitUntil:'domcontentloaded'});
  await password.waitFor();
  if(await page.getByRole('navigation',{name:SECTIONS}).count())throw new Error('signed-out account page still rendered the workspace');
  return {signedOut:true,accountKeptRequiringSignIn:true};
 });
}
