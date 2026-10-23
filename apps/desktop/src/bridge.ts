import {invoke} from '@tauri-apps/api/core';

export type NativeObservation = Readonly<{generation: number; state: 'idle' | 'loading' | 'playing' | 'paused' | 'buffering' | 'ended' | 'error'; positionMs: number; rate: number; volume: number; muted: boolean; droppedFrames: number; nativeSurfaceActive: boolean}>;
export type SurfaceBounds = Readonly<{x: number; y: number; width: number; height: number}>;
export type NativeCommand = {action: string; generation?: number; source?: {url: string; generation: number; mode: string; startPositionMs: number; autoplay: boolean}; milliseconds?: number; rate?: number; volume?: number; muted?: boolean};
export interface Bridge {command(value: NativeCommand): Promise<void>; observe(): Promise<NativeObservation>; surface?(bounds: SurfaceBounds, generation: number): Promise<void>}
export function observation(value: unknown): NativeObservation {
  if (!value || typeof value !== 'object') throw new Error('engine_failure');
  const x = value as Record<string, unknown>;
  if (!Number.isSafeInteger(x.generation) || Number(x.generation) < 0 || !['idle','loading','playing','paused','buffering','ended','error'].includes(String(x.state)) ||
      !['positionMs','rate','volume','droppedFrames'].every(key => typeof x[key] === 'number' && Number.isFinite(x[key]) && Number(x[key]) >= 0) || typeof x.muted !== 'boolean' || typeof x.nativeSurfaceActive !== 'boolean') throw new Error('engine_failure');
  // Explicit projection: no native fields can accidentally become UI/session telemetry.
  return {generation: Number(x.generation), state: x.state as NativeObservation['state'], positionMs: Number(x.positionMs), rate: Number(x.rate), volume: Number(x.volume), muted: x.muted, droppedFrames: Number(x.droppedFrames), nativeSurfaceActive: x.nativeSurfaceActive};
}
export const nativeBridge: Bridge = {
  command: command => invoke<void>('desktop_command', {command}),
  observe: () => invoke<unknown>('desktop_observe').then(observation),
  surface: (bounds,generation) => invoke<void>('desktop_surface', {bounds,generation}),
};
