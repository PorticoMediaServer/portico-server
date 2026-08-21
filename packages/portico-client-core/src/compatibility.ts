import { foundationContract } from "./foundationContract.generated.js";
import type { HostedSystemInfo, ProductContract, SystemStatusResponse } from "./types.js";

export const PORTICO_API_VERSION = "v1" as const;
export const PORTICO_CLIENT_PROTOCOL = foundationContract.compatibility.supportedClientProtocol.maximum;
export const PORTICO_FOUNDATION_COMPATIBILITY = { ...foundationContract.compatibility, build: foundationContract.build, forwardCompatibility: foundationContract.forwardCompatibility } as const;
export type CompatibilityEnvelope = SystemStatusResponse["compatibility"];
export type PorticoCompatibilityErrorCode = "invalid_compatibility_envelope" | "unsupported_client_protocol" | "api_contract_digest_mismatch" | "required_semantic_unknown" | "required_semantic_incompatible" | "unsafe_forward_compatibility_policy" | "system_product_contract_mismatch";

export class PorticoCompatibilityError extends Error {
  constructor(readonly code: PorticoCompatibilityErrorCode, message: string) { super(message); this.name = "PorticoCompatibilityError"; }
}
export class HostedCompatibilityError extends PorticoCompatibilityError {
  constructor(code: PorticoCompatibilityErrorCode, message: string) { super(code, message); this.name = "HostedCompatibilityError"; }
}
export interface CompatibilityCapability { readonly id: string; readonly revision: number; readonly state: "available" | "requires_configuration" | "degraded" | "unavailable"; readonly requiredSemantics: readonly string[]; }
export interface CompatibilityDiagnostic { code: "api_contract_digest_mismatch"; clientDigest: string; serverDigest: string; policy: "allow_semantic_overlap"; }
export interface ProductContractCompatibility {
  apiVersion: string; clientProtocol: number; serverCapabilities: readonly string[]; capabilities: readonly CompatibilityCapability[];
  build: CompatibilityEnvelope["build"]; semanticRevisions: Readonly<Record<string, number>>; diagnostics: readonly CompatibilityDiagnostic[];
}
export type ServerAPICompatibility = ProductContractCompatibility;
export type PorticoCompatibility = ProductContractCompatibility;
export type HostedServicesCompatibility = ProductContractCompatibility;

export function assertHostedServicesCompatibility(system: Pick<HostedSystemInfo, "name" | "status" | "apiVersion" | "compatibility">): HostedServicesCompatibility {
  if (!system || system.name !== "Portico" || system.status !== "ok") throw new HostedCompatibilityError("invalid_compatibility_envelope", "The Hosted endpoint did not identify a healthy Portico service.");
  try { return evaluateEnvelope(system.apiVersion, system.compatibility); }
  catch (error) { if (error instanceof PorticoCompatibilityError) throw new HostedCompatibilityError(error.code, error.message); throw error; }
}
export function assertServerAPICompatibility(status: Pick<SystemStatusResponse, "apiVersion" | "compatibility">): ServerAPICompatibility { return evaluateEnvelope(status?.apiVersion, status?.compatibility); }
export function assertProductContractCompatibility(contract: Pick<ProductContract, "apiVersion" | "serverCapabilities" | "compatibility">): ProductContractCompatibility {
  const result = evaluateEnvelope(contract?.apiVersion, contract?.compatibility);
  const productCapabilities = requireStringSet(contract?.serverCapabilities, "serverCapabilities", true);
  const advertised = result.capabilities.filter(({ state }) => state === "available").map(({ id }) => id);
  if (!sameStrings(productCapabilities, advertised)) throw new PorticoCompatibilityError("system_product_contract_mismatch", "The Product Contract capability list does not exactly match its available compatibility capabilities.");
  return result;
}
export function evaluatePorticoCompatibility(status: Pick<SystemStatusResponse, "apiVersion" | "compatibility">, contract: Pick<ProductContract, "apiVersion" | "serverCapabilities" | "compatibility">): PorticoCompatibility {
  const system = assertServerAPICompatibility(status); const product = assertProductContractCompatibility(contract);
  if (canonical(status.compatibility) !== canonical(contract.compatibility)) throw new PorticoCompatibilityError("system_product_contract_mismatch", "System and Product Contract compatibility identities differ.");
  return { ...product, diagnostics: Object.freeze([...system.diagnostics]) };
}
export function supportsServerCapability(compatibility: Pick<ProductContractCompatibility, "serverCapabilities">, capability: string): boolean { return compatibility.serverCapabilities.includes(capability.trim()); }

function evaluateEnvelope(apiVersion: unknown, rawEnvelope: unknown): ProductContractCompatibility {
  if (apiVersion !== PORTICO_API_VERSION || !isRecord(rawEnvelope)) invalid("Portico returned an unsupported compatibility envelope.");
  const envelope = rawEnvelope as Record<string, unknown>;
  if (envelope.envelopeRevision !== foundationContract.compatibility.envelopeRevision) invalid("The compatibility envelope revision is missing or unsupported.");
  const range = requireRecord(envelope.supportedClientProtocol, "supportedClientProtocol");
  const minimum = positiveInteger(range.minimum, "supportedClientProtocol.minimum"); const maximum = positiveInteger(range.maximum, "supportedClientProtocol.maximum");
  if (minimum > maximum || PORTICO_CLIENT_PROTOCOL < minimum || PORTICO_CLIENT_PROTOCOL > maximum) throw new PorticoCompatibilityError("unsupported_client_protocol", `This client uses protocol ${PORTICO_CLIENT_PROTOCOL}; the server supports ${minimum}-${maximum}.`);

  const apiContract = requireRecord(envelope.apiContract, "apiContract");
  const algorithm = stringValue(apiContract.digestAlgorithm, "apiContract.digestAlgorithm");
  const identity = stringValue(apiContract.identity, "apiContract.identity");
  const digest = stringValue(apiContract.digest, "apiContract.digest");
  if (algorithm !== "sha256" || identity !== foundationContract.compatibility.apiContract.identity || !/^[a-f0-9]{64}$/.test(digest)) invalid("The API contract identity is malformed or unsupported.");
  const policy = requireRecord(envelope.forwardCompatibility, "forwardCompatibility");
  if (policy.authorizationOnPartialUpgrade !== "never_broaden" || policy.unknownOptionalCapabilities !== "ignore_and_preserve" || policy.unknownRequiredSemantics !== "reject_actionably" || policy.apiContractDigestMismatch !== "allow_semantic_overlap") throw new PorticoCompatibilityError("unsafe_forward_compatibility_policy", "The server compatibility policy could broaden authorization or hide required incompatibility.");

  const build = requireBuild(envelope.build); const semanticRevisions = requireRevisionMap(envelope.semanticRevisions);
  const requiredSemantics = requireStringSet(envelope.requiredSemantics, "requiredSemantics", true);
  for (const name of requiredSemantics) {
    const clientRevision = (foundationContract.compatibility.semanticRevisions as Record<string, number>)[name];
    if (clientRevision === undefined) throw new PorticoCompatibilityError("required_semantic_unknown", `The server requires unknown semantic ${name}.`);
    if (semanticRevisions[name] !== clientRevision) throw new PorticoCompatibilityError("required_semantic_incompatible", `Semantic ${name} is incompatible with this client.`);
  }
	const foundationSemantics = Object.entries(foundationContract.compatibility.semanticRevisions as Record<string, number>);
	for (const [name, revision] of foundationSemantics) if (semanticRevisions[name] !== revision) throw new PorticoCompatibilityError("required_semantic_incompatible", `Semantic ${name} is missing or incompatible with this client.`);
	if (!sameStrings(requiredSemantics, foundationSemantics.map(([name]) => name))) throw new PorticoCompatibilityError("required_semantic_incompatible", "The server does not require the complete Foundation semantic set.");
  const capabilities = requireCapabilities(envelope.capabilities, semanticRevisions);
  const serverCapabilities = Object.freeze(capabilities.filter(({ state }) => state === "available").map(({ id }) => id));
  const diagnostics: CompatibilityDiagnostic[] = digest === foundationContract.compatibility.apiContract.digest ? [] : [{ code: "api_contract_digest_mismatch", clientDigest: foundationContract.compatibility.apiContract.digest, serverDigest: digest, policy: "allow_semantic_overlap" }];
  return Object.freeze({ apiVersion: PORTICO_API_VERSION, clientProtocol: PORTICO_CLIENT_PROTOCOL, serverCapabilities, capabilities, build, semanticRevisions: Object.freeze(semanticRevisions), diagnostics: Object.freeze(diagnostics) });
}

function requireCapabilities(value: unknown, semantics: Record<string, number>): readonly CompatibilityCapability[] {
	if (!Array.isArray(value) || value.length === 0 || value.length > 128) invalid("capabilities must be a bounded nonempty array.");
  const seen = new Set<string>();
  return Object.freeze(value.map((entry, index) => {
    const capability = requireRecord(entry, `capabilities[${index}]`); const id = stringValue(capability.id, `capabilities[${index}].id`);
    if (!/^[a-z0-9][a-z0-9.-]{0,95}$/.test(id) || seen.has(id)) invalid("Capability identifiers must be valid and unique."); seen.add(id);
    const revision = positiveInteger(capability.revision, `capabilities[${index}].revision`); const state = stringValue(capability.state, `capabilities[${index}].state`);
    if (!["available", "requires_configuration", "degraded", "unavailable"].includes(state)) invalid(`Capability ${id} has an unsupported state.`);
		const requiredSemantics = requireStringSet(capability.requiredSemantics, `capabilities[${index}].requiredSemantics`, true);
		if (requiredSemantics.length > 16) invalid(`Capability ${id} requires too many semantics.`);
    for (const name of requiredSemantics) if (!semantics[name]) invalid(`Capability ${id} references absent semantic ${name}.`);
    return Object.freeze({ id, revision, state: state as CompatibilityCapability["state"], requiredSemantics: Object.freeze(requiredSemantics) });
  }));
}
function requireBuild(value: unknown): CompatibilityEnvelope["build"] {
  const build = requireRecord(value, "build"); const version = stringValue(build.version, "build.version"); const buildNumber = stringValue(build.buildNumber, "build.buildNumber"); const channel = stringValue(build.channel, "build.channel"); const commit = stringValue(build.commit, "build.commit"); const timestamp = build.timestamp;
	const defaults = foundationContract.build;
	const exactDevelopment = version === defaults.version && buildNumber === defaults.buildNumber && channel === defaults.channel && commit === defaults.commit && (timestamp === undefined || timestamp === null);
	const exactRFC3339 = typeof timestamp === "string" && timestamp.length <= 64 && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(timestamp) && !Number.isNaN(Date.parse(timestamp));
	const release = version.length <= 64 && /^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(version) && /^[1-9]\d{0,11}$/.test(buildNumber) && ["production", "stable", "beta", "staging"].includes(channel) && /^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(commit) && exactRFC3339;
	if (!exactDevelopment && !release) invalid("build must use the exact development sentinel or a complete immutable release identity.");
  return Object.freeze({ version, buildNumber, channel, commit, ...(timestamp ? { timestamp } : {}) }) as CompatibilityEnvelope["build"];
}
function requireRevisionMap(value: unknown): Record<string, number> {
	const revisions = requireRecord(value, "semanticRevisions"); if (Object.keys(revisions).length === 0 || Object.keys(revisions).length > 64) invalid("semanticRevisions must be bounded and nonempty."); const result: Record<string, number> = {};
	for (const [name, revision] of Object.entries(revisions)) { if (!name.trim() || name !== name.trim() || name.length > 64) invalid("Semantic names must be bounded and canonical."); result[name] = positiveInteger(revision, `semanticRevisions.${name}`); } return result;
}
function requireStringSet(value: unknown, field: string, nonempty: boolean): string[] {
  if (!Array.isArray(value) || (nonempty && value.length === 0)) invalid(`${field} must be ${nonempty ? "a nonempty" : "an"} array.`); const result: string[] = []; const seen = new Set<string>();
	for (const entry of value) { if (typeof entry !== "string" || !entry || entry.length > 64 || entry !== entry.trim() || seen.has(entry)) invalid(`${field} must contain unique bounded canonical strings.`); seen.add(entry); result.push(entry); } return result;
}
function requireRecord(value: unknown, field: string): Record<string, unknown> { if (!isRecord(value)) invalid(`${field} must be an object.`); return value; }
function stringValue(value: unknown, field: string): string { if (typeof value !== "string" || !value || value !== value.trim()) invalid(`${field} must be a nonempty canonical string.`); return value; }
function positiveInteger(value: unknown, field: string): number { if (!Number.isInteger(value) || (value as number) < 1) invalid(`${field} must be a positive integer.`); return value as number; }
function invalid(message: string): never { throw new PorticoCompatibilityError("invalid_compatibility_envelope", message); }
function isRecord(value: unknown): value is Record<string, unknown> { return typeof value === "object" && value !== null && !Array.isArray(value); }
function sameStrings(left: readonly string[], right: readonly string[]): boolean { return [...left].sort().join("\0") === [...right].sort().join("\0"); }
function canonical(value: unknown): string { return JSON.stringify(value, (_key, nested) => isRecord(nested) ? Object.fromEntries(Object.entries(nested).sort(([a], [b]) => a.localeCompare(b))) : nested); }
