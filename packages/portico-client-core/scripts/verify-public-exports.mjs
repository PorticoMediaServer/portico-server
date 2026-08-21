import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const packageRoot = resolve(fileURLToPath(new URL("..", import.meta.url)));
const snapshot = JSON.parse(await readFile(new URL("./public-exports.snapshot.json", import.meta.url), "utf8"));
const manifest = JSON.parse(await readFile(resolve(packageRoot, "package.json"), "utf8"));
const subpaths = Object.keys(manifest.exports ?? {}).sort();
const rootExports = Object.keys(await import(pathToFileURL(resolve(packageRoot, "dist/index.js")))).sort();
const digest = createHash("sha256").update(rootExports.join("\n")).digest("hex");

if (JSON.stringify(subpaths) !== JSON.stringify(snapshot.subpaths)) {
  throw new Error(`Public package subpaths changed: ${subpaths.join(", ")}`);
}
if (rootExports.length !== snapshot.rootExportCount || digest !== snapshot.rootExportSha256) {
  throw new Error(`Root public exports changed (${rootExports.length}, ${digest}); review intentionally before updating the snapshot.`);
}
console.log(`Public API snapshot passed (${rootExports.length} root exports, ${subpaths.length} subpaths).`);
