/** A fake BaseAudioContext for node: records what the engine schedules and plays it back as
 * `advance()` moves the clock (onended fires for buffers that finished). */
export function fakeContext(sampleRate = 48000) {
  const scheduled: {start: number; frames: number; node: any}[] = [];
  const param = (value = 1) => ({value, events: [] as unknown[], setValueAtTime(v: number, t: number) { this.events.push(['set', v, t]); }, linearRampToValueAtTime(v: number, t: number) { this.events.push(['ramp', v, t]); this.value = v; }, cancelScheduledValues() {}, setValueCurveAtTime(c: Float32Array, t: number, d: number) { this.events.push(['curve', c[0], c[c.length - 1], t, d]); }, setTargetAtTime(v: number) { this.value = v; }});
  const ctx: any = {
    sampleRate, currentTime: 0, state: 'running', destination: {channelCount: 2},
    createGain: () => ({gain: param(), connect() {}, disconnect() {}}),
    createBuffer: (channels: number, length: number, rate: number) => ({numberOfChannels: channels, length, sampleRate: rate, data: Array.from({length: channels}, () => new Float32Array(length)), copyToChannel(src: Float32Array, ch: number) { this.data[ch].set(src); }}),
    createBufferSource: () => {
      const node: any = {buffer: null, onended: null, started: undefined, connect() {}, disconnect() {}, start(t: number) { node.started = t; scheduled.push({start: Math.round(t * sampleRate), frames: node.buffer.length, node}); }, stop() { node.stopped = true; }};
      return node;
    },
  };
  return {
    ctx, scheduled,
    advance(seconds: number) {
      ctx.currentTime += seconds;
      const frame = Math.round(ctx.currentTime * sampleRate);
      for (const s of scheduled) if (!s.node.done && !s.node.stopped && s.start + s.frames <= frame) { s.node.done = true; s.node.onended?.(); }
    },
    /** Frames the scheduled buffers hold that haven't played yet. */
    unplayed() { const frame = Math.round(ctx.currentTime * sampleRate); return scheduled.filter(s => !s.node.done && !s.node.stopped).reduce((n, s) => n + Math.min(s.frames, s.start + s.frames - Math.max(frame, s.start)), 0); },
  };
}

/** A synthetic PCM source: `frames` of a sine, generated as read (no memory of its own). */
export function sineSource(frames: number, sampleRate: number, channels = 2) {
  let at = 0, closed = false;
  return {
    sampleRate, channels, get closed() { return closed; },
    async read(max: number) {
      if (at >= frames) return undefined;
      const n = Math.min(max, frames - at), c = Array.from({length: channels}, () => new Float32Array(n));
      for (let i = 0; i < n; i++) { const v = 0.5 * Math.sin(2 * Math.PI * 441 * (at + i) / sampleRate); for (const ch of c) ch[i] = v; }
      at += n;
      return {channels: c, frames: n};
    },
    close() { closed = true; },
  };
}
