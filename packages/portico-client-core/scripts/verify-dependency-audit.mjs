import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { dependencyWaiverKey, findUnwaivedFindings } from "./dependency-audit-lib.mjs";

const packageRoot = resolve(fileURLToPath(new URL("..", import.meta.url)));
const policy = JSON.parse(readFileSync(resolve(packageRoot, "scripts/dependency-audit-policy.json"), "utf8"));
if (policy.version !== 1 || !Array.isArray(policy.waivers)) throw new Error("Dependency audit policy is invalid.");

const waivers = new Map();
for (const waiver of policy.waivers) {
  if (!waiver || typeof waiver.advisory !== "string" || !waiver.advisory.trim() || typeof waiver.package !== "string" || !waiver.package.trim() || typeof waiver.scope !== "string" || !waiver.scope.trim() || typeof waiver.reason !== "string" || !waiver.reason.trim() || typeof waiver.mitigation !== "string" || !waiver.mitigation.trim() || typeof waiver.owner !== "string" || !waiver.owner.trim() || !Number.isFinite(Date.parse(waiver.expiresAt))) {
    throw new Error("Every dependency waiver requires an advisory, package, scope, reason, mitigation, owner, and valid expiry.");
  }
  if (Date.parse(waiver.expiresAt) <= Date.now()) throw new Error(`Dependency waiver ${waiver.advisory} has expired.`);
  const key = dependencyWaiverKey(waiver.advisory, waiver.package, waiver.scope);
  if (waivers.has(key)) throw new Error(`Dependency waiver ${waiver.advisory} is duplicated for ${waiver.package}/${waiver.scope}.`);
  waivers.set(key, waiver);
}

audit("runtime", ["audit", "--omit=dev", "--json"]);
audit("build/dev", ["audit", "--json"]);
console.log("Client Core runtime and build dependency audits passed.");

function audit(scope, args) {
  const result = spawnSync("npm", args, { cwd: packageRoot, encoding: "utf8" });
  let report;
  try {
    report = JSON.parse(result.stdout);
  } catch {
    throw new Error(`npm ${args.join(" ")} did not return a readable JSON report: ${result.stderr.trim()}`);
  }
  if (report.error) throw new Error(`Client Core ${scope} dependency audit could not run: ${report.error.summary ?? report.error}`);

  const unwaived = findUnwaivedFindings(report.vulnerabilities, waivers, scope);
  if (unwaived.length > 0) {
    const summary = unwaived.map((finding) => {
      const advisoryIds = finding.unwaivedAdvisories.length > 0
        ? finding.unwaivedAdvisories.map((advisory) => `${advisory.advisoryId} (${advisory.packageName})`).join(", ")
        : finding.advisoryIds.join(", ") || "no advisory id";
      const suffix = finding.unresolved ? "; unresolved aggregate dependency" : "";
      return `${finding.name} (${finding.severity}; ${advisoryIds}${suffix})`;
    });
    throw new Error(`Client Core ${scope} dependency audit has unwaived findings: ${summary.join("; ")}`);
  }
  if (result.status !== 0 && Object.keys(report.vulnerabilities ?? {}).length === 0) throw new Error(`Client Core ${scope} dependency audit exited with ${result.status}.`);
}
