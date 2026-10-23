import {useEffect, useRef, useState} from 'react';
import {DeviceEqualizer} from './Equalizer';
import {createBrowserEqualizerPort} from './equalizer-port';

/** Settings and the player edit the renderer's same app/device-local record. */
export function BrowserSettingsEqualizer({layout, onError}: Readonly<{layout: 'settings'; onError(error: unknown): void}>) {
  const error = useRef(onError);
  error.current = onError;
  const [port] = useState(() => createBrowserEqualizerPort(value => error.current(value)));
  const [sampleRate, setSampleRate] = useState<number>();
  useEffect(() => {
    // A silent context measures the browser's filter rate for the preview. It neither opens
    // an audio source nor supplies an observed DAC/output rate or an applied-EQ fact.
    let context: AudioContext | undefined;
    try { context = new AudioContext(); setSampleRate(context.sampleRate); } catch { /* No invented rate on unsupported browsers. */ }
    return () => { void context?.close().catch(() => {}); };
  }, []);
  return <DeviceEqualizer port={port} layout={layout} sampleRate={sampleRate} reportError={onError} />;
}
