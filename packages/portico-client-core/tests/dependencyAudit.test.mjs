import assert from "node:assert/strict";
import test from "node:test";
import { dependencyWaiverKey, findUnwaivedFindings, resolveAdvisories } from "../scripts/dependency-audit-lib.mjs";

test("resolves aggregate npm audit findings to known leaf advisories", () => {
  const vulnerabilities = {
    "image-size": { severity: "high", via: [{ source: 1138808 }, { source: 1138809 }] },
    metro: { severity: "high", via: ["image-size", "metro-config"] },
    "metro-config": { severity: "high", via: ["metro"] }
  };

  assert.deepEqual(resolveAdvisories(vulnerabilities, "metro"), {
    advisories: [
      { advisoryId: "1138808", packageName: "image-size" },
      { advisoryId: "1138809", packageName: "image-size" }
    ],
    advisoryIds: ["1138808", "1138809"],
    unresolved: false
  });
  const waivers = new Map([
    [dependencyWaiverKey("1138808", "image-size", "build/dev"), {}],
    [dependencyWaiverKey("1138809", "image-size", "build/dev"), {}]
  ]);
  assert.deepEqual(findUnwaivedFindings(vulnerabilities, waivers, "build/dev"), []);
});

test("partial, wrong-package, or wrong-scope waivers fail closed", () => {
  const vulnerabilities = { "image-size": { severity: "high", via: [{ source: 1138808 }, { source: 1138809 }] } };
  const partial = new Map([[dependencyWaiverKey("1138808", "image-size", "build/dev"), {}]]);
  const wrongPackage = new Map([[dependencyWaiverKey("1138808", "metro", "build/dev"), {}]]);
  const wrongScope = new Map([[dependencyWaiverKey("1138808", "image-size", "runtime"), {}]]);
  assert.equal(findUnwaivedFindings(vulnerabilities, partial, "build/dev")[0].unwaivedAdvisories.length, 1);
  assert.equal(findUnwaivedFindings(vulnerabilities, wrongPackage, "build/dev")[0].unwaivedAdvisories.length, 2);
  assert.equal(findUnwaivedFindings(vulnerabilities, wrongScope, "build/dev")[0].unwaivedAdvisories.length, 2);
});

test("does not treat an unknown aggregate dependency as safely waived", () => {
  const vulnerabilities = {
    metro: { severity: "high", via: ["unpublished-parser"] }
  };

  const findings = findUnwaivedFindings(vulnerabilities, new Map(), "build/dev");
  assert.equal(findings.length, 1);
  assert.equal(findings[0].unresolved, true);
});
