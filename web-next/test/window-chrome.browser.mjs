import assert from 'node:assert/strict';

/** Real App/Shell regression. The caller owns its fixture browser and screenshot destination.
 * Native environment facts are explicitly synthetic; native caption/underlay proof is separate. */
export async function checkWindowChrome(browser, origin, capture = async () => {}) {
 const cases=[];
 await browser.resize(1440,900);await browser.navigate(origin + '/test/size-shell.html');
 await browser.evaluate(`(async()=>{const m=await import('/src/platform/window-chrome.ts'); window.chromeFixture={states:[],module:m};window.chromeFixture.host=m.installWindowChromeHost({update:state=>window.chromeFixture.states.push(state)});return true})()`);
 const read=()=>browser.evaluate(`(()=>{const nav=document.querySelector('[data-window-drag-region="page-topbar"]'),rail=document.querySelector('[data-window-drag-region="rail-empty"]'),root=getComputedStyle(document.documentElement);return{width:innerWidth,height:innerHeight,os:document.querySelector('[data-window-os]')?.dataset.windowOs,left:nav?getComputedStyle(nav).paddingLeft:null,right:nav?getComputedStyle(nav).paddingRight:null,rail:rail?.getBoundingClientRect().width,railTop:rail?getComputedStyle(rail).paddingTop:null,mark:!!document.querySelector('[class*="nativeRailMark"]'),clearanceLeft:root.getPropertyValue('--window-chrome-left-clearance').trim(),clearanceRight:root.getPropertyValue('--window-chrome-right-clearance').trim(),state:window.chromeFixture.states.at(-1),retained:window.sizeShellProof.proof.probeMounts,unmounted:window.sizeShellProof.proof.probeUnmounts,noDrag:!!nav?.querySelector('[data-window-no-drag]')}})()`);
 const set=async(os,fullscreen=false)=>{await browser.evaluate(`window.chromeFixture.host.updateEnvironment(${JSON.stringify({os,fullscreen})})`);await browser.evaluate('new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))');};
 let facts=await read();assert.equal(facts.left,'64px');assert.equal(facts.clearanceLeft,'');cases.push({name:'web-uninstalled',facts});
 await set('macos');facts=await read();assert.equal(facts.left,'104px');assert.equal(facts.right,'64px');assert.equal(facts.noDrag,true);cases.push({name:'mac-windowed',facts});
 await capture('mac-windowed', facts);
 await set('macos',true);facts=await read();assert.equal(facts.left,'64px');assert.equal(facts.state.fullscreen,true);cases.push({name:'mac-fullscreen',facts});
 await set('windows');facts=await read();assert.equal(facts.right,'150px');assert.equal(facts.left,'64px');cases.push({name:'windows-windowed',facts});
 await set('windows',true);facts=await read();assert.equal(facts.right,'64px');cases.push({name:'windows-fullscreen',facts});
 await set('linux');facts=await read();assert.equal(facts.left,'64px');assert.equal(facts.right,'64px');cases.push({name:'linux-windowed',facts});
 await browser.resize(720,900);await set('macos');facts=await read();assert.equal(facts.rail,84);assert.equal(facts.railTop,'48px');assert.equal(facts.mark,true);cases.push({name:'mac-medium',facts});
 await set('windows');await browser.evaluate('scrollTo(0,200)');await new Promise(r=>setTimeout(r,200));facts=await read();assert.equal(facts.state.scrolled,true);assert.equal(facts.state.surface,'pages');cases.push({name:'windows-medium-scrolled',facts});
 await browser.evaluate('window.chromeFixture.host.dispose()');await browser.resize(1440,900);await browser.evaluate('new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))');facts=await read();assert.equal(facts.left,'64px');assert.equal(facts.right,'64px');assert.equal(facts.unmounted,0);assert.equal(facts.retained,1);cases.push({name:'web-restored',facts});
 return {browser: browser.version, pass: true, cases, boundary: 'Actual App/Shell with synthetic services and native environment; no native caption/underlay or full-frame parity claim.'};
}
