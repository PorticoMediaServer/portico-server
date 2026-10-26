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
