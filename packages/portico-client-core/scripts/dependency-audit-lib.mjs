export function resolveAdvisories(vulnerabilities, name, stack = new Set()) {
  const vulnerability = vulnerabilities[name];
  if (!vulnerability) return { advisories: [], advisoryIds: [], unresolved: true };
  if (stack.has(name)) return { advisories: [], advisoryIds: [], unresolved: false };

  const nextStack = new Set(stack);
  nextStack.add(name);
  const advisories = (vulnerability.via ?? []).flatMap((entry) => {
    if (!entry || typeof entry !== "object") return [];
    const advisoryId = String(entry.source ?? entry.url ?? "");
    return advisoryId ? [{ advisoryId, packageName: name }] : [];
  });
  let unresolved = false;

  for (const dependency of vulnerability.via ?? []) {
    if (typeof dependency !== "string") continue;
    const child = resolveAdvisories(vulnerabilities, dependency, nextStack);
    advisories.push(...child.advisories);
    unresolved ||= child.unresolved;
  }

  const uniqueAdvisories = [...new Map(advisories.map((advisory) => [`${advisory.advisoryId}\0${advisory.packageName}`, advisory])).values()];
  return {
    advisories: uniqueAdvisories,
    advisoryIds: [...new Set(uniqueAdvisories.map((advisory) => advisory.advisoryId))],
    unresolved
  };
}

export function dependencyWaiverKey(advisory, packageName, scope) {
  return `${advisory}\0${packageName}\0${scope}`;
}

export function findUnwaivedFindings(vulnerabilities, waivers, scope) {
  const findings = [];
  for (const [name, vulnerability] of Object.entries(vulnerabilities ?? {})) {
    const resolution = resolveAdvisories(vulnerabilities, name);
    const missingAdvisory = resolution.unresolved || resolution.advisoryIds.length === 0;
    const unwaivedAdvisories = resolution.advisories.filter((advisory) => !waivers.has(dependencyWaiverKey(advisory.advisoryId, advisory.packageName, scope)));
    if (missingAdvisory || unwaivedAdvisories.length > 0) {
      findings.push({
        name,
        severity: vulnerability.severity ?? "unknown",
        advisoryIds: resolution.advisoryIds,
        unwaivedAdvisories,
        unresolved: resolution.unresolved
      });
    }
  }
  return findings;
}
