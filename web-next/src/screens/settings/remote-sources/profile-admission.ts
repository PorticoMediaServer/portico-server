import type {RemoteSource, SourceReadSettings, SourceReadValues} from '@core/remote-sources.ts';

export type HeldProfile = Readonly<{token: object; profile: SourceReadSettings; consent?: boolean; consentDirty?: boolean; values: SourceReadValues}>;

/** Private to one committed source opening. Publication and editing compare the
 * exact record, so neither an old handler nor an old read can adopt a successor. */
export class ProfileAdmission {
  private live = true;
  private record?: HeldProfile;
  private flight?: object;
  constructor(readonly source: RemoteSource, private readonly isCurrent: () => boolean) {}
  current = () => {try {return this.live && this.isCurrent();} catch {return false;}};
  opening = () => this.live;
  holds = (expected: HeldProfile | undefined) => this.current() && this.record === expected;
  replace = (expected: HeldProfile | undefined, next: HeldProfile | undefined, flight?: object) => {
    if (!this.holds(expected) || this.flight && this.flight !== flight || next && (next.token !== this || next.profile.sourceId !== this.source.id)) return false;
    if (next && (!expected && next.profile.generation !== String(this.source.generation) ||
      next.values.wholeFileReads !== next.profile.values.wholeFileReads ||
      (next.profile.values.allowAdvancedMediaAnalysis === undefined ? next.consent !== undefined : typeof next.consent !== 'boolean'))) return false;
    this.record = next && Object.freeze({...next, values: Object.freeze({...next.values})});
    return true;
  };
  snapshot = () => this.record;
  begin = (expected: HeldProfile) => {
    if (!this.holds(expected) || this.flight) return undefined;
    return this.flight = {};
  };
  owns = (flight: object) => this.current() && this.flight === flight;
  finish = (flight: object) => {
    if (this.flight !== flight) return false;
    this.flight = undefined;return this.current();
  };
  retire = () => {this.live = false;this.record = undefined;};
}

/** A wizard profile has no published source row yet. Its permission draft still
 * belongs to the exact fetched profile and original form admission. */
export class AnalysisConsentDraft {
  private live = true;
  private consent: boolean | undefined;
  private dirty = false;
  constructor(readonly profile: SourceReadSettings, private readonly isCurrent: () => boolean) {this.consent = profile.values.allowAdvancedMediaAnalysis;}
  current = () => {try {return this.live && this.isCurrent();} catch {return false;}};
  value = () => this.consent;
  change = (value: boolean) => {
    if (!this.current() || this.consent === undefined) return false;
    this.consent = value;this.dirty = true;return true;
  };
  values = (values: SourceReadValues): SourceReadValues => {
    if (!this.current()) throw new DOMException('Source profile retired', 'AbortError');
    const {allowAdvancedMediaAnalysis: _consent, ...read} = values;
    return Object.freeze({...read, wholeFileReads: this.profile.values.wholeFileReads,
      ...(!this.dirty || this.consent === undefined ? {} : {allowAdvancedMediaAnalysis: this.consent})});
  };
  retire = () => {this.live = false;};
}
