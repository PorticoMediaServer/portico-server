import {signedOutLaunch,signIn} from './auth.mjs';

/** Hosted Services account flows: the ordinary signed-in directory, account
 * page, security and device views, then sign-out. Ordinary account only. */
export async function runHosted(session){
 const {page,input}=session;
 await signedOutLaunch(session);
 const authenticated=await signIn(session);
 if(!authenticated){session.unavailable('authenticated-flows','sign-in failed or dropped');return;}

 await session.flow('account-directory',async()=>{
  await page.goto(input.origin+'/',{waitUntil:'domcontentloaded'});
  await page.locator('input[name="password"]').waitFor({state:'hidden'});
  await page.getByRole('heading').first().waitFor();
  if(await page.locator('a[href^="/library/"]').count())throw new Error('Hosted workspace exposed a server library');
 });

 await session.flow('account-management',async()=>{
  await page.goto(input.origin+'/account',{waitUntil:'domcontentloaded'});
  await page.getByRole('heading').first().waitFor();
  if(await page.locator('input[name="password"]').count())throw new Error('account page exposed a password control');
 });

 await session.flow('security-view',async()=>{
  await page.getByText('Security',{exact:true}).first().click();
  await page.getByText(/Password|Two-step|Two-factor|Authenticator/).first().waitFor();
 });

 await session.flow('devices-view',async()=>{
  await page.getByText('Devices',{exact:true}).first().click();
  await page.getByRole('heading').first().waitFor();
 });

 await session.flow('hosted-sign-out',async()=>{
  await page.getByRole('button',{name:/^Sign out/}).first().click();
  await page.locator('input[name="password"]').waitFor();
 });
}
