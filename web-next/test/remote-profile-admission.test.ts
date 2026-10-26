import test from 'node:test';
import assert from 'node:assert/strict';
import type {RemoteSource, SourceReadSettings} from '@core/remote-sources.ts';
import {sourceReadPreset} from '@core/remote-sources.ts';
import {AnalysisConsentDraft, ProfileAdmission} from '../src/screens/settings/remote-sources/profile-admission.ts';

// Authored UNRUN. These exercise the production admission used by the React
// holder, including exact record replacement and original-flight settlement.
const source: RemoteSource = Object.freeze({id: 'source', name: 'source', kind: 'webdav', rootPath: '/', generation: 3,
  state: 'ready', rangeSupport: 'conditional', insecureLocal: false, credentialPresent: true});
const profile = (consent?: boolean, revision = 4): SourceReadSettings => Object.freeze({sourceId: source.id, generation: '3', revision, saved: true,
  values: Object.freeze({...sourceReadPreset('once'), wholeFileReads: true, ...(consent === undefined ? {} : {allowAdvancedMediaAnalysis: consent})})});
const install = (admission: ProfileAdmission, value: SourceReadSettings) => {
  assert.equal(admission.replace(undefined, {token: admission, profile: value, consent: value.values.allowAdvancedMediaAnalysis, values: value.values}), true);
  return admission.snapshot()!;
};

test('old source and Strict replay tokens never edit or settle a new admission', () => {
  const a = new ProfileAdmission(source, () => true), old = install(a, profile(false));
  const flight = a.begin(old)!; a.retire();
  const b = new ProfileAdmission(Object.freeze({...source, generation: 5}), () => true);
  assert.equal(b.replace(undefined, {token: b, profile: profile(false), values: profile(false).values}), false);
  const current = install(b, {...profile(false), generation: '5'});
  assert.equal(a.replace(old, {...old, consent: true}), false);
  assert.equal(a.replace(old, {...old, profile: profile(true)}, flight), false);
  assert.equal(a.finish(flight), false);
  assert.equal(b.holds(current), true);assert.equal(current.consent, false);
});

test('a retained same-opening checkbox cannot edit the successor profile record', () => {
  const a = new ProfileAdmission(source, () => true), original = install(a, profile(false));
  assert.equal(a.replace(original, {...original, consent: true}), true);
  const changed = a.snapshot()!;
  assert.equal(a.replace(original, {...original, consent: false}), false);
  assert.equal(a.holds(changed), true);assert.equal(changed.consent, true);
  const flight = a.begin(changed)!;
  assert.equal(a.replace(changed, {...changed, consent: false}), false);
  assert.equal(a.begin(changed), undefined);
  const accepted = profile(true, 5);
  assert.equal(a.replace(changed, {token: a, profile: accepted, consent: true, values: accepted.values}, flight), true);
  assert.equal(a.finish(flight), true);
  assert.equal(a.replace(changed, {...changed, consent: false}), false);
  assert.equal(a.snapshot()?.profile, accepted);
});

test('late original flight cannot release a successor flight or revive replaced authority', () => {
  let current = true;
  const a = new ProfileAdmission(source, () => current), record = install(a, profile(false));
  const first = a.begin(record)!;assert.equal(a.finish(first), true);
  const second = a.begin(record)!;
  assert.equal(a.finish(first), false);assert.equal(a.owns(second), true);
  current = false;
  assert.equal(a.replace(record, {...record, consent: true}, second), false);
  assert.equal(a.finish(second), false);
  const broken = new ProfileAdmission(source, () => {throw new Error('retired');});
  assert.equal(broken.current(), false);
});

test('unknown consent stays absent; every preset preserves independent consent and legacy wire value', () => {
  const unknown = new AnalysisConsentDraft(profile(), () => true);
  assert.equal(unknown.value(), undefined);assert.equal(unknown.change(true), false);
  assert.equal(Object.hasOwn(unknown.values({...sourceReadPreset('everything'), allowAdvancedMediaAnalysis: true}), 'allowAdvancedMediaAnalysis'), false);
  for (const name of ['once', 'list', 'everything', 'ask'] as const) {
    for (const consent of [false, true]) {
      const draft = new AnalysisConsentDraft(profile(consent), () => true);
      const body = draft.values(sourceReadPreset(name));
      assert.equal(body.wholeFileReads, true);assert.equal(Object.hasOwn(body, 'allowAdvancedMediaAnalysis'), false);
      assert.equal(draft.change(!consent), true);assert.equal(draft.values(sourceReadPreset(name)).allowAdvancedMediaAnalysis, !consent);
      draft.retire();assert.equal(draft.change(!consent), false);assert.throws(() => draft.values(sourceReadPreset(name)));
    }
  }
});
