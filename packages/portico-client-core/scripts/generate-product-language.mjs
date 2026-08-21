import { readFile, writeFile } from "node:fs/promises";
import { parseUniqueJSON } from "./parse-unique-json.mjs";

const sourceURL = new URL("../../../api/product-language/en-US.json", import.meta.url);
const outputURL = new URL("../src/productLanguageCatalog.generated.ts", import.meta.url);
const serverEmbedURL = new URL("../../../internal/app/productlanguage/catalog.en-US.json", import.meta.url);
const clientOnly = process.argv.includes("--client-only") || process.env.PORTICO_CLIENT_CORE_ONLY === "1";
const checkOnly = process.argv.includes("--check");
const catalog = parseUniqueJSON(await readFile(sourceURL, "utf8"), "Product language catalog");

if (!catalog || typeof catalog !== "object" || Array.isArray(catalog)) throw new Error("Product language catalog must be an object.");
if (catalog.revision !== "v1") throw new Error("The unreleased Product Language contract must remain the single coordinated v1.");
if (catalog.locale !== "en-US" || catalog.fallbackLocale !== "en-US") throw new Error("The first product language catalog must define en-US as its locale and fallback.");
if (catalog.iconFamily !== "lucide") throw new Error("Portico's canonical semantic icon family is Lucide.");

const icons = catalog.icons ?? {};
const messages = catalog.messages ?? {};
for (const [id, icon] of Object.entries(icons)) {
  if (!/^[a-z][a-z0-9.-]*$/.test(id)) throw new Error(`Invalid semantic icon ID: ${id}`);
  if (!icon || typeof icon !== "object" || typeof icon.glyph !== "string" || typeof icon.label !== "string") {
    throw new Error(`Invalid semantic icon definition: ${id}`);
  }
}
for (const [id, message] of Object.entries(messages)) {
  if (!/^[a-z][a-z0-9.-]*$/.test(id)) throw new Error(`Invalid product message ID: ${id}`);
  if (!message || typeof message !== "object" || (typeof message.text !== "string" && typeof message.title !== "string")) {
    throw new Error(`Invalid product message definition: ${id}`);
  }
  if (message.icon && !icons[message.icon]) throw new Error(`Message ${id} references unknown icon ${message.icon}.`);
  for (const action of message.actions ?? []) {
    if (!messages[action]?.text) throw new Error(`Message ${id} references unknown action ${action}.`);
  }
}

const output = `// Generated from api/product-language/en-US.json. Do not edit by hand.\nexport const productLanguageCatalog = ${JSON.stringify(catalog, null, 2)} as const;\n`;
const serverOutput = `${JSON.stringify(catalog, null, 2)}\n`;

async function assertCurrent(url, expected, label) {
  let current;
  try {
    current = await readFile(url, "utf8");
  } catch (error) {
    if (error?.code === "ENOENT") throw new Error(`${label} is missing. Run the explicit Product Language generator.`);
    throw error;
  }
  if (current !== expected) throw new Error(`${label} is stale. Run the explicit Product Language generator.`);
}

if (checkOnly) {
  await assertCurrent(outputURL, output, "Client Product Language catalog");
  if (!clientOnly) await assertCurrent(serverEmbedURL, serverOutput, "Server Product Language catalog");
} else {
  await writeFile(outputURL, output, "utf8");
  if (!clientOnly) await writeFile(serverEmbedURL, serverOutput, "utf8");
}
