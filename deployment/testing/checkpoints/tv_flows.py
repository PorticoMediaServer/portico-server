#!/usr/bin/env python3
"""Android TV and Fire OS M0 flows, driven only through android.py's authoritative facade.

Runs only against the exact Release-allocated emulator already installed and whose app data was
retained. It never clears data, signs in, enters a PIN, moves the clock, or changes lastGood,
snapshot or proof windows: a credential screen ends the flow as a truthful retention failure.

Focus is read only from the hierarchy's own `focused` state, never `selected`. Play needs this
package's media session in PLAYING with an advancing position plus the fixture title observed in
the session metadata or player chrome, never a changed image. When the authoritative tree, focus
state or media session cannot be read at all, the flow is Unavailable, not a pass and not a guess.

run_tv(device, flow, legacy, flavour) runs, in order:
  launch-restored-family, home-real-rows, movies-browse, fire-os-menu-back (firetv only),
  dpad-focus-walk, movie-detail, movie-playback-session, settings-open.

Owner integration, deliberately not applied in this file (android.py, native-checkpoint.py and
install.py stay untouched here):
  1. in android.py main(): tv=load('tv_flows',Path(__file__).with_name('tv_flows.py'))
  2. in android.py flow(): before `except Refusal`,
       except tv.Failure as e:record.update(status='fail',reason=str(e))
       except tv.Unavailable as e:record.update(status='unavailable',reason=str(e),dependency='authoritative hierarchy/focus/media read unavailable; no proof claimed')
     and after `def flow(...)`, attach
       flow.unavailable=lambda name,reason,dependency:flows.append({'surface':surface,'id':name,'status':'unavailable','reason':reason,'dependency':dependency})
     and, so pass receipts keep their counts, `value=fn()` then
       if isinstance(value,dict):record['evidence']=value
  3. in android.py main(): `if a.mode=='tv':tv.run_tv(d,flow,legacy,a.flavour)` and keep the
     existing phone path unchanged under `else`.
  4. in install.py: add 'tv_flows.py' beside 'android.py' in the native tool copy list.
"""
import re,time

PACKAGE='tv.getportico.portico'
FIXTURE='12 Angry Men'
NAV_LABELS={'Home','Movies','TV','Music','Books','Live','Search','Settings','Browse','Libraries'}
FIRE_ACTIONS=re.compile(r'(?:Play(?: next| from start)?|Resume|Add to [^,]*|Remove from [^,]*|Mark as [^,]*|Select|More(?: like this)?(?:…|\.\.\.)?|Go to [^,]*|Shuffle|Play all)',re.I)
SETTINGS_PAGES=('Profile & account','Playback','Audio & subtitles','Quality & data','Music','Appearance','Language & region','Privacy','This device','Account','Server')

class Failure(Exception):
    """A product-visible M0 gap: the flow did not do what the ruling requires."""

class Unavailable(Exception):
    """No authoritative hierarchy, focus state or media session could be read; never a pass."""

def _label(node):
    return str(node.get('label') or '').strip()

def _focused(nodes):
    return [node for node in nodes if node.get('focused') is True]

def _tree(device):
    try:
        return device.nodes()
    except Exception as error:
        if str(error)=='authoritative-tree-unavailable':
            raise Unavailable('authoritative-tree-unavailable') from None
        raise

def _visible(device):
    try:
        return device.text()
    except Exception as error:
        if str(error)=='credential-screen':
            raise Failure('retained-auth-ended-at-credential-screen') from None
        if str(error)=='authoritative-tree-unavailable':
            raise Unavailable('authoritative-tree-unavailable') from None
        raise

def _wait_text(device,pattern,timeout,reason):
    until=time.monotonic()+timeout
    while True:
        text=_visible(device)
        if re.search(pattern,text,re.I):
            return text
        if time.monotonic()>=until:
            raise Failure(reason)
        time.sleep(1)

def _wait_control(device,pattern,timeout,reason):
    until=time.monotonic()+timeout
    while True:
        for node in _tree(device):
            if node.get('editable') is not True and re.fullmatch(pattern,_label(node),re.I):
                return _label(node)
        if time.monotonic()>=until:
            raise Failure(reason)
        time.sleep(.6)

def _wait_focus(device,timeout=8):
    until=time.monotonic()+timeout
    while True:
        nodes=_tree(device)
        hits=_focused(nodes)
        if hits:
            return hits[0],nodes
        if time.monotonic()>=until:
            return None,nodes
        time.sleep(.25)

def _press(device,key,before,timeout=3.5):
    """Press one D-pad key and wait for the hierarchy to report a different focused control."""
    device.key(key);time.sleep(.15)
    until=time.monotonic()+timeout
    while True:
        hits=_focused(_tree(device))
        if hits and _label(hits[0])!=before:
            return hits[0]
        if time.monotonic()>=until:
            return None
        time.sleep(.2)

def _focus_control(device,pattern,directions,limit=6,timeout=6):
    node,_=_wait_focus(device,timeout)
    if node is not None and re.fullmatch(pattern,_label(node),re.I):
        return node
    for direction in directions:
        for _ in range(limit):
            before=_label(node) if node is not None else ''
            moved=_press(device,direction,before)
            if moved is None:
                break
            node=moved
            if re.fullmatch(pattern,_label(node),re.I):
                return node
    return None

def _activate(device,pattern,directions,reason):
    """D-pad activation when focus reaches it; otherwise the drawn control itself, never for Play."""
    node=_focus_control(device,pattern,directions)
    if node is not None:
        device.key('KEYCODE_DPAD_CENTER')
        return 'dpad-center'
    try:
        device.tap(pattern)
    except Exception as error:
        if str(error)=='unique-visible-control-unavailable':
            raise Failure(reason) from None
        raise
    return 'control-ref'

def _screen_title(device):
    """The player chrome's own title text, read from the hierarchy, not from pixels."""
    try:
        return FIXTURE.casefold() in _visible(device).casefold()
    except (Failure,Unavailable):
        return False

def _session(shell):
    """This package's media session state; None until Android lists one for the app."""
    raw,code=shell.shell('dumpsys','media_session',timeout=20)
    if code or not raw:
        return None
    block=[];inside=False
    for line in raw.splitlines():
        match=re.search(r'(?:^|\s)package=([A-Za-z0-9_.]+)',line)
        if match:
            if inside:
                break
            inside=match.group(1)==PACKAGE
            continue
        if inside:
            block.append(line)
    if not block:
        return None
    session={'state':None,'positionMs':None,'speed':None,'metadataTitle':None}
    for line in block:
        if 'PlaybackState' in line:
            match=re.search(r'state=(?:[A-Z_]+\((\d+)\)|(\d+))',line)
            if match:
                session['state']=int(match.group(1) or match.group(2))
            match=re.search(r'position=(-?\d+)',line)
            if match:
                session['positionMs']=int(match.group(1))
            match=re.search(r'speed=([0-9.]+)',line)
            if match:
                session['speed']=float(match.group(1))
        if 'metadata:' in line:
            match=re.search(r'(?:description|title)=([^,]+)',line)
            if match and match.group(1).strip().casefold() not in ('','null'):
                session['metadataTitle']=match.group(1).strip()
    return session

def _restore(device,shell,home):
    _,code=shell.shell('am','start','-W','-n',PACKAGE+'/.MainActivity',timeout=25)
    if code:
        raise Unavailable('launch-command-failed')
    try:
        _wait_text(device,home,25,'restored-app-did-not-draw-home')
    except Failure:
        if re.search(r'Sign in|Welcome home|Use a local server|Server address',_visible(device),re.I):
            raise Failure('retained-auth-ended-no-re-auth') from None
        raise

def _home(device,home):
    text=_wait_text(device,home,20,'home-row-not-drawn')
    rows=[name for name in ('Continue Watching','New on','On now','Listen') if name.casefold() in text.casefold()]
    if not rows:
        raise Failure('home-row-not-drawn')
    return {'homeRowsMatched':len(rows)}

def _browse(device,home):
    text=_visible(device);opened='already-open'
    if not re.search(r'\d+ movies',text,re.I):
        opened=_activate(device,r'Movies(?:,.*)?',('KEYCODE_DPAD_LEFT','KEYCODE_DPAD_DOWN','KEYCODE_DPAD_UP','KEYCODE_DPAD_RIGHT'),'movies-control-not-reachable')
    _wait_text(device,re.escape(FIXTURE),15,'fixture-title-absent-from-movies')
    text=_visible(device)
    if re.search(home,text,re.I) and not re.search(r'\d+ movies',text,re.I):
        raise Failure('movies-grid-did-not-open')
    count=re.search(r'(\d+) movies',text,re.I)
    return {'openedBy':opened,'movieCount':int(count.group(1)) if count else None}

def _detail(device):
    opened=_activate(device,r'12 Angry Men(?:,.*)?',('KEYCODE_DPAD_RIGHT','KEYCODE_DPAD_DOWN','KEYCODE_DPAD_LEFT','KEYCODE_DPAD_UP'),'fixture-card-not-reachable')
    _wait_control(device,r'(?:Play|Resume)(?:,.*)?',20,'detail-play-control-not-drawn')
    if FIXTURE.casefold() not in _visible(device).casefold():
        raise Failure('detail-title-does-not-match-requested-fixture')
    return {'openedBy':opened,'fixtureMatched':True,'playControlDrawn':True}

def _walk(device,start,key,limit=5):
    node=start;labels=[_label(start)];steps=0
    for _ in range(limit):
        moved=_press(device,key,_label(node))
        if moved is None:
            break
        node=moved;steps+=1;labels.append(_label(node))
    return node,labels,steps

def _focus_walk(device):
    node,nodes=_wait_focus(device,8)
    if not any('focused' in item for item in nodes):
        raise Unavailable('focus-attribute-absent-from-authoritative-tree')
    if node is None:
        raise Unavailable('no-focused-control-in-authoritative-tree')
    start=_label(node);walk=None;tried=0
    for forward,backward in (('KEYCODE_DPAD_RIGHT','KEYCODE_DPAD_LEFT'),('KEYCODE_DPAD_LEFT','KEYCODE_DPAD_RIGHT')):
        node,labels,steps=_walk(device,node,forward)
        distinct={label for label in labels if label}
        if steps>=3 and len(distinct)>=3:
            walk=(forward,backward,node,steps,len(distinct));break
        for _ in range(steps):
            moved=_press(device,backward,_label(node))
            if moved is None:
                raise Failure('dpad-focus-return-did-not-move')
            node=moved
        if _label(node)!=start:
            raise Failure('dpad-focus-return-mismatch')
        tried=max(tried,len(distinct))
    if walk is None:
        raise Failure('dpad-focus-reached-'+str(tried)+'-labelled-controls')
    forward,backward,node,steps,controls=walk
    for _ in range(steps):
        moved=_press(device,backward,_label(node))
        if moved is None:
            raise Failure('dpad-focus-return-did-not-move')
        node=moved
    if _label(node)!=start:
        raise Failure('dpad-focus-return-mismatch')
    return {'focusedControls':controls,'movesOut':steps,'returnedToStart':True}

def _playback(device,shell):
    node=_focus_control(device,r'(?:Play|Resume)(?:,.*)?',('KEYCODE_DPAD_DOWN','KEYCODE_DPAD_RIGHT','KEYCODE_DPAD_LEFT','KEYCODE_DPAD_UP'))
    if node is None:
        raise Failure('play-control-not-dpad-focusable')
    device.key('KEYCODE_DPAD_CENTER')
    until=time.monotonic()+35;playing=None;chrome=False;seen_session=False;readable_state=False
    while time.monotonic()<until:
        if not chrome:
            chrome=_screen_title(device)
        session=_session(shell)
        if session is not None:
            seen_session=True
            if session['state'] is not None:
                readable_state=True
        if session and session['state']==3 and (session['speed'] or 0)>0:
            playing=session
            if chrome or (session['metadataTitle'] and FIXTURE.casefold() in session['metadataTitle'].casefold()):
                break
        time.sleep(.6)
    if playing is None:
        if not seen_session:
            raise Unavailable('own-media-session-unreadable')
        if not readable_state:
            raise Unavailable('own-media-session-state-unreadable')
        raise Failure('own-media-session-not-playing')
    if playing['positionMs'] is None or playing['positionMs']<0:
        raise Unavailable('media-position-unavailable')
    if playing['metadataTitle']:
        if FIXTURE.casefold() not in playing['metadataTitle'].casefold():
            raise Failure('playback-target-changed')
        proof='media-session-metadata'
    elif chrome:
        proof='player-chrome-title'
    else:
        raise Failure('playback-target-not-observed')
    start=playing['positionMs']
    time.sleep(2.5)
    later=_session(shell)
    if later is None:
        raise Unavailable('own-media-session-unreadable-at-progress')
    if later['state'] is None:
        raise Unavailable('own-media-session-state-unreadable-at-progress')
    if later['state']!=3:
        raise Failure('media-session-left-playing')
    if (later['speed'] or 0)<=0:
        raise Failure('media-session-speed-not-positive')
    if later['positionMs'] is None or later['positionMs']<0:
        raise Unavailable('media-position-unavailable-at-progress')
    second_metadata=later['metadataTitle']
    if second_metadata and FIXTURE.casefold() not in second_metadata.casefold():
        raise Failure('playback-target-changed')
    if not (second_metadata and FIXTURE.casefold() in second_metadata.casefold()) and not _screen_title(device):
        raise Unavailable('fixture-identity-unavailable-at-progress')
    delta=later['positionMs']-start
    if delta<1500:
        raise Failure('media-position-not-advancing')
    return {'activation':'dpad-center','titleProof':proof,'fixtureMatched':True,'positionStartMs':start,'positionEndMs':later['positionMs'],'positionDeltaMs':delta,'speedAtProgress':later['speed']}

def _menu_back(device):
    node,nodes=_wait_focus(device,8)
    if node is None:
        raise Unavailable('no-focused-control-in-authoritative-tree')
    for _ in range(3):
        if _label(node) not in NAV_LABELS:
            break
        node=_press(device,'KEYCODE_DPAD_RIGHT',_label(node))
        if node is None:
            raise Failure('focused-content-card-not-reachable-from-nav')
    target=_label(node)
    nodes=_tree(device)
    before={_label(item) for item in nodes if _label(item)}
    device.key('KEYCODE_MENU')
    until=time.monotonic()+6;actions=None
    while time.monotonic()<until:
        current=_tree(device)
        actions={_label(item) for item in current if _label(item) and _label(item) not in before and FIRE_ACTIONS.fullmatch(_label(item))}
        if actions:
            break
        time.sleep(.4)
    if not actions:
        raise Failure('menu-context-not-observed')
    device.key('KEYCODE_BACK')
    until=time.monotonic()+6
    while time.monotonic()<until:
        current=_tree(device)
        labels={_label(item) for item in current if _label(item)}
        if not labels & actions:
            focused=[item for item in current if item.get('focused') is True]
            if focused and _label(focused[0])==target:
                return {'openedActions':len(actions),'backRestoredFocus':True}
        time.sleep(.4)
    raise Failure('menu-back-did-not-dismiss-or-restore-focus')

def _settings(device,home):
    device.key('KEYCODE_MEDIA_STOP')
    for _ in range(5):
        if re.search(home,_visible(device),re.I):
            break
        device.key('KEYCODE_BACK');time.sleep(1.5)
    else:
        raise Failure('home-not-restored-before-settings')
    opened=_activate(device,r'Settings(?:,.*)?',('KEYCODE_DPAD_LEFT','KEYCODE_DPAD_DOWN','KEYCODE_DPAD_UP','KEYCODE_DPAD_RIGHT'),'settings-control-not-reachable')
    _wait_text(device,r'\bSettings\b',15,'settings-screen-not-drawn')
    text=_visible(device)
    pages=[page for page in SETTINGS_PAGES if page.casefold() in text.casefold()]
    if not pages:
        raise Failure('settings-page-content-absent')
    return {'openedBy':opened,'pagesMatched':len(pages)}

def _fail_with(reason):
    def raise_it():
        raise Failure(reason)
    return raise_it

def run_tv(device,flow,legacy,flavour):
    if flavour not in ('google','firetv'):
        raise ValueError('unknown-tv-flavour')
    shell=legacy.Device(device.serial,str(device.private));home_rows=legacy.HOME_ROWS.pattern
    def skip(name,reason):
        mark=getattr(flow,'unavailable',None)
        if mark is not None:
            mark(name,reason,'earlier TV flow did not complete; no later evidence claimed')
        else:
            flow(name,_fail_with('dependency:'+reason))
    if flow('launch-restored-family',lambda:_restore(device,shell,home_rows)):
        home=flow('home-real-rows',lambda:_home(device,home_rows))
    else:
        home=False;skip('home-real-rows','retained launch did not reach the app')
    if home:
        browse=flow('movies-browse',lambda:_browse(device,home_rows))
    else:
        browse=False;skip('movies-browse','Home did not draw; no library entry to open')
    if not home:
        skip('remaining-core-flows','retained launch or Home failed; no reset, re-auth or clock fallback')
        return
    if flavour=='firetv':
        if browse:
            flow('fire-os-menu-back',lambda:_menu_back(device))
        else:
            skip('fire-os-menu-back','Movies grid did not open; no focused card to test')
    flow('dpad-focus-walk',lambda:_focus_walk(device))
    if browse:
        detail=flow('movie-detail',lambda:_detail(device))
    else:
        detail=False;skip('movie-detail','Movies grid did not open; no fixture to open')
    if detail:
        flow('movie-playback-session',lambda:_playback(device,shell))
    else:
        skip('movie-playback-session','detail did not open; the Play target cannot be proven')
    flow('settings-open',lambda:_settings(device,home_rows))
