import type { BrowseLibraryRequest, ProductContract } from "./types.js";
import {
  productLanguageCatalog,
  productMessage,
  semanticIcon,
  type ProductMessageId,
  type SemanticIconId
} from "./productLanguage.js";

export type MediaActionCapability = ProductContract["mediaActions"][number];
export type BrowseFacetSource = ProductContract["browseFields"][number]["facetSource"];

export interface BrowseFacetOption {
  value: string;
  label: string;
  count?: number;
}

export interface ResolvedMediaActionCommand {
  kind: "api" | "client-flow";
  execution: MediaActionCapability["command"]["execution"];
  method?: "GET" | "POST" | "PATCH" | "DELETE";
  path?: string;
  body?: Record<string, unknown>;
  flowId?: string;
  resultHandling: MediaActionCapability["command"]["resultHandling"];
  confirmation: MediaActionCapability["confirmation"];
  invalidates: string[];
}

export type MediaActionSurface = MediaActionCapability["presentation"]["surfaces"][number];

export interface PresentedMediaAction {
  id: MediaActionCapability["id"];
  label: string;
  labelMessageId: ProductMessageId;
  iconId: SemanticIconId;
  icon: ReturnType<typeof semanticIcon>;
  group: MediaActionCapability["presentation"]["group"];
  priority: number;
  capability: MediaActionCapability;
}

/**
 * Resolves only actions that both the item projection and the canonical
 * Product Contract allow on a client surface. Labels and icons always come
 * from the shared product-language catalog, never from platform-local copy.
 */
export function mediaActionsForSurface(
  contract: ProductContract,
  availableActionIds: ReadonlyArray<MediaActionCapability["id"]>,
  surface: MediaActionSurface
): PresentedMediaAction[] {
  const available = new Set(availableActionIds);
  return contract.mediaActions
    .filter((action) => available.has(action.id) && action.presentation.surfaces.includes(surface))
    .map((capability) => {
      const labelMessageId = capability.presentation.labelMessageId;
      const iconId = capability.presentation.iconId;
      if (!Object.hasOwn(productLanguageCatalog.messages, labelMessageId)) {
        throw new Error(`Media action ${capability.id} references unknown product message ${labelMessageId}.`);
      }
      if (!Object.hasOwn(productLanguageCatalog.icons, iconId)) {
        throw new Error(`Media action ${capability.id} references unknown semantic icon ${iconId}.`);
      }
      const message = productMessage(labelMessageId as ProductMessageId);
      const label = message.text ?? message.title;
      if (!label) throw new Error(`Media action ${capability.id} label ${labelMessageId} has no display text.`);
      return {
        id: capability.id,
        label,
        labelMessageId: labelMessageId as ProductMessageId,
        iconId: iconId as SemanticIconId,
        icon: semanticIcon(iconId as SemanticIconId),
        group: capability.presentation.group,
        priority: capability.presentation.priority,
        capability
      };
    })
    .sort((left, right) => right.priority - left.priority);
}

export interface ContextualPlaybackMedia {
  entityKind?: string;
  kind?: string;
  type?: string;
  progress?: number;
  progressSeconds?: number;
  state?: { progressSeconds?: number };
  seasonNumber?: number;
  episodeNumber?: number;
  playbackTarget?: ContextualPlaybackMedia;
}

const contextualKindAliases: Readonly<Record<string, string>> = Object.freeze({
  audiobook: "book",
  "audiobook-series": "series",
  live_channel: "channel",
  "live-channel": "channel",
  live_recording: "recording",
  "live-recording": "recording"
});

function contextualMediaKind(media: ContextualPlaybackMedia): string {
  const raw = String(media.entityKind ?? media.kind ?? media.type ?? "")
    .trim()
    .toLocaleLowerCase()
    .replaceAll("_", "-");
  return contextualKindAliases[raw] ?? raw;
}

function hasPlaybackProgress(media: ContextualPlaybackMedia): boolean {
  return (media.progressSeconds ?? media.state?.progressSeconds ?? 0) > 0 || (media.progress ?? 0) > 0;
}

function contextualEpisodeCode(media: ContextualPlaybackMedia): string {
  if (media.seasonNumber != null && media.episodeNumber != null) return `S${media.seasonNumber}E${media.episodeNumber}`;
  if (media.episodeNumber != null) return `E${media.episodeNumber}`;
  return "episode";
}

function contextualPlayMessage(media: ContextualPlaybackMedia): { id: ProductMessageId; variables?: Record<string, string | number> } | undefined {
  const kind = contextualMediaKind(media);
  const resume = hasPlaybackProgress(media);
  if (kind === "channel") return { id: "action.watch-live" };
  if (kind === "show") {
    const target = media.playbackTarget;
    if (!target) return { id: "action.play-next-episode" };
    return {
      id: hasPlaybackProgress(target) ? "action.resume-episode" : "action.play-episode",
      variables: { episodeCode: contextualEpisodeCode(target) }
    };
  }
  if (kind === "season") return { id: "action.play-season" };
  if (kind === "artist") return { id: "action.play-artist" };
  if (kind === "album") return { id: "action.play-album" };
  if (kind === "track") return { id: "action.play-track" };
  if (kind === "author") return { id: "action.play-author" };
  if (kind === "book") return { id: resume ? "action.resume-audiobook" : "action.play-audiobook" };
  if (kind === "series") return { id: "action.play-series" };
  if (kind === "recording") return { id: resume ? "action.resume-recording" : "action.play-recording" };
  if (kind === "collection") return { id: "action.play-collection" };
  if (kind === "playlist") return { id: "action.play-playlist" };
  if (resume) return { id: "action.resume" };
  return undefined;
}

/**
 * Adds media context to an already server-projected play capability. This
 * helper never creates an action: an absent or non-play action is returned
 * unchanged so server eligibility and platform gating remain authoritative.
 */
export function contextualMediaPlayAction(
  action: PresentedMediaAction | undefined,
  media: ContextualPlaybackMedia
): PresentedMediaAction | undefined {
  if (!action || !["play", "live.play", "dvr.play"].includes(action.id)) return action;
  const contextual = contextualPlayMessage(media);
  if (!contextual) return action;
  const message = productMessage(contextual.id, contextual.variables);
  const label = message.text ?? message.title;
  if (!label) throw new Error(`Contextual media action label ${contextual.id} has no display text.`);
  return { ...action, label, labelMessageId: contextual.id };
}

/**
 * Resolves the versioned server-published action vocabulary without dispatching
 * it. The application remains responsible for confirmation UI and for running
 * client-flow commands such as pickers and metadata editors.
 */
export function resolveMediaActionCommand(
  contract: ProductContract,
  actionId: MediaActionCapability["id"],
  inputs: Record<string, unknown> = {}
): ResolvedMediaActionCommand {
  const action = contract.mediaActions.find((candidate) => candidate.id === actionId);
  if (!action) throw new Error(`Media action ${actionId} is not defined by Portico API ${contract.apiVersion}.`);

  const required = action.command.requiredInputs ?? [];
  if (action.command.kind === "client-flow") {
    const flowId = action.command.flowId;
    if (!flowId) throw new Error(`Client flow ${actionId} has no flow identifier.`);
    return {
      kind: "client-flow",
      execution: action.command.execution,
      flowId,
      resultHandling: action.command.resultHandling,
      confirmation: action.confirmation,
      invalidates: [...action.invalidates]
    };
  }

  const missing = required.filter((key) => inputs[key] === undefined);
  if (missing.length) throw new Error(`Media action ${actionId} requires: ${missing.join(", ")}.`);
  if (!action.command.method || !action.command.pathTemplate) {
    throw new Error(`Media action ${actionId} does not publish a complete API command.`);
  }
  const path = action.command.pathTemplate.replace(/\{([^}]+)\}/g, (_match, key: string) => {
    const value = inputs[key];
    if (value === undefined || value === null || value === "") throw new Error(`Media action ${actionId} requires path input ${key}.`);
    return encodeURIComponent(String(value));
  });
  const pathInputs = new Set(Array.from(action.command.pathTemplate.matchAll(/\{([^}]+)\}/g), (match) => match[1]));
  const body: Record<string, unknown> = { ...(action.command.staticBody ?? {}) };
  for (const key of required) {
    if (!pathInputs.has(key)) body[key] = inputs[key];
  }
  return {
    kind: "api",
    execution: action.command.execution,
    method: action.command.method,
    path,
    ...(Object.keys(body).length ? { body } : {}),
    resultHandling: action.command.resultHandling,
    confirmation: action.confirmation,
    invalidates: [...action.invalidates]
  };
}

export function entitySemantic(contract: ProductContract, entityKind: string) {
  return contract.entitySemantics.find((semantic) => semantic.id === entityKind);
}

export function artworkRole(contract: ProductContract, role: string) {
  return contract.artworkRoles.find((artwork) => artwork.id === role);
}

export function resolveBrowseFacetEndpoint(source: BrowseFacetSource, libraryId: string): string {
  if (!source) throw new Error("Browse field does not publish a dynamic facet source.");
  return source.endpointTemplate.replace("{libraryId}", encodeURIComponent(libraryId));
}

export function browseFacetOptions(source: BrowseFacetSource, items: ReadonlyArray<Record<string, unknown>>): BrowseFacetOption[] {
  if (!source) return [];
  const options: BrowseFacetOption[] = [];
  for (const item of items) {
    const filter = item[source.filterField];
    if (typeof filter !== "string" || !filter.startsWith(source.filterPrefix)) continue;
    const value = item[source.valueField];
    const label = item[source.labelField];
    if (typeof value !== "string" || typeof label !== "string" || !value.trim() || !label.trim()) continue;
    const rawCount = item[source.countField];
    options.push({
      value,
      label,
      ...(typeof rawCount === "number" && Number.isFinite(rawCount) ? { count: rawCount } : {})
    });
  }
  return options;
}

/** Adds an alphabet anchor that remains part of every keyset continuation scope. */
export function withBrowseAlphaSeek(request: BrowseLibraryRequest, prefix: string): BrowseLibraryRequest {
  const normalized = prefix.trim().toUpperCase();
  if (!/^(?:[A-Z]|#)$/.test(normalized)) throw new Error("Browse seek must be A-Z or #.");
  const primary = request.sort?.[0];
  if (!primary || (primary.field !== "title" && primary.field !== "sortTitle") || primary.direction !== "asc") {
    throw new Error("Browse seek requires title or sortTitle ascending as the primary sort.");
  }
  return { ...request, seek: { prefix: normalized } };
}
