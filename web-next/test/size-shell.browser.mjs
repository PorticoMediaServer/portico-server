import assert from 'node:assert/strict';

// Uses a real browser + React DOM and real App/Shell/Home/PlaybackHost. Only the
// service boundary supplies synthetic data; this does not establish real sign-in.
export async function checkSizeShell(page, origin, onCase = async () => {}) {
  await page.setViewportSize({width: 599, height: 900});
  await page.goto(`${origin}/test/size-shell.html`, {waitUntil: 'networkidle'});
  await page.locator('[data-home-document] article[data-card-kind]').first().waitFor();
  await page.evaluate(async () => {
    await document.fonts.ready;
    const state = window.sizeShellProof;
    state.home = document.querySelector('[data-home-document]');
    state.main = document.querySelector('main');
    state.video = document.querySelector('video');
    state.snapshot = state.service.getSnapshot().document;
    state.initialViewer = state.viewer;
    // A locally generated WAV exercises the actual persistent media element.
    const bytes = new Uint8Array(44 + 8000 * 2 * 30), view = new DataView(bytes.buffer);
    const text = (at, value) => [...value].forEach((char, i) => {bytes[at + i] = char.charCodeAt(0);});
    text(0, 'RIFF'); view.setUint32(4, bytes.length - 8, true); text(8, 'WAVE'); text(12, 'fmt '); view.setUint32(16, 16, true);
    view.setUint16(20, 1, true); view.setUint16(22, 1, true); view.setUint32(24, 8000, true); view.setUint32(28, 16000, true); view.setUint16(32, 2, true); view.setUint16(34, 16, true); text(36, 'data'); view.setUint32(40, bytes.length - 44, true);
    state.mediaURL = URL.createObjectURL(new Blob([bytes], {type: 'audio/wav'}));
    state.video.src = state.mediaURL; state.video.muted = true; state.video.loop = true; await state.video.play();
    state.startTime = state.video.currentTime;
  });
  const inspect = () => page.evaluate(() => {
    const s = window.sizeShellProof, shell = document.querySelector('[data-navigation]'), video = document.querySelector('video');
    return {width: innerWidth, size: shell.dataset.sizeClass, navigation: shell.dataset.navigation, pointer: shell.dataset.pointer,
      sameHome: s.home === document.querySelector('[data-home-document]'), homeCount: document.querySelectorAll('[data-home-document]').length,
      sameMain: s.main === document.querySelector('main'), sameVideo: s.video === video, videoCount: document.querySelectorAll('video').length,
      sameSource: video.src === s.mediaURL, playing: !video.paused, time: video.currentTime,
      sameSession: s.viewer === s.initialViewer && s.bindings.shell.identity.viewer === s.initialViewer,
      sameDocument: s.service.getSnapshot().document === s.snapshot, proof: s.proof};
  });
  const results = [];
  for (const [width, size, navigation] of [[599,'compact','tabs'],[600,'medium','rail'],[599,'compact','tabs'],[600,'medium','rail'],[1023,'medium','rail'],[1024,'expanded','topbar'],[1023,'medium','rail'],[1024,'expanded','topbar']]) {
    await page.setViewportSize({width, height:900});
    await page.waitForFunction(expected => document.querySelector('[data-navigation]')?.dataset.navigation === expected, navigation);
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    const result = await inspect();
    assert.equal(result.size, size); assert.equal(result.navigation, navigation);
    for (const key of ['sameHome','sameMain','sameVideo','sameSource','sameSession','sameDocument','playing']) assert.equal(result[key],true,key);
    assert.equal(result.homeCount,1); assert.equal(result.videoCount,1);
    const dockLeft=await page.locator('[data-shell-dock]').evaluate(e=>e.getBoundingClientRect().left);
    assert.equal(dockLeft,navigation==='rail'?96:navigation==='tabs'?8:0);
    const firstRowTop=await page.locator('[data-home-document] section').first().evaluate(e=>e.getBoundingClientRect().top);
    if(navigation==='tabs'||navigation==='topbar') assert.ok(firstRowTop>=(navigation==='tabs'?56:76),'no-spotlight Home must clear fixed navigation');
    const nav = page.getByRole('navigation', {name:'Main',exact:true});
    assert.equal(await nav.getByRole('link',{name:'Home',exact:true}).getAttribute('aria-current'),'page');
    if (navigation === 'rail') {
      assert.deepEqual(await nav.locator('a').allTextContents(), ['Search','Home','Movies','Music','Live TV','Saved','Settings']);
    }
    results.push(result); await onCase({id:`${width}-${results.length}`,result});
  }
  // A pointer change by itself must switch chrome without a resize or remount.
  const cdp = await page.context().newCDPSession(page);
  for (const coarse of [true,false,true]) {
    await cdp.send('Emulation.setTouchEmulationEnabled',{enabled:coarse,maxTouchPoints:1});
    await page.waitForFunction(value => matchMedia('(pointer: coarse)').matches === value,coarse);
    await page.waitForFunction(expected => document.querySelector('[data-navigation]').dataset.navigation === expected,coarse?'sidebar':'topbar');
    const result=await inspect(); assert.equal(result.width,1024);assert.equal(result.size,'expanded'); assert.equal(result.sameHome,true);assert.equal(result.sameVideo,true);assert.equal(result.sameSession,true);
    results.push(result);await onCase({id:coarse?'expanded-coarse':'expanded-fine',result});
  }
  const nav = page.getByRole('navigation',{name:'Main',exact:true});
  await nav.getByRole('button',{name:'Collapse',exact:true}).click();
  await page.waitForFunction(()=>document.querySelector('[data-navigation]').dataset.navigation==='rail');
  await nav.getByRole('button',{name:'Expand',exact:true}).click();
  await page.waitForFunction(()=>document.querySelector('[data-navigation]').dataset.navigation==='sidebar');
  // Every touch action is visible, named, and at least 44 CSS pixels.
  for (const width of [599,600,1024]) {
    await page.setViewportSize({width,height:900});
    await page.waitForFunction(w=>innerWidth===w,width);
    const card=page.locator('article[data-card-kind]').first(), play=card.getByRole('button',{name:'Play',exact:true}), more=card.getByRole('button',{name:/More/});
    for (const button of [play,more]) {
      const box=await button.boundingBox();assert.ok(box&&box.width>=44&&box.height>=44);
      assert.equal(await button.evaluate(e=>getComputedStyle(e).opacity),'1');
    }
    await play.click();await more.click();
  }
  assert.equal((await inspect()).proof.plays,3); assert.equal((await inspect()).proof.more,3);
  // Preserve a real Home item/ref and the shelf offset across platform boundaries.
  await page.evaluate(()=>{
    const s=window.sizeShellProof; s.anchor=document.querySelectorAll('[data-layout-item]')[16];
    window.scrollBy(0,s.anchor.getBoundingClientRect().top-120);
    s.track=s.anchor.parentElement;s.track.scrollLeft=120;
  });
  await page.waitForTimeout(100);
  await page.evaluate(()=>{const s=window.sizeShellProof;s.firstVisible=[...s.track.children].find(e=>e.getBoundingClientRect().right>s.track.getBoundingClientRect().left)?.dataset.layoutItem;});
  const anchorBefore=await page.evaluate(()=>window.sizeShellProof.anchor.getBoundingClientRect().top);
  for (const width of [1023,600,599,600,1023,1024]) {
    await page.setViewportSize({width,height:900});await page.waitForTimeout(100);
    const value=await page.evaluate(()=>{const s=window.sizeShellProof;return {top:s.anchor.getBoundingClientRect().top,connected:s.anchor.isConnected,firstVisible:[...s.track.children].find(e=>e.getBoundingClientRect().right>s.track.getBoundingClientRect().left)?.dataset.layoutItem,expected:s.firstVisible};});
    assert.equal(value.connected,true);assert.ok(Math.abs(value.top-anchorBefore)<=2,JSON.stringify({width,anchorBefore,...value}));assert.equal(value.firstVisible,value.expected);
  }
  const final=await inspect();assert.equal(final.playing,true);assert.ok(final.time>0);assert.deepEqual(final.proof.errors,[]);assert.equal(final.proof.signOut,0);
  await page.evaluate(()=>{window.sizeShellProof.video.pause();window.sizeShellProof.video.removeAttribute('src');window.sizeShellProof.video.load();URL.revokeObjectURL(window.sizeShellProof.mediaURL);});
  await cdp.detach();
  return {results,final,anchorBefore,syntheticService:true,realSignInVerified:false};
}
