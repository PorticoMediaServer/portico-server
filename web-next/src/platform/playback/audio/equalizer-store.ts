import {eqFrequencies, flatEqualizer, parseEqualizer, type EqualizerSettings} from './equalizer';

/** Device-local relational stores. A sink row and ten keyed band rows commit atomically. */
export class EqualizerStore {
  private opened?: Promise<IDBDatabase>;
  constructor(private readonly factory: IDBFactory | undefined = globalThis.indexedDB, private readonly name = 'portico-audio-output-v1') {}
  private database(): Promise<IDBDatabase> {
    return this.opened ??= new Promise((resolve, reject) => {
      if (!this.factory) { reject(new Error('equalizer_storage_unavailable')); return; }
      const request = this.factory.open(this.name, 1);
      let abandoned = false;
      request.onupgradeneeded = () => {
        request.result.createObjectStore('outputs', {keyPath: 'outputId'});
        request.result.createObjectStore('bands', {keyPath: ['outputId', 'band']});
      };
      request.onerror = () => reject(request.error);
      request.onblocked = () => { abandoned = true; reject(new Error('equalizer_storage_blocked')); };
      request.onsuccess = () => { if (abandoned) { request.result.close(); return; } request.result.onversionchange = () => request.result.close(); resolve(request.result); };
    });
  }
  private key(outputId: string): string {
    if (!outputId || outputId.length > 256 || /[\x00-\x1f\x7f]/.test(outputId)) throw new Error('invalid_audio_output');
    return outputId;
  }
  async load(outputId: string): Promise<EqualizerSettings> {
    const key = this.key(outputId), db = await this.database();
    return new Promise((resolve, reject) => {
      const tx = db.transaction(['outputs', 'bands'], 'readonly');
      const output = tx.objectStore('outputs').get(key);
      const bands = eqFrequencies.map((_, band) => tx.objectStore('bands').get([key, band]));
      tx.onerror = () => reject(tx.error); tx.onabort = () => reject(tx.error);
      tx.oncomplete = () => {
        try { resolve(output.result ? parseEqualizer({enabled: output.result.enabled, preampDb: output.result.preampDb, bandsDb: bands.map(r => r.result?.gainDb)}) : flatEqualizer); }
        catch (error) { reject(error); }
      };
    });
  }
  async save(outputId: string, settings: EqualizerSettings): Promise<void> {
    const key = this.key(outputId), value = parseEqualizer(settings), db = await this.database();
    return new Promise((resolve, reject) => {
      const tx = db.transaction(['outputs', 'bands'], 'readwrite');
      tx.objectStore('outputs').put({outputId: key, enabled: value.enabled, preampDb: value.preampDb});
      value.bandsDb.forEach((gainDb, band) => tx.objectStore('bands').put({outputId: key, band, gainDb}));
      tx.oncomplete = () => resolve(); tx.onerror = () => reject(tx.error); tx.onabort = () => reject(tx.error);
    });
  }
  async close(): Promise<void> { const pending = this.opened; this.opened = undefined; if (pending) (await pending.catch(() => undefined))?.close(); }
}

/** Default output has its own row; an explicit sinkId identifies only an actual selected sink. */
export function browserOutputId(context: object & {sinkId?: string | object}): string {
  return typeof context.sinkId === 'string' && context.sinkId ? context.sinkId : 'default';
}

/** No permission prompt: identify the OS default only when the browser actually exposes its group. */
export async function resolvedOutputId(context: object & {sinkId?: string | object} = {}): Promise<string> {
  const selected = browserOutputId(context);
  if (selected !== 'default') return selected;
  try {
    const devices = await globalThis.navigator?.mediaDevices?.enumerateDevices();
    const outputs = devices?.filter(device => device.kind === 'audiooutput');
    const current = outputs?.find(device => device.deviceId === 'default');
    return current?.groupId ? outputs?.find(device => device.deviceId !== 'default' && device.groupId === current.groupId)?.deviceId || 'default' : 'default';
  } catch { return 'default'; }
}

export async function savedOutputEqualizer(): Promise<EqualizerSettings> {
  const store = new EqualizerStore();
  try { return await store.load(await resolvedOutputId()); }
  finally { await store.close(); }
}

export async function saveOutputEqualizer(settings: EqualizerSettings): Promise<void> {
  const store = new EqualizerStore();
  try { await store.save(await resolvedOutputId(), settings); }
  finally { await store.close(); }
}
