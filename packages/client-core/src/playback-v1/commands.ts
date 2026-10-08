/**
 * Remote control and transfer (spec §9.1, §9.2).
 *
 * Sender side: list controllable devices, send a command, offer a transfer. Target side: commands
 * arrive as `device.command` events and transfers as `playback.transfer_offered`; the router
 * de-duplicates by command id (events can be redelivered after a resync) and hands each to the
 * app's handler. Control always goes through the server; local-network discovery only finds
 * devices (spec §9.3 as amended).
 */
import {call, enc, idempotencyKey, type V1Http} from './http.ts';
import type {QualityRequest} from './quality.ts';
import {parseDevice, parsePage, obj, type ControllableDevice, type Selector, type ServerEvent} from './types.ts';

export type PlayTarget = Readonly<{itemId: string} | {queueId: string} | {selector: Selector}>;

export type DeviceCommand =
  | Readonly<{type: 'play'; target: PlayTarget; startFrom?: 'resume' | 'beginning'}>
  | Readonly<{type: 'pause' | 'resume' | 'stop' | 'next' | 'previous'}>
  | Readonly<{type: 'seek'; positionMs: number}>
  | Readonly<{type: 'setTracks'; audio?: string; subtitles?: string | null}>
  | Readonly<{type: 'setQuality'; quality: QualityRequest}>
  | Readonly<{type: 'setVolume'; level: number}>
  | Readonly<{type: 'setRate'; rate: number}>
  | Readonly<{type: 'playNext' | 'addToQueue'; selector: Selector}>;

export type ReceivedCommand = DeviceCommand & Readonly<{commandId: string; fromDeviceId?: string}>;

export class DeviceCommandsClient {
  private http: V1Http;
  private key: () => string;
  constructor(http: V1Http, key: () => string = () => idempotencyKey()) { this.http = http; this.key = key; }

  /** Devices this viewer may control, with what each is playing (`GET /v1/me/devices?controllable=true`). */
  async controllable(signal?: AbortSignal): Promise<readonly ControllableDevice[]> {
    return parsePage((await call(this.http, {method: 'GET', path: '/v1/me/devices?controllable=true', signal})).body, parseDevice).items;
  }

  /** Send one command; resolves with the server's command id when it returns one. */
  async send(deviceId: string, command: DeviceCommand, signal?: AbortSignal): Promise<string | undefined> {
    const r = await call(this.http, {method: 'POST', path: `/v1/devices/${enc(deviceId)}/commands`, headers: {'Idempotency-Key': this.key()}, body: command, signal}, [200, 201, 202, 204]);
    const b = r.body as {commandId?: unknown; id?: unknown} | undefined;
    return typeof b?.commandId === 'string' ? b.commandId : typeof b?.id === 'string' ? b.id : undefined;
  }

  /** Offer this device's session to another device (spec §9.2). The source keeps playing until the target commits. */
  async offerTransfer(sessionId: string, targetDeviceId: string, signal?: AbortSignal): Promise<string> {
    const r = await call(this.http, {method: 'POST', path: `/v1/playback/sessions/${enc(sessionId)}/transfers`, headers: {'Idempotency-Key': this.key()}, body: {targetDeviceId}, signal}, [200, 201, 202]);
    const id = (r.body as {transferId?: unknown} | undefined)?.transferId;
    if (typeof id !== 'string' || !id) throw new Error('transfer id missing');
    return id;
  }
}

const COMMAND_TYPES = new Set(['play', 'pause', 'resume', 'stop', 'seek', 'next', 'previous', 'setTracks', 'setQuality', 'setVolume', 'setRate', 'playNext', 'addToQueue']);

/** Validate a `device.command` payload (tolerant: unknown types are ignored, not errors). */
export function parseCommand(data: unknown): ReceivedCommand | undefined {
  if (!obj(data) || typeof data.commandId !== 'string' || !data.commandId || typeof data.type !== 'string' || !COMMAND_TYPES.has(data.type)) return undefined;
  const c = data as Record<string, unknown>;
  switch (c.type) {
    case 'seek': if (typeof c.positionMs !== 'number' || !(c.positionMs >= 0)) return undefined; break;
    case 'setVolume': if (typeof c.level !== 'number' || c.level < 0 || c.level > 1) return undefined; break;
    case 'setRate': if (typeof c.rate !== 'number' || c.rate < 0.25 || c.rate > 4) return undefined; break;
    case 'play': if (!obj(c.target)) return undefined; break;
    case 'playNext': case 'addToQueue': if (!obj(c.selector)) return undefined; break;
    case 'setQuality': if (!obj(c.quality)) return undefined; break;
  }
  return Object.freeze({...(c as object)}) as ReceivedCommand;
}

export type CommandHandler = (command: ReceivedCommand) => void | Promise<void>;
export type TransferOffer = Readonly<{transferId: string; fromDeviceId?: string; itemId?: string; title?: string}>;

/**
 * Target side: turns events into commands and offers. Each command id is handled once (a bounded
 * memory of recent ids), in arrival order.
 */
export class CommandRouter {
  private seen: string[] = [];
  private seenSet = new Set<string>();
  private onCommand?: CommandHandler;
  private onOffer?: (offer: TransferOffer) => void;
  private chain: Promise<unknown> = Promise.resolve();
  private memory: number;

  constructor(handlers: {onCommand?: CommandHandler; onTransferOffered?: (offer: TransferOffer) => void; memory?: number} = {}) {
    this.onCommand = handlers.onCommand;
    this.onOffer = handlers.onTransferOffered;
    this.memory = handlers.memory ?? 256;
  }

  private remember(id: string): boolean {
    if (this.seenSet.has(id)) return false;
    this.seen.push(id); this.seenSet.add(id);
    if (this.seen.length > this.memory) this.seenSet.delete(this.seen.shift()!);
    return true;
  }

  /** Feed every event; returns true when it was a command or an offer this router took. */
  handle(event: ServerEvent): boolean {
    if (event.type === 'device.command') {
      const c = parseCommand(event.data);
      if (!c || !this.remember('c:' + c.commandId)) return false;
      const run = () => this.onCommand?.(c);
      this.chain = this.chain.then(run, run).catch(() => {});
      return true;
    }
    if (event.type === 'playback.transfer_offered' && obj(event.data) && typeof event.data.transferId === 'string') {
      const d = event.data;
      if (!this.remember('t:' + d.transferId)) return false;
      this.onOffer?.(Object.freeze({transferId: d.transferId as string, fromDeviceId: typeof d.fromDeviceId === 'string' ? d.fromDeviceId : undefined, itemId: typeof d.itemId === 'string' ? d.itemId : undefined, title: typeof d.title === 'string' ? d.title : undefined}));
      return true;
    }
    return false;
  }

  /** Wait for handlers already dispatched (tests, shutdown). */
  settled(): Promise<void> { return this.chain.then(() => undefined); }
}

/** The part of a session controller a command needs. `PlaybackSessionController` satisfies it. */
export type CommandTarget = Readonly<{
  play(): void; pause(): void; stop(): Promise<void>;
  seek(positionMs: number): unknown; setAudio(trackId: string): unknown; setSubtitles(trackId: string | null): unknown;
  setQuality(quality: QualityRequest): unknown; setRate?(rate: number): void; setVolume?(level: number): void;
  start(target: {itemId: string} | {transferId: string}, options?: {startFrom?: 'resume' | 'beginning'}): Promise<unknown>;
}>;

/**
 * Default command behaviour over a session controller. Queue commands (`next`, `previous`,
 * `playNext`, `addToQueue`, play of a queue or selector) go to `queue`, which the app supplies
 * once its queue view exists.
 */
export function commandHandler(target: CommandTarget, queue?: Partial<Record<'next' | 'previous' | 'playNext' | 'addToQueue' | 'playQueue', (c: ReceivedCommand) => unknown>>): CommandHandler {
  return async c => {
    switch (c.type) {
      case 'pause': target.pause(); return;
      case 'resume': target.play(); return;
      case 'stop': await target.stop(); return;
      case 'seek': await target.seek(c.positionMs); return;
      case 'setTracks':
        if (c.audio) await target.setAudio(c.audio);
        if (c.subtitles !== undefined) await target.setSubtitles(c.subtitles);
        return;
      case 'setQuality': await target.setQuality(c.quality); return;
      case 'setRate': target.setRate?.(c.rate); return;
      case 'setVolume': target.setVolume?.(c.level); return;
      case 'play':
        if ('itemId' in c.target) { await target.start({itemId: c.target.itemId}, {startFrom: c.startFrom}); return; }
        await queue?.playQueue?.(c); return;
      case 'next': case 'previous': case 'playNext': case 'addToQueue': await queue?.[c.type]?.(c); return;
    }
  };
}
