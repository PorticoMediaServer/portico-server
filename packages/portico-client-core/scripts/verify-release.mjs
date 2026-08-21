import { spawnSync } from "node:child_process";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const packageRoot = resolve(fileURLToPath(new URL("..", import.meta.url)));
const checks = [
  ["npm", ["run", "api:types:check"]],
  ["npm", ["run", "product-language:check:all"]],
  ["npm", ["run", "lint"]],
  ["npm", ["run", "build"]],
  ["npm", ["run", "coverage"]],
  ["npm", ["run", "exports:check"]],
  ["npm", ["run", "audit:verify"]]
];

for (const [command, args] of checks) {
  const result = spawnSync(command, args, { cwd: packageRoot, stdio: "inherit" });
  if (result.status !== 0) process.exit(result.status ?? 1);
}
console.log("Client Core release verification passed.");
