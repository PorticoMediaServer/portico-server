import { mkdtemp, readFile, rm } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const packageRoot = resolve(fileURLToPath(new URL("..", import.meta.url)));
const temporaryRoot = await mkdtemp(join(tmpdir(), "portico-api-types-"));
const inputs = [
  ["../../api/openapi/portico-server.openapi.json", "src/openapi-types.ts"],
  ["../../api/openapi/hosted/portico-hosted.openapi.json", "src/hosted-openapi-types.ts"]
];

try {
  for (const [specification, committed] of inputs) {
    const generated = join(temporaryRoot, committed.split("/").at(-1));
    const result = spawnSync(join(packageRoot, "node_modules", ".bin", "openapi-typescript"), [specification, "-o", generated], { cwd: packageRoot, encoding: "utf8" });
    if (result.status !== 0) throw new Error(result.stderr || `Could not generate ${committed}`);
    if (await readFile(generated, "utf8") !== await readFile(join(packageRoot, committed), "utf8")) {
      throw new Error(`${committed} is stale; run npm run api:types`);
    }
  }
  console.log("Public Server and Hosted protocol declarations are fresh.");
} finally {
  await rm(temporaryRoot, { recursive: true, force: true });
}
