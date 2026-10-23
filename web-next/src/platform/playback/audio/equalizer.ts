/** MC7 engine contract. These octave centres match AutoEq's fixed ten-band export. */
export const eqFrequencies = Object.freeze([31.25, 62.5, 125, 250, 500, 1000, 2000, 4000, 8000, 16000]);
export type EqualizerSettings = Readonly<{enabled: boolean; preampDb: number; bandsDb: readonly number[]}>;
export const flatEqualizer: EqualizerSettings = Object.freeze({enabled: false, preampDb: 0, bandsDb: Object.freeze(eqFrequencies.map(() => 0))});

/** Reject corrupt persisted/input data rather than turning an invalid preset into an audible one. */
export function parseEqualizer(value: unknown): EqualizerSettings {
  const v = value as Partial<EqualizerSettings> | null;
  if (!v || typeof v.enabled !== 'boolean' || typeof v.preampDb !== 'number' || !Number.isFinite(v.preampDb) || v.preampDb < -24 || v.preampDb > 24 || !Array.isArray(v.bandsDb) || v.bandsDb.length !== 10 || !v.bandsDb.every(n => typeof n === 'number' && Number.isFinite(n) && n >= -12 && n <= 12)) throw new Error('invalid_equalizer');
  return Object.freeze({enabled: v.enabled, preampDb: v.preampDb, bandsDb: Object.freeze([...v.bandsDb])});
}

/** Import an AutoEq fixed-band export supplied locally. No measurements are downloaded or
 * redistributed here. Arbitrary parametric exports cannot be silently treated as these bands. */
export function parseAutoEqFixedBand(text: string): EqualizerSettings {
  if (text.length > 16384) throw new Error('invalid_equalizer');
  const lines = text.trim().split(/\r?\n/).filter(line => line.trim() && !line.startsWith('#'));
  const preamp = /^Preamp:\s*(-?\d+(?:\.\d+)?)\s*dB$/i.exec(lines.shift() ?? '');
  if (!preamp || lines.length !== 10) throw new Error('invalid_equalizer');
  const bandsDb = lines.map((line, i) => {
    const band = /^Filter\s+(\d+):\s*ON\s+PK\s+Fc\s+(\d+(?:\.\d+)?)\s*Hz\s+Gain\s+(-?\d+(?:\.\d+)?)\s*dB\s+Q\s+(\d+(?:\.\d+)?)$/i.exec(line.trim());
    if (!band || Number(band[1]) !== i + 1 || Math.abs(Number(band[2]) / eqFrequencies[i]! - 1) > 0.02 || Math.abs(Number(band[4]) - Math.SQRT2) > 0.02) throw new Error('invalid_equalizer');
    return Number(band[3]);
  });
  return parseEqualizer({enabled:false,preampDb:Number(preamp[1]),bandsDb});
}

/** One shared graph across decks: filtering never resets at a gapless boundary. Off is a dry path. */
export class BrowserEqualizer {
  readonly input: GainNode;
  readonly output: GainNode;
  private readonly dry: GainNode;
  private readonly wet: GainNode;
  private preamp?: GainNode;
  private filters: BiquadFilterNode[] = [];
  private value = flatEqualizer;

  constructor(private readonly context: BaseAudioContext) {
    this.input = context.createGain(); this.output = context.createGain();
    this.dry = context.createGain(); this.wet = context.createGain();
    this.wet.gain.value = 0;
    this.input.connect(this.dry); this.dry.connect(this.output); this.wet.connect(this.output);
  }
  get settings(): EqualizerSettings { return this.value; }
  apply(settings: EqualizerSettings): void {
    const value = parseEqualizer(settings), time = this.context.currentTime;
    if (value.enabled && !this.preamp) {
      const preamp = this.context.createGain(), filters: BiquadFilterNode[] = [];
      let previous: AudioNode = preamp;
      try { eqFrequencies.forEach(frequency => {
        const filter = this.context.createBiquadFilter();
        filter.type = 'peaking'; filter.frequency.value = frequency;
        // One-octave bandwidth, Q = sqrt(2). All channels are processed independently.
        filter.Q.value = Math.SQRT2;
        previous.connect(filter); previous = filter;
        filters.push(filter);
      });
      previous.connect(this.wet); this.input.connect(preamp);
      this.preamp = preamp; this.filters = filters;
      } catch (error) { preamp.disconnect(); filters.forEach(filter => filter.disconnect()); throw error; }
    }
    const set = (param: AudioParam, target: number) => { param.cancelScheduledValues(time); param.setTargetAtTime(target, time, 0.005); };
    if (this.preamp) {
      set(this.preamp.gain, 10 ** (value.preampDb / 20));
      this.filters.forEach((filter, i) => set(filter.gain, filter.frequency.value < this.context.sampleRate / 2 ? value.bandsDb[i]! : 0));
    }
    set(this.dry.gain, value.enabled ? 0 : 1); set(this.wet.gain, value.enabled ? 1 : 0);
    this.value = value;
  }
  dispose(): void {
    for (const node of [this.input, this.output, this.dry, this.wet, this.preamp, ...this.filters]) node?.disconnect();
  }
}
