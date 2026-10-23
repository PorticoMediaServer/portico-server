import {CAST_NAMESPACE, parseCastEvent, type CastEvent, type CastTransport} from '@core/cast/index.ts';
import type {CastContext, CastFramework, CastMessageListener, CastSession, CastSessionEvent} from './sdk';

export type ConfirmedCastDevice = Readonly<{name: string; id?: string}>;
export type CastReceiverStatus = Extract<CastEvent, {type: 'status'}>;

/** Only a selected SDK session supplies a receiver's name and id. */
export function confirmedCastDevice(session: CastSession): ConfirmedCastDevice | undefined {
  const device = session.getCastDevice();
  if (typeof device.friendlyName !== 'string' || !device.friendlyName.trim()) return undefined;
  return Object.freeze({name: device.friendlyName, ...(typeof device.label === 'string' && device.label ? {id: device.label} : {})});
}

/** Portico's custom channel only. No CAF media load or media/grant URL is accepted here. */
export function castTransportFor(session: CastSession, context: CastContext, framework: CastFramework, observeStatus?: (status: CastReceiverStatus) => void): CastTransport {
  const device = session.getCastDevice();
  return {
    deviceName: typeof device.friendlyName === 'string' ? device.friendlyName : '',
    ...(typeof device.label === 'string' && device.label ? {deviceId: device.label} : {}),
    send: command => session.sendMessage(CAST_NAMESPACE, command).then(() => undefined),
    onMessage: receive => {
      const listener: CastMessageListener = (_namespace, raw) => {
        const event = parseCastEvent(raw);
        if (event?.type === 'status') observeStatus?.(event);
        receive(raw);
      };
      session.addMessageListener(CAST_NAMESPACE, listener);
      return () => session.removeMessageListener(CAST_NAMESPACE, listener);
    },
    onSessionEnd: ended => {
      const listener = (event: CastSessionEvent) => {
        if (event.sessionState !== framework.SessionState.SESSION_ENDED && event.sessionState !== framework.SessionState.SESSION_START_FAILED) return;
        const current = context.getCurrentSession();
        if (!current || current === session) ended();
      };
      const type = framework.CastContextEventType.SESSION_STATE_CHANGED;
      context.addEventListener(type, listener);
      return () => context.removeEventListener(type, listener);
    },
    end: stopReceiver => {
      try { void Promise.resolve(session.endSession(stopReceiver)).catch(() => {}); } catch { /* The SDK may have already ended it. */ }
    },
  };
}
