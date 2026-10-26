import type {ReadItemEligibility} from './navigation';

/** The already-authorized eligibility row, never a fresh probe. */
export type ReadVersion = ReadItemEligibility['versions'][number];

/** A chooser exists only when the item really has more than one version. */
export function readVersionCanChoose(versions: readonly ReadVersion[]): boolean {
  return versions.length > 1;
}

/** The versions an owner may switch to: real, inventory-available and not the current one. */
export function readVersionAlternatives(versions: readonly ReadVersion[], chosen: string | undefined): readonly ReadVersion[] {
  return versions.filter(version => version.sourceId !== chosen && version.inventoryAvailable);
}

/** The exact version a selection may dispatch, or undefined when it is absent, duplicated,
 * unavailable or already the chosen one. It reads only the supplied eligibility list. */
export function readVersionPick(versions: readonly ReadVersion[], chosen: string | undefined, next: string | undefined): ReadVersion | undefined {
  if (!next || next === chosen) return undefined;
  const matches = versions.filter(version => version.sourceId === next);
  return matches.length === 1 && matches[0]!.inventoryAvailable ? matches[0] : undefined;
}

/** The component's own admission for one explicit version change. It refuses a
 * callback from an older render (retained menu), a replaced source or version list,
 * and a successor attempt, while still allowing recovery from a failed or
 * not-ready original. */
export type ReadVersionAdmission = Readonly<{
  liveRoute: boolean;
  renderedContext: unknown;
  currentContext: unknown;
  renderedSourceId: string | undefined;
  currentSourceId: string | undefined;
  renderedAttempt: unknown;
  currentAttempt: unknown;
  attemptRetired: boolean;
  candidate: ReadVersion;
  offered: readonly ReadVersion[];
}>;

export function readVersionAdmission(facts: ReadVersionAdmission): boolean {
  if (!facts.liveRoute || facts.renderedContext !== facts.currentContext) return false;
  if (facts.renderedSourceId === undefined || facts.renderedSourceId !== facts.currentSourceId) return false;
  if (facts.renderedAttempt !== facts.currentAttempt || facts.attemptRetired) return false;
  const matches = facts.offered.filter(version => version.sourceId === facts.candidate.sourceId);
  return matches.length === 1 && matches[0] === facts.candidate && facts.candidate.inventoryAvailable && facts.candidate.sourceId !== facts.currentSourceId;
}

/** Back returns in-app only when the captured route really has a predecessor. */
export function readBackDestination(canGoBack: boolean): 'back' | 'home' {
  return canGoBack ? 'back' : 'home';
}
