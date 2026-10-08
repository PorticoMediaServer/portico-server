/*
 * Portico Cast receiver.
 *
 * The page is served by the Portico server it talks to (and, unchanged, at
 * cast.getportico.tv for every server). The receiver framework script from
 * Google is the only external resource.
 *
 * Pairing: a sender asks its server for a one-use code (POST /v1/cast/bootstrap)
 * and delivers it over the `urn:x-cast:tv.getportico.cast` channel (a Cast device
 * has no keyboard); this page redeems it (POST /v1/cast/redeem) for two
 * credentials:
 *
 *   deviceToken  device-scoped, kept in localStorage, accepted only by
 *                POST /v1/cast/reconnect, rotated on every reconnect; used here
 *                only to renew an expired access token mid-session
 *   accessToken  an ordinary short-lived viewer bearer
 *
 * Control (SEC-14, CAST-04, D-FEAT-10): the sender that paired is the
 * controller. Only it may load, play, seek, change tracks or read status; any
 * sender may pause or stop (anyone in the room can do that with the remote).
 * A different sender that pairs while something is playing doesn't interrupt
 * it: the TV shows who is asking, the controller is asked, and the TV switches
 * only if the controller allows it, stops, or has left (then after 10 seconds).
 * The same Portico account on the same server switches straight away. The old
 * pairing is kept until the new one is adopted, and nothing is sent to other
 * senders but "busy".
 *
 * Sender protocol additions (all optional, backwards compatible):
 *   pair  {displayName}                         → paired | pair-pending | pair-failed {code: 'tv_busy' | …}
 *   load  {title, subtitle, seriesTitle, season, episode, kind, artworkPath}
 *   controller receives takeover-requested {requester} and replies takeover {allow}
 *   a replaced controller receives {type: 'replaced'}
 *
 * Playback uses Playback v1, the session contract every Portico client plays through:
 * POST /v1/playback/sessions, the presentation URL it answers with, timeline reports
 * (progress and lease renewal), PATCH for a seek the server must produce, DELETE to end.
 * CAF owns media loading, its standard player draws playback (V-11), and the
 * overlays in index.html cover idle, preparing, errors and takeover requests.
 * Pairing credentials never enter media URLs or sender messages.
 */
(function () {
  'use strict';

  var STORAGE_KEY = 'portico.cast.deviceToken';
  var PROTOCOL = '1.0';
  var PROGRESS_INTERVAL_MS = 10000;
  var NAMESPACE = 'urn:x-cast:tv.getportico.cast';
  var ORIGIN_KEY = 'portico.cast.serverOrigin';
  var TAKEOVER_ASK_MS = 30000;     // controller present: no answer means no
  var TAKEOVER_ABSENT_MS = 10000;  // controller gone: the TV says who is taking over, then switches
  var ERROR_VISIBLE_MS = 20000;

  // Which Portico server this receiver talks to. Served by a server, that is the server itself.
  // Served from the shared page at cast.getportico.tv, the sender names the viewer's server when
  // it pairs; only an HTTPS origin is accepted, because a Cast device will not load anything else.
  var base = '';
  function serverOrigin(value) {
    if (!value) { return ''; }
    try {
      var url = new URL(String(value));
      if (url.protocol !== 'https:' || url.username || url.password) { return null; }
      return url.origin === window.location.origin ? '' : url.origin;
    } catch (e) { return null; }
  }
  try { base = serverOrigin(window.localStorage.getItem(ORIGIN_KEY)) || ''; } catch (e) { base = ''; }
  var broadcast = function () {};

  // --- Presentation ---------------------------------------------------------

  function $(id) { return document.getElementById(id); }
  var ui = $('ui');
  var status = $('status');
  var errorTimer = null, noticeTimer = null;

  function show(state) { if (ui && ui.setAttribute) ui.setAttribute('data-state', state); }
  function text(id, value) { var el = $(id); if (el) el.textContent = value || ''; }
  function visible(id, on) { var el = $(id); if (el) el.hidden = !on; }
  function say(message, tone) {
    if (!status) return;
    status.textContent = message || '';
    if (tone) { status.setAttribute('data-tone', tone); } else { status.removeAttribute('data-tone'); }
  }
  /** A short notice over whatever is on screen ("Reconnecting…"). */
  function notice(message, ms) {
    clearTimeout(noticeTimer);
    text('notice', message); visible('notice', !!message);
    if (message && ms) noticeTimer = setTimeout(function () { visible('notice', false); }, ms);
  }
  function showIdle() { clearTimeout(errorTimer); show('idle'); }
  function showLoading(meta) {
    clearTimeout(errorTimer);
    text('loading-eyebrow', 'Getting ready');
    text('loading-title', meta && meta.title || '');
    text('loading-subtitle', meta && (meta.subtitle || meta.seriesTitle) || '');
    visible('loading-art', false);
    show('loading');
  }
  function showArtwork(url, square) {
    var art = $('loading-art');
    if (!art || !url) return;
    art.src = url;
    if (art.classList) art.classList.toggle('square', !!square);
    art.hidden = false;
  }
  function showPlaying() { clearTimeout(errorTimer); show('playing'); }
  var errorCopy = {
    unavailable: ['This title isn’t available right now', 'It may have been removed, or your access may have changed.'],
    busy: ['Your server is busy right now', 'Try again in a moment from your phone or computer.'],
    unreachable: ['This TV can’t reach your server', 'Check that your server is on and that this TV is on the same network, then try again.'],
    decode: ['This title couldn’t play on this TV', 'Try again from your phone or computer, or choose a lower quality.'],
    generic: ['This title couldn’t play on this TV', 'Try again from your phone or computer.'],
  };
  function errorKind(code) {
    code = String(code || '');
    if (/not_found|unavailable|forbidden|permission/.test(code)) return 'unavailable';
    if (/busy|capacity|cap$|occupied|timeout/.test(code)) return 'busy';
    if (/network|fetch|http_5|unreach|abort/.test(code)) return 'unreachable';
    if (/decode|stalled|playback_failed/.test(code)) return 'decode';
    return 'generic';
  }
  /** A visible error card: the TV always says what went wrong (CAST-01). Never shows raw codes. */
  function showError(code) {
    var copy = errorCopy[errorKind(code)];
    text('error-title', copy[0]); text('error-body', copy[1]);
    show('error');
    clearTimeout(errorTimer);
    errorTimer = setTimeout(function () { if (!playback) showIdle(); }, ERROR_VISIBLE_MS);
  }

  // --- Credentials and requests --------------------------------------------

  var session = null; // {accessToken, expiresAt}
  var scope = null;   // {accountId, profileId, authority, capabilities}
  // The Playback v1 session playing here: {id, revision, itemId, generation, serverSeek}.
  var playback = null;
  var manager = null, castContext = null, profiled = false;
  var reportTimer = null, reportSeq = 0, reportFailures = 0, recoveries = 0, recovering = false, bufferingTimer = null;
  var commands = Promise.resolve();
  function serial(work) { var result = commands.then(work); commands = result.catch(function () {}); return result; }
  function token(bytes) { var a = new Uint8Array(bytes || 24); window.crypto.getRandomValues(a); return Array.from(a, function (x) { return x.toString(16).padStart(2, '0'); }).join(''); }
  function position() { return manager ? manager.getCurrentTimeSec() || 0 : 0; }
  function paused() { return !manager || manager.getPlayerState() === 'PAUSED'; }
  function sessionPath(id) { return '/v1/playback/sessions/' + encodeURIComponent(id); }
  function wait(ms) { return new Promise(function (resolve) { setTimeout(resolve, ms); }); }

  function readToken() {
    try { return window.localStorage.getItem(STORAGE_KEY) || ''; } catch (e) { return ''; }
  }
  function writeToken(value) {
    try {
      if (value) { window.localStorage.setItem(STORAGE_KEY, value); }
      else { window.localStorage.removeItem(STORAGE_KEY); }
    } catch (e) { /* a receiver with storage disabled simply re-pairs each boot */ }
  }

  // deviceId is stable for the life of the stored pairing. A receiver that has
  // been unpaired generates a new one, so an old device row is never reused.
  function deviceId() {
    var key = 'portico.cast.deviceId';
    var value = '';
    try { value = window.localStorage.getItem(key) || ''; } catch (e) { /* ignore */ }
    if (!value) {
      var bytes = new Uint8Array(16);
      (window.crypto || window.msCrypto).getRandomValues(bytes);
      value = 'cast-';
      for (var i = 0; i < bytes.length; i++) { value += bytes[i].toString(16).padStart(2, '0'); }
      try { window.localStorage.setItem(key, value); } catch (e) { /* ignore */ }
    }
    return value;
  }

  /** One request, bounded in time and size. Resolves {status, body} for 2xx; throws with
   * {code, status, current} otherwise (`current` is a 412's session). */
  async function request(path, options) {
    var headers = Object.assign({'Content-Type': 'application/json'}, options.headers || {});
    if (options.bearer) headers.Authorization = 'Bearer ' + options.bearer;
    var abort = new AbortController(), timer = setTimeout(function () { abort.abort(); }, 15000);
    try {
      var response = await fetch((options.origin === undefined ? base : options.origin) + path, {method: options.method || 'POST', headers: headers, signal: abort.signal, body: options.body === undefined ? undefined : JSON.stringify(options.body)});
      var text = '';
      if (response.body) {
        var reader = response.body.getReader(), decoder = new TextDecoder(), size = 0;
        try { for (;;) { var chunk = await reader.read(); if (chunk.done) break; size += chunk.value.byteLength; if (size > 1048576) throw new Error('response_too_large'); text += decoder.decode(chunk.value, {stream: true}); } text += decoder.decode(); } finally { reader.releaseLock(); }
      }
      var payload = text ? JSON.parse(text) : {};
      if (!response.ok) {
        var failure = payload.error || {};
        var error = new Error(failure.code || 'http_' + response.status); error.code = error.message; error.status = response.status; error.current = failure.current;
        throw error;
      }
      return {status: response.status, body: payload};
    } finally { clearTimeout(timer); abort.abort(); }
  }
  function post(path, body, bearer, origin) { return request(path, {body: body, bearer: bearer, origin: origin}).then(function (r) { return r.body; }); }
  /** A request with the receiver's bearer. An expired bearer is renewed once through the device
   * token; a transport failure or 5xx is retried once with the identical request (the same
   * Idempotency-Key, If-Match or timeline sequence), which the server treats as the same request. */
  async function api(path, options) {
    options = options || {};
    var attempt = function () { return request(path, Object.assign({}, options, {bearer: session.accessToken})); };
    try { return await attempt(); }
    catch (e) {
      if (e.status === 401 && readToken()) {
        var renewed = await post('/v1/cast/reconnect', {protocolVersion: PROTOCOL, deviceToken: readToken(), deviceId: deviceId()});
        if (!renewed.scope || renewed.scope.accountId !== scope.accountId || renewed.scope.profileId !== scope.profileId) throw new Error('pairing_scope_changed');
        session = renewed.session; scope = renewed.scope; writeToken(renewed.deviceToken);
        return attempt();
      }
      if (e.status && e.status < 500) throw e;
      return attempt();
    }
  }
  function buildProfile(context) {
    var caps = context ? context.getDeviceCapabilities() || {} : {};
    function supports(codec, width, height, fps) { try { var parts=codec.split('; codecs=');return !!context && context.canDisplayType(parts[0],parts[1]?parts[1].replace(/"/g,''):'',width,height,fps); } catch (_) { return false; } }
    var fourK = supports('video/mp4; codecs="hev1.2.4.L153.B0"', 3840, 2160, 60);
    var ranges = ['sdr'];
    if (caps.is_hdr_supported) ranges.push('hdr10', 'hlg');
    if (caps.is_dv_supported) ranges.push('dolby_vision');
    var width = fourK ? 3840 : 1920, height = fourK ? 2160 : 1080;
    // Every Chromecast since the 2nd generation decodes H.264 High@4.2 1080p60; probe it rather than assume 30 fps (CAST-08).
    var h264At60 = supports('video/mp4; codecs="avc1.64002A"', 1920, 1080, 60);
    var video = [{codec:'h264',bitDepths:[8],maxWidth:1920,maxHeight:1080,maxLevel:h264At60?42:41,maxFrameRate:h264At60?60:30,dynamicRanges:['sdr'],evidence:h264At60?'probed':'declared'},{codec:'vp8',bitDepths:[8],maxWidth:1920,maxHeight:1080,maxFrameRate:30,dynamicRanges:['sdr'],evidence:'declared'}];
    [['hevc','hev1.2.4.L153.B0'],['vp9','vp09.02.10.10'],['av1','av01.0.08M.10']].forEach(function (pair) {
      if (supports('video/mp4; codecs="' + pair[1] + '"', width, height, 60)) video.push({codec:pair[0],bitDepths:[8,10],maxWidth:width,maxHeight:height,maxFrameRate:60,dynamicRanges:ranges.filter(function (x) {return x !== 'dolby_vision' || pair[0] === 'hevc';}),dolbyVisionProfiles:pair[0] === 'hevc' && caps.is_dv_supported ? [5,8] : [],evidence:'probed'});
    });
    var audio = ['aac','mp3','opus','vorbis','flac'].map(function(codec){return {codec:codec,maxChannels:2,evidence:'declared'};});
    [['ac3','ac-3'],['eac3','ec-3']].forEach(function (pair) { if (supports('audio/mp4; codecs="' + pair[1] + '"')) audio.push({codec:pair[0],maxChannels:6,passthrough:true,evidence:'probed'}); });
    var surround=audio.some(function(a){return a.codec==='ac3'||a.codec==='eac3';});
    var hlsAudio=audio.filter(function(a){return ['aac','mp3','ac3','eac3'].indexOf(a.codec)>=0;}).map(function(a){return a.codec;});
    var names=video.map(function (v) {return v.codec;});
    return {version:1,evidence:'mixed',client:{family:'cast',platform:'cast',engine:'caf-shaka',app:'portico-cast',model:fourK?'uhd_probed':'hd_probed'},display:{width:width,height:height,maxFrameRate:fourK||h264At60?60:30,dynamicRanges:ranges},video:video,audio:audio,audioOutput:{route:'hdmi',maxChannels:surround?6:2},transports:[{transport:'direct',containers:['mp4','m4v'],video:names.filter(function (c) {return c!=='vp9'&&c!=='vp8';}),audio:hlsAudio},{transport:'direct',containers:['webm'],video:names.filter(function(c){return c==='vp8'||c==='vp9'||c==='av1';}),audio:['opus','vorbis']},{transport:'direct',containers:['mp3','m4a','aac','flac','ogg','wav'],video:[],audio:['aac','mp3','opus','vorbis','flac']},{transport:'hls_ts',containers:['mpegts'],video:['h264'],audio:hlsAudio},{transport:'hls_fmp4',containers:['fmp4'],video:names.filter(function (c) {return c!=='vp9'&&c!=='vp8';}),audio:hlsAudio}],subtitles:{text:['vtt'],styled:[],bitmap:[]}};
  }
  /** What this TV decodes, published for the session's device before anything plays. */
  async function prepareProfile() {
    if (profiled) return;
    var context = window.cast && cast.framework.CastReceiverContext.getInstance();
    await api('/v1/playback/client-profile', {method: 'PUT', body: buildProfile(context)});
    profiled = true;
  }

  function adopt(result) {
    session = result.session;
    scope = result.scope;
    writeToken(result.deviceToken);
    profiled = false;
    say('');
    return prepareProfile().then(function () { return result; });
  }

  // A reconnect whose answer was lost left the old token in storage; the server accepts it once
  // more, for a short while, with this receiver's deviceId, so one retry recovers it.
  function reconnect() {
    var stored = readToken();
    if (!stored) { return Promise.reject(new Error('no_stored_pairing')); }
    var ask = function () { return post('/v1/cast/reconnect', {protocolVersion: PROTOCOL, deviceToken: stored, deviceId: deviceId()}); };
    return ask().catch(function (e) { if (e && e.status) throw e; return ask(); }).then(adopt);
  }

  /** Redeem a code with a server without touching the current pairing (kept until the new one is adopted). */
  function redeemAt(origin, code, name) {
    return post('/v1/cast/redeem', {protocolVersion:PROTOCOL,code:code,deviceId:deviceId(),displayName:name||'Portico on this TV'}, undefined, origin);
  }
  /** Make a redeemed pairing current: stop what's playing, forget the old credential, adopt the new one. */
  async function switchTo(result, origin) {
    await stopPlayback();
    base = origin;
    try { if (base) window.localStorage.setItem(ORIGIN_KEY, base); else window.localStorage.removeItem(ORIGIN_KEY); } catch (_) { /* ignore */ }
    profiled=false;
    return adopt(result);
  }
  /** Pairing by typing a code: only offered when the page is opened in an ordinary browser. */
  async function redeem(code) {
    var result = await redeemAt(base, code);
    return switchTo(result, base);
  }

  // A Cast device (Chromecast, Google TV) has no keyboard: the code box is only for this page opened
  // in an ordinary browser, for diagnosis. Never ask a TV to type (CAST-01, V-11).
  var ua = (typeof navigator !== 'undefined' && navigator.userAgent) || '';
  var castDevice = /CrKey|Android|Google ?TV|Fuchsia|AFT/i.test(ua);
  var browserPair = $('browser-pair');
  if (!castDevice && browserPair) {
    browserPair.hidden = false;
    browserPair.addEventListener('submit', function (event) {
      if (event && event.preventDefault) event.preventDefault();
      var input = $('code-input'), button = $('pair');
      var code = ((input && input.value) || '').trim().toUpperCase();
      if (code.length < 4) { say('Enter the code shown on your phone.', 'error'); return; }
      if (button) button.disabled = true;
      say('Connecting…');
      serial(function(){return redeem(code);}).then(function () {
        if (button) button.disabled = false;
        if (input) input.value = '';
        say('Connected. Choose a title on your phone or computer.');
      }).catch(function (error) {
        if (button) button.disabled = false;
        say(pairingMessage(error), 'error');
      });
    });
  }

  function pairingMessage(error) {
    switch (error && error.code) {
      case 'cast_code_expired': return 'That code has expired. Try again from your phone or computer.';
      case 'cast_code_consumed': return 'That code has already been used. Try again from your phone or computer.';
      case 'cast_code_revoked': return 'That code was canceled.';
      case 'cast_code_not_found': return 'That code isn’t valid.';
      case 'rate_limited': return 'Too many attempts. Wait a moment, then try again.';
      case 'tv_busy': return 'This TV is playing something for someone else. Ask them to stop, then try again.';
      default: return 'This TV couldn’t connect. Try again.';
    }
  }

  // --- Playback --------------------------------------------------------------

  var meta = null; // display identity for the title being loaded (from the controller)
  function cleanText(value, max) { return typeof value === 'string' ? value.replace(/[\u0000-\u001f\u007f]/g, ' ').trim().slice(0, max || 200) : ''; }
  function displayMeta(message) {
    if (!message || typeof message !== 'object') return null;
    var kind = cleanText(message.kind, 32);
    var season = Number.isSafeInteger(message.season) ? message.season : undefined;
    var episode = Number.isSafeInteger(message.episode) ? message.episode : undefined;
    var artworkPath = typeof message.artworkPath === 'string' && /^\/v[12]\/[^\s\\]{1,500}$/.test(message.artworkPath) ? message.artworkPath : '';
    var m = {title: cleanText(message.title), subtitle: cleanText(message.subtitle), seriesTitle: cleanText(message.seriesTitle), kind: kind, season: season, episode: episode, artworkPath: artworkPath};
    return m.title || m.subtitle || m.artworkPath ? m : null;
  }
  /** Artwork is protected: fetch it with the receiver's own bearer and hand CAF a data URL. */
  async function artworkUrl(path) {
    if (!path || !session || typeof FileReader === 'undefined') return '';
    var abort = new AbortController(), timer = setTimeout(function () { abort.abort(); }, 8000);
    try {
      var response = await fetch(base + path, {headers: {Authorization: 'Bearer ' + session.accessToken}, signal: abort.signal});
      if (!response.ok) return '';
      var blob = await response.blob();
      if (!/^image\//.test(blob.type) || blob.size > 3145728) return '';
      return await new Promise(function (resolve) { var reader = new FileReader(); reader.onload = function () { resolve(String(reader.result || '')); }; reader.onerror = function () { resolve(''); }; reader.readAsDataURL(blob); });
    } catch (e) { return ''; } finally { clearTimeout(timer); }
  }
  function castMetadata(m, image) {
    if (!m) return undefined;
    var images = image ? [{url: image}] : [];
    if (m.kind === 'episode') return {metadataType: 2, title: m.title, seriesTitle: m.seriesTitle || m.subtitle, season: m.season, episode: m.episode, images: images};
    if (m.kind === 'movie') return {metadataType: 1, title: m.title, subtitle: m.subtitle, images: images};
    if (m.kind === 'song' || m.kind === 'album') return {metadataType: 3, title: m.title, artist: m.subtitle, images: images};
    return {metadataType: 0, title: m.title, subtitle: m.subtitle, images: images};
  }

  // Playback v1 (the same session contract every Portico client plays through): start a session
  // for the item, load its presentation URL, report the timeline (progress, lease renewal and
  // observation in one POST), and end it. Play and pause are the player's own and are reported;
  // a seek goes through the server only when it must produce the stream again. Media URLs are
  // accepted only from the server's answer. Commands share one serialized lane.
  var directTypes = {mp3:'audio/mpeg',flac:'audio/flac',m4a:'audio/mp4',m4b:'audio/mp4',aac:'audio/aac',ogg:'audio/ogg',opus:'audio/ogg',wav:'audio/wav',webm:'video/webm',mp4:'video/mp4',m4v:'video/mp4',mov:'video/mp4'};
  /** Whether a seek must go through the server: only when something is transcoded or burned in. */
  function serverSeek(session) {
    var d = session.presentation.decision || {};
    return [d.video, d.audio, d.subtitles].some(function (x) { return !!x && (x.action === 'transcode' || x.action === 'burn'); });
  }
  function track(session) {
    return {id: session.id, revision: session.revision, itemId: session.itemId || '', generation: session.presentation.generation, serverSeek: serverSeek(session)};
  }
  /** CAF's load request for a session's presentation. */
  function loadRequest(session, image, autoplay) {
    var p = session.presentation;
    if (typeof p.url !== 'string' || !/^\/v1\/media\/[^\\\s]+$/.test(p.url)) throw new Error('invalid_media_url');
    var container = String(p.container || '');
    var media = {contentId: session.itemId || session.id, contentUrl: base + p.url, streamType: 'BUFFERED'};
    if (p.mode === 'stream') {
      media.contentType = 'application/x-mpegurl';
      media.hlsSegmentFormat = media.hlsVideoSegmentFormat = container === 'fmp4_hls' ? 'FMP4' : 'TS';
    } else {
      media.contentType = directTypes[container] || 'video/mp4';
    }
    // Title and artwork for CAF's standard player, the Google TV ambient UI and the Google Home app (CAST-01 §2).
    var metadata = castMetadata(meta, image);
    if (metadata) media.metadata = metadata;
    return {media: media, currentTime: Math.max(0, (p.startPositionMs || 0) / 1000), autoplay: autoplay !== false};
  }

  async function stopPlayback(ended) {
    clearTimeout(reportTimer); clearTimeout(bufferingTimer);
    if (!playback) return;
    var id = playback.id, at = Math.round(position() * 1000);
    playback = null; meta = null;
    if (manager) manager.stop();
    showIdle();
    // A session that has already ended (404, 410) is simply gone.
    await api(sessionPath(id), {method: 'DELETE', body: ended ? undefined : {positionMs: at}}).catch(function (e) { if (!e.status || e.status >= 500) throw e; });
  }
  /** Starts a v1 session for the item and answers CAF's load request for it. A session already
   * playing here is replaced in the same request, so this TV holds one playback slot. */
  async function resolvePlayback(itemId, startSeconds, identity) {
    if (!session || !manager || !/^[A-Za-z0-9_-]{1,128}$/.test(itemId)) throw new Error('playback_unavailable');
    clearTimeout(reportTimer); clearTimeout(bufferingTimer);
    var replaces = playback && playback.id;
    playback = null;
    meta = identity || null;
    showLoading(meta);
    var art = meta && meta.artworkPath ? artworkUrl(meta.artworkPath).then(function (url) { if (url) showArtwork(url, meta && (meta.kind === 'song' || meta.kind === 'album')); return url; }) : Promise.resolve('');
    await prepareProfile();
    var body = {itemId: itemId, state: 'playing'};
    if (Number.isFinite(startSeconds)) body.startPositionMs = Math.round(Math.max(0, startSeconds) * 1000); else body.startFrom = 'resume';
    if (replaces) body.replacesSessionId = replaces;
    var key = token(16), waited = 0, answer;
    // 202: the server is still preparing; the identical request is repeated until it answers.
    for (;;) {
      answer = await api('/v1/playback/sessions', {body: body, headers: {'Idempotency-Key': key}});
      if (answer.status !== 202) break;
      var delay = Math.min(10000, Math.max(250, Number(answer.body.retryAfterMs) || 1000));
      waited += delay;
      if (waited > 120000) throw new Error('preparation_timeout');
      await wait(delay);
    }
    var started = answer.body;
    if (!started || !started.presentation) throw new Error('playback_not_ready');
    var image = await Promise.race([art, wait(1500).then(function () { return ''; })]);
    var load = loadRequest(started, image, true);
    playback = track(started);
    reportSeq = 0; reportFailures = 0;
    return load;
  }
  function startPlayback(itemId,startSeconds,identity) { recoveries=0; return serial(async function(){try{var load=await resolvePlayback(itemId,startSeconds,identity);await manager.load(load);showPlaying();scheduleReport(0);return playback;}catch(e){showError(e&&(e.code||e.message));throw e;}}); }

  // --- Timeline: progress, lease renewal and observation in one report -------

  function timelineState() {
    var s = manager ? manager.getPlayerState() : 'IDLE';
    return s === 'PLAYING' ? 'playing' : s === 'BUFFERING' ? 'buffering' : 'paused';
  }
  async function report(state) {
    if (!playback) return;
    var id = playback.id;
    try {
      await api(sessionPath(id) + '/timeline', {body: {seq: ++reportSeq, generation: playback.generation, state: state || timelineState(), positionMs: Math.round(Math.max(0, position()) * 1000), rate: 1}});
      reportFailures = 0; notice('');
    } catch (e) {
      // The server no longer has this session (stopped elsewhere, moved, ended by an administrator).
      if (e.status === 404 || e.status === 410 || e.code === 'session_ended') { if (playback && playback.id === id) { playback = null; meta = null; if (manager) manager.stop(); showIdle(); broadcast(); } return; }
      reportFailures++; notice('Reconnecting…');
      throw e;
    }
  }
  /** The next report: 10 s while playing, 30 s paused, sooner after a failure (the lease is 120 s). */
  function scheduleReport(ms) {
    clearTimeout(reportTimer);
    if (!playback) return;
    if (ms === undefined) ms = reportFailures ? [1000, 2000, 5000, 10000][Math.min(reportFailures, 4) - 1] : paused() ? 30000 : 10000;
    reportTimer = setTimeout(function () { serial(function () { return report(); }).catch(function () {}).finally(function () { scheduleReport(); }); }, ms);
  }
  /** Reload the media after the server produced a new presentation (a server seek, a recovery). */
  async function adoptPresentation(updated, autoplay) {
    if (!playback || updated.id !== playback.id) return;
    var fresh = updated.presentation.generation !== playback.generation;
    playback = track(updated);
    if (!fresh) return;
    await manager.load(loadRequest(updated, '', autoplay));
    showPlaying();
  }
  /** PATCH the session. A 412 carries the server's session: adopt it and apply the change to it. */
  async function change(body) {
    if (!playback) return;
    var patch = function () { return api(sessionPath(playback.id), {method: 'PATCH', body: body, headers: {'If-Match': playback.revision, 'Content-Type': 'application/merge-patch+json'}}); };
    var answer;
    try { answer = await patch(); }
    catch (e) {
      if (e.status !== 412 || !e.current || !e.current.presentation) throw e;
      await adoptPresentation(e.current, !paused());
      answer = await patch();
    }
    await adoptPresentation(answer.body, !paused());
  }
  function seekTo(seconds) {
    return serial(async function () {
      if (!playback) return;
      var at = Math.max(0, seconds);
      if (!playback.serverSeek) { manager.seek(at); return; }
      await change({seek: {positionMs: Math.round(at * 1000), id: token(16)}});
    });
  }
  function recover(code) {
    if(recovering||!playback||recoveries>=3)return;
    recovering=true;recoveries++;broadcast();notice('Reconnecting…');
    serial(async function(){var item=playback.itemId,at=position(),wasPaused=paused(),identity=meta;
      var result=(await api('/v1/playback/route-failures',{body:{sessionId:playback.id,code:code,detail:''}})).body;
      if(!result.escalates&&code!=='stalled_without_data')throw new Error('playback_failed');
      var load=await resolvePlayback(item,at,identity);load.autoplay=!wasPaused;await manager.load(load);showPlaying();scheduleReport(0);
    }).then(function(){notice('');}).catch(function(e){notice('');showError(code==='decode_error'?'decode':e&&(e.code||e.message));}).finally(function(){recovering=false;broadcast();});
  }

  // --- Control: who may drive this TV (SEC-14, CAST-04, D-FEAT-10) ------------

  var controllerSender = null;   // the sender that paired in this session
  var controllerPresent = false; // still connected
  var pendingPair = null;        // {senderId, requester, origin, result, timer}

  function send(senderId, body) { try { if (castContext && senderId) castContext.sendCustomMessage(NAMESPACE, senderId, body); } catch (e) { /* the sender left */ } }
  function requesterName(message, senderId) {
    var name = cleanText(message && message.displayName, 64);
    if (name) return name;
    try { var sender = castContext && castContext.getSender && castContext.getSender(senderId); if (sender && sender.userAgent && /iPhone|iPad/.test(sender.userAgent)) return 'An iPhone or iPad'; } catch (e) { /* ignore */ }
    return 'Someone';
  }
  function hideTakeover() { visible('takeover', false); }
  function askTakeover(requester, controllerHere) {
    text('takeover-title', requester + ' wants to play on this TV');
    text('takeover-body', controllerHere ? 'Waiting for the person watching now to allow it.' : 'Switching in a few seconds.');
    visible('takeover', true);
  }
  function denyPending(code) {
    var p = pendingPair; if (!p) return;
    pendingPair = null; clearTimeout(p.timer); hideTakeover();
    send(p.senderId, {type: 'pair-failed', code: code || 'tv_busy', message: pairingMessage({code: code || 'tv_busy'})});
  }
  function allowPending() {
    var p = pendingPair; if (!p) return;
    pendingPair = null; clearTimeout(p.timer); hideTakeover();
    serial(function () { return takeControl(p.senderId, p.origin, p.result); }).catch(function (error) {
      send(p.senderId, {type: 'pair-failed', code: error.code || '', message: pairingMessage(error)});
    });
  }
  async function takeControl(senderId, origin, result) {
    var previous = controllerSender;
    await switchTo(result, origin);
    controllerSender = senderId; controllerPresent = true;
    if (previous && previous !== senderId) send(previous, {type: 'replaced'});
    send(senderId, {type: 'paired'});
  }
  /** A pair request: redeem first (keeping the current pairing), then decide whether to switch now or ask. */
  function handlePair(senderId, message) {
    var named = serverOrigin(message.origin);
    if (named === null) { send(senderId, {type: 'pair-failed', code: 'origin_refused', message: 'This server can only be reached over HTTPS.'}); return; }
    var requester = requesterName(message, senderId);
    var code = String(message.code || '').trim().toUpperCase();
    redeemAt(named, code).then(function (result) {
      var sameViewer = !!(scope && result.scope && result.scope.accountId === scope.accountId && named === base);
      var busy = !!playback && controllerSender !== senderId && !sameViewer;
      if (!busy) return serial(function () { return takeControl(senderId, named, result); });
      if (pendingPair) denyPending('tv_busy');
      pendingPair = {senderId: senderId, requester: requester, origin: named, result: result, timer: null};
      send(senderId, {type: 'pair-pending'});
      var here = !!controllerSender && controllerPresent;
      askTakeover(requester, here);
      if (here) {
        send(controllerSender, {type: 'takeover-requested', requester: requester});
        pendingPair.timer = setTimeout(function () { denyPending('tv_busy'); }, TAKEOVER_ASK_MS);
      } else {
        pendingPair.timer = setTimeout(allowPending, TAKEOVER_ABSENT_MS);
      }
    }).catch(function (error) {
      send(senderId, {type: 'pair-failed', code: error.code || '', message: pairingMessage(error)});
    });
  }
  // The TV remote (where a Google TV delivers keys to the page): Select allows, Back declines.
  if (window.addEventListener) window.addEventListener('keydown', function (event) {
    if (!pendingPair) return;
    if (event.key === 'Enter' || event.key === ' ') allowPending();
    else if (event.key === 'Escape' || event.key === 'Backspace' || event.key === 'GoBack' || event.key === 'BrowserBack') denyPending('tv_busy');
  });
  function isController(senderId) { return !!controllerSender && senderId === controllerSender; }
  function statusBody() {
    return {
      type: 'status', paired: !!session, itemId: playback ? playback.itemId || '' : '',
      positionSeconds: position(), durationSeconds: playback && manager && typeof manager.getDurationSec === 'function' ? manager.getDurationSec() || 0 : 0,
      audioTracks: manager ? manager.getAudioTracksManager().getTracks().map(function(t){return {id:t.trackId,language:t.language||'',name:t.name||''};}) : [],
      textTracks: manager ? manager.getTextTracksManager().getTracks().map(function(t){return {id:t.trackId,language:t.language||'',name:t.name||''};}) : [],
      paused: paused(), state: recovering?'recovering':playback?(paused()?'paused':'playing'):'idle'
    };
  }

  // --- Cast framework wiring ------------------------------------------------

  function attachCastFramework() {
    if (!window.cast || !window.cast.framework) { return; }
    var context = cast.framework.CastReceiverContext.getInstance();
    castContext=context; manager = context.getPlayerManager();
    var types=cast.framework.messages.MessageType;
    // Media-channel commands from a sender that isn't the controller: pause and stop are allowed
    // (anyone in the room can do that with the remote); load, play and seek are not.
    function fromOther(request) { return !!controllerSender && !!request && !!request.senderId && request.senderId !== controllerSender; }
    // A LOAD carries only a Portico item id in customData. It never carries a
    // media URL, a bearer or a media grant: the receiver resolves everything
    // itself with its own credential.
    manager.setMessageInterceptor(types.LOAD, function (request) {
      var custom = (request && request.media && request.media.customData) || {};
      if (!custom.itemId || fromOther(request)) { return null; }
      var identity = displayMeta(custom) || (request.media.metadata ? displayMeta({title: request.media.metadata.title, subtitle: request.media.metadata.subtitle || request.media.metadata.seriesTitle}) : null);
      return serial(function(){recoveries=0;return resolvePlayback(custom.itemId,custom.startSeconds,identity);}).then(function(load){showPlaying();scheduleReport(0);return load;}).catch(function(e){showError(e&&(e.code||e.message));return null;});
    });
    // Play and pause are the player's own; the timeline reports them. A seek the server must
    // produce again is sent to it and the new presentation loaded here instead.
    manager.setMessageInterceptor(types.PAUSE,function(request){return request;});
    manager.setMessageInterceptor(types.PLAY,function(request){return fromOther(request)?null:request;});
    manager.setMessageInterceptor(types.SEEK,function(request){
      if (fromOther(request) || !playback || !Number.isFinite(request.currentTime)) return null;
      if (!playback.serverSeek) return request;
      seekTo(request.currentTime).catch(function(){notice('That seek couldn’t be made.', 5000);});
      return null;
    });
    manager.setMessageInterceptor(types.STOP,function(request){return serial(stopPlayback).then(function(){return request;}).catch(function(){return null;});});
    // A Cast device has no keyboard, so the sender delivers the pairing code it was issued
    // and then drives playback over this channel. What plays is always the library of the
    // person who paired, and only they drive it.
    context.addCustomMessageListener(NAMESPACE, function (event) {
      var message = event.data || {};
      if (typeof message === 'string') { try { message = JSON.parse(message); } catch (e) { message = {}; } }
      var senderId = event.senderId;
      var reply = function (body) { send(senderId, body); };
      if (message.type === 'pair') { handlePair(senderId, message); return; }
      if (message.type === 'takeover') {
        if (isController(senderId) && pendingPair) { if (message.allow === true) allowPending(); else denyPending('tv_busy'); }
        return;
      }
      if (!isController(senderId)) {
        // Nothing about what's playing, and no control, for a sender that didn't pair.
        if (message.type === 'pause' && playback) { manager.pause(); return; }
        if (message.type === 'stop' && playback) { serial(stopPlayback).catch(function(){}); return; }
        reply(controllerSender ? {type: 'busy'} : {type: 'pair-required'});
        return;
      }
      switch (message.type) {
        case 'load':
          startPlayback(String(message.itemId || ''), typeof message.startSeconds === 'number' ? message.startSeconds : undefined, displayMeta(message)).then(function () {
            broadcast();
          }).catch(function (error) {
            reply({ type: 'load-failed', code: error.code || error.message || '' });
          });
          break;
        case 'play': if (playback) manager.play(); break;
        case 'pause': if (playback) manager.pause(); break;
        case 'seek': if(Number.isFinite(message.positionSeconds)) seekTo(message.positionSeconds).catch(function(){notice('That seek couldn’t be made.', 5000);}); break;
        case 'audio': if(Number.isSafeInteger(message.trackId)) manager.getAudioTracksManager().setActiveById(message.trackId); break;
        case 'subtitles': if(Array.isArray(message.trackIds)&&message.trackIds.length<=1&&message.trackIds.every(Number.isSafeInteger)) manager.getTextTracksManager().setActiveByIds(message.trackIds); break;
        case 'stop': serial(stopPlayback).catch(function(){notice('Stop couldn’t be confirmed.', 5000);}); break;
        case 'status': break;
        default: return;
      }
      broadcast();
    });
    // Status goes only to the controller (SEC-14).
    broadcast = function () { if (controllerSender && controllerPresent) send(controllerSender, statusBody()); };
    var events=cast.framework.events.EventType;
    manager.addEventListener(events.ERROR,function(e){var code=Number(e.detailedErrorCode);if(code>=300&&code<500)recover('decode_error');});
    manager.addEventListener(events.BUFFERING,function(e){clearTimeout(bufferingTimer);if(e.isBuffering)bufferingTimer=setTimeout(function(){recover('stalled_without_data');},20000);});
    manager.addEventListener(events.MEDIA_FINISHED,function(){serial(async function(){await report('ended');await stopPlayback(true);}).catch(function(){});});
    // A state change or a completed seek is reported at once.
    [events.PAUSE,events.PLAYING,events.SEEKED].filter(Boolean).forEach(function(name){manager.addEventListener(name,function(){scheduleReport(0);broadcast();});});
    context.addEventListener(cast.framework.system.EventType.SENDER_DISCONNECTED,function(e){
      if (e && e.senderId && e.senderId === controllerSender) {
        controllerPresent = false;
        // Someone was waiting for the controller's answer: with the controller gone, the TV says who is taking over, then switches.
        if (pendingPair) { clearTimeout(pendingPair.timer); askTakeover(pendingPair.requester, false); pendingPair.timer = setTimeout(allowPending, TAKEOVER_ABSENT_MS); }
      }
      if (pendingPair && e && e.senderId === pendingPair.senderId) { clearTimeout(pendingPair.timer); pendingPair = null; hideTakeover(); }
      if(context.getSenders().length===0){controllerSender=null;controllerPresent=false;serial(stopPlayback).catch(function(){});}
    });
    setInterval(function(){if(playback)broadcast();},2000);
    // The standard idle timeout closes the app when nothing is playing and nobody is connected (CAST-01 §5).
    context.start({ customNamespaces: (function () { var n = {}; n[NAMESPACE] = cast.framework.system.MessageType.JSON; return n; })() });
  }

  showIdle();
  attachCastFramework();

  // Exposed for the server's own smoke test page and for manual diagnosis.
  window.porticoCastReceiver = {
    buildProfile: buildProfile,
    redeem: redeem,
    reconnect: reconnect,
    startPlayback: startPlayback,
    state: function () { return { paired: !!session, scope: scope, playbackId: playback && playback.id, controller: controllerSender, pending: pendingPair ? pendingPair.senderId : null }; }
  };
})();
