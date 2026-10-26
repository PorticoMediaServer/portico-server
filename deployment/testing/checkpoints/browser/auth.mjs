/** Surface-independent front door: the real signed-out form, then one sign-in
 * through the normal form. Web selects a profile; Hosted lands in its account
 * workspace. */
export async function signedOutLaunch(session){
 const {page,input}=session;
 return session.flow('signed-out-launch',async()=>{
  await page.goto(input.origin+'/',{waitUntil:'domcontentloaded'});
  await page.locator('input[name="username"]').waitFor();
  await page.locator('input[name="password"]').waitFor();
  if(await page.locator('[data-home-document]').count())throw new Error('signed-out launch rendered a signed-in document');
  return {form:'sign-in',passwordEmpty:!(await page.locator('input[name="password"]').inputValue())};
 });
}

export async function signIn(session){
 const {page,input}=session;
 return session.flow('sign-in',async()=>{
  session.markPopulatedCredentials();
  await page.locator('input[name="username"]').fill(input.username);
  await page.locator('input[name="password"]').fill(input.password);
  await page.locator('form button[type="submit"]').click();
  if(await page.locator('input[name="password"]').count()){
   await page.locator('input[name="password"]').waitFor({state:'hidden'});
  }
  if(input.surface==='web'){
   await page.waitForURL(url=>url.pathname==='/'||url.pathname==='/profiles');
   const selector=new URL(page.url()).pathname==='/profiles';
   if(selector)await page.getByRole('button').filter({hasText:input.profileName}).first().click();
   await page.locator('[data-home-document]').waitFor();
   return {profileSelector:selector,home:true};
  }
  await page.getByRole('heading').first().waitFor();
  return {accountWorkspace:true};
 });
}
