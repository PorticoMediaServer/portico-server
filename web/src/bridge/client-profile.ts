import {browserConfigCheck, probeAudioDecode} from './audio/capabilities';
import {ClientProfilePublisher, probeWebProfile, type ClientProfile, type DecodingAnswer, type WebProbeEnvironment} from '@core/client-profile.ts';

/*
 * The real browser behind the shared probe. Each answer has a safe "no": a browser that cannot
 * be asked is described as one that cannot, and the server converts for it.
 */

function engine(): {engine: string; engineVersion: string; os: string} {
  const ua = navigator.userAgent;
  const firefox = /Firefox\/([\d.]+)/.exec(ua), blink = /(?:Chrome|Chromium|Edg)\/([\d.]+)/.exec(ua), webkit = /Version\/([\d.]+).*Safari/.exec(ua);
  const os = /iPhone|iPad/.test(ua) ? 'ios' : /Android/.test(ua) ? 'android' : /Mac OS X/.test(ua) ? 'macos' : /Windows/.test(ua) ? 'windows' : /CrOS/.test(ua) ? 'chromeos' : /Linux/.test(ua) ? 'linux' : '';
  if (firefox) return {engine: 'gecko', engineVersion: firefox[1], os};
  if (blink) return {engine: 'blink', engineVersion: blink[1], os};
  return {engine: 'webkit', engineVersion: webkit?.[1] ?? '', os};
}

function outputChannels(): number | undefined {
  try {
    const Context = window.AudioContext ?? (window as unknown as {webkitAudioContext?: typeof AudioContext}).webkitAudioContext;
    if (!Context) return undefined;
    const context = new Context();
    const channels = context.destination.maxChannelCount;
    void context.close().catch(() => {});
    return channels || undefined;
  } catch { return undefined; }
}

function environment(): WebProbeEnvironment {
  const element = document.createElement('video');
  // Safari on iPhone has ManagedMediaSource and no MediaSource.
  const Source = (window as unknown as {ManagedMediaSource?: typeof MediaSource}).ManagedMediaSource ?? (typeof MediaSource === 'undefined' ? undefined : MediaSource);
  const capabilities = navigator.mediaCapabilities;
  const browser = engine();
  return {
    canPlayType: type => { try { return element.canPlayType(type); } catch { return ''; } },
    mediaSourceSupports: Source ? type => { try { return Source.isTypeSupported(type); } catch { return false; } } : null,
    decodingInfo: capabilities?.decodingInfo
      ? async question => {
          const answer = await capabilities.decodingInfo({type: 'media-source', video: question as VideoConfiguration});
          return {supported: answer.supported, smooth: answer.smooth, powerEfficient: answer.powerEfficient} satisfies DecodingAnswer;
        }
      : undefined,
    matchMedia: query => { try { return matchMedia(query).matches; } catch { return false; } },
    screen: {width: Math.round(screen.width * devicePixelRatio), height: Math.round(screen.height * devicePixelRatio)},
    audioOutputChannels: outputChannels(),
    embeddedAudioTracks: 'audioTracks' in HTMLMediaElement.prototype,
    tonemapsHdr: browser.engine === 'webkit' || (browser.engine === 'blink' && browser.os === 'macos'),
    identity: {platform: 'web', os: browser.os, app: 'Portico Web', appVersion: import.meta.env.VITE_APP_VERSION ?? '', engineVersion: browser.engineVersion, platformVersion: browser.engine},
  };
}

let asked: Promise<ClientProfile> | undefined;
/** The document for this browser. Asked once, and again when the screen or the audio device changes.
 * It carries what the music engine decodes on the device (`audioDecode`, §18.1): pairs that passed
 * the §18.8 fixtures on this engine and that `AudioDecoder` confirms here. */
export function browserClientProfile(fresh = false): Promise<ClientProfile> {
  if (fresh || !asked) asked = Promise.all([probeWebProfile(environment()), probeAudioDecode(browserConfigCheck()).catch(() => [])])
    .then(([profile, audioDecode]): ClientProfile => ({...profile, audioDecode: audioDecode.map(({sampleRates, containers, ...a}) => ({...a, containers: [...containers], ...(sampleRates ? {sampleRates: [...sampleRates]} : {})}))}));
  return asked;
}

type Api = {request<T>(path: string, method?: string, body?: unknown): Promise<T>};

/**
 * Keeps this sign-in's server told what the browser plays: once after sign-in, and again if the
 * answer changes (a window dragged to an HDR display, headphones swapped for a surround
 * receiver). It never throws and nothing waits for it. Returns a function that stops watching.
 */
export function keepBrowserProfilePublished(api: Api, signIn: string): () => void {
  const publisher = new ClientProfilePublisher((path, method, body) => api.request<unknown>(path, method, body), () => signIn);
  let stopped = false;
  const publish = (fresh: boolean) => { void browserClientProfile(fresh).then(profile => { if (!stopped) void publisher.publish(profile); }).catch(() => {}); };
  publish(false);
  const changed = () => publish(true);
  const hdr = (() => { try { return matchMedia('(dynamic-range: high)'); } catch { return undefined; } })();
  hdr?.addEventListener?.('change', changed);
  navigator.mediaDevices?.addEventListener?.('devicechange', changed);
  return () => { stopped = true; hdr?.removeEventListener?.('change', changed); navigator.mediaDevices?.removeEventListener?.('devicechange', changed); };
}
