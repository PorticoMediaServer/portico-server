export const liveActions = {
  play: 'live.play',
  favoriteAdd: 'favorite.add',
  favoriteRemove: 'favorite.remove',
  record: 'dvr.record',
  recordSeries: 'dvr.record-series',
  recordingCancel: 'dvr.cancel',
  recordingDelete: 'dvr.delete',
  recordingPlay: 'dvr.play',
  ruleDelete: 'dvr.delete',
  ruleEdit: 'dvr.edit',
  ruleEnable: 'dvr.enable',
  ruleDisable: 'dvr.disable',
  ruleCreate: 'dvr.rule.create',
} as const;

export function hasAction(item: { actions?: string[] } | undefined, action: string) {
  return item?.actions?.includes(action) ?? false;
}

export type ServerClockSample = {
  serverTimeMs: number;
  clientMonotonicMs: number;
};

function monotonicNow() {
  return typeof globalThis.performance?.now === 'function' ? globalThis.performance.now() : Date.now();
}

export function createServerClockSample(serverTime: string, clientMonotonicMs = monotonicNow()): ServerClockSample | undefined {
  const serverTimeMs = Date.parse(serverTime);
  return Number.isFinite(serverTimeMs) ? { serverTimeMs, clientMonotonicMs } : undefined;
}

export function serverClockNow(sample: ServerClockSample, clientMonotonicMs = monotonicNow()): number {
  return sample.serverTimeMs + Math.max(0, clientMonotonicMs - sample.clientMonotonicMs);
}
