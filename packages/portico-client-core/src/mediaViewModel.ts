import type { MediaCard, MediaItem, ProductContract } from "./types.js";

export interface MediaViewSource {
  readonly id: string;
  readonly title: string;
  readonly entityKind: string;
  readonly libraryId?: string;
  readonly subtitle?: string;
  readonly parentTitle?: string;
  readonly summary?: string;
  readonly year?: number;
  readonly durationSeconds?: number;
  readonly artwork?: Readonly<Record<string, string>>;
  readonly actions?: readonly string[];
  readonly fields?: Readonly<Record<string, unknown>>;
  readonly userState?: MediaViewStateSource;
  readonly availability?: MediaViewAvailabilitySource;
}

export interface MediaViewStateSource {
  readonly watched?: boolean;
  readonly watchlisted?: boolean;
  readonly favorite?: boolean;
  readonly progressSeconds?: number;
  readonly rating?: number;
  readonly reaction?: string;
  readonly lastPlayedAt?: string;
}

export type MediaViewAvailabilityStatus = "available" | "partial" | "unavailable";

export interface MediaViewAvailabilitySource {
  readonly status: MediaViewAvailabilityStatus;
  readonly fileCount?: number;
  readonly missingFileCount?: number;
}

export interface MediaViewDestination {
  /** Semantic destination from Product Contract, not a platform route string. */
  readonly kind: string;
  readonly entityId: string;
}

export interface MediaViewArtwork {
  /** The semantic artwork role whose geometry the application should render. */
  readonly role: string;
  /** The concrete source role used when the preferred role has no asset. */
  readonly sourceRole?: string;
  readonly url?: string;
  readonly purpose: string;
  readonly accessibilityLabel: string;
  readonly shape: {
    readonly aspectRatio: number;
    readonly fit: "cover" | "contain";
  };
}

export interface MediaViewSemantics {
  /** False means the kind was preserved but rendered with conservative defaults. */
  readonly known: boolean;
  readonly container: boolean;
  readonly playable: boolean;
  readonly parentKinds: readonly string[];
  readonly childKinds: readonly string[];
  readonly childOrder: readonly string[];
}

export interface MediaViewState {
  readonly watched: boolean;
  readonly watchlisted: boolean;
  readonly favorite: boolean;
  readonly progressSeconds: number;
  readonly rating?: number;
  readonly reaction?: string;
  readonly lastPlayedAt?: string;
}

/**
 * Platform-neutral media render DTO for cards, rows, and detail headers.
 * Applications resolve the semantic destination into their own navigation
 * stack and render the published artwork geometry with native components.
 */
export interface MediaViewModel {
  readonly id: string;
  readonly libraryId?: string;
  /** Canonical contract kind when known; otherwise the unchanged wire kind. */
  readonly kind: string;
  readonly sourceKind: string;
  readonly title: string;
  readonly subtitle?: string;
  readonly summary?: string;
  readonly year?: number;
  readonly durationSeconds?: number;
  readonly accessibilityLabel: string;
  readonly destination: MediaViewDestination;
  readonly artwork: MediaViewArtwork;
  readonly semantics: MediaViewSemantics;
  readonly actionIds: readonly string[];
  readonly state: MediaViewState;
  readonly availability?: MediaViewAvailabilitySource;
  readonly fields: Readonly<Record<string, unknown>>;
}

export type MediaDetailViewSource = Pick<MediaItem,
  "id" | "libraryId" | "entityKind" | "title" | "parentTitle" | "summary" | "year" |
  "durationSeconds" | "artwork" | "images" | "actions" | "typedMetadata" | "state" |
  "missing" | "fileCount" | "missingFileCount"
>;

interface ResolvedArtworkRole {
  readonly id: string;
  readonly aspectRatio: number;
  readonly fit: "cover" | "contain";
  readonly purpose: string;
}

const safeArtworkRole: ResolvedArtworkRole = {
  id: "poster",
  aspectRatio: 2 / 3,
  fit: "cover",
  purpose: "primary"
};

/**
 * Resolves media presentation exclusively from the current Product Contract.
 * Unknown optional kinds remain navigable and visible through conservative
 * detail/poster defaults.
 */
export function resolveMediaViewModel(contract: ProductContract, source: MediaViewSource): MediaViewModel {
  const sourceKind = normalizedText(source.entityKind) ?? "unknown";
  const mappedKind = contract.search.resultSemantics.kindMappings.find((mapping) => mapping.resultKind === sourceKind)?.entityKind;
  const semantic = contract.entitySemantics.find((candidate) => candidate.id === sourceKind)
    ?? contract.entitySemantics.find((candidate) => candidate.id === mappedKind);
  const kind = semantic?.id ?? sourceKind;
  const fallbackRole = resolveFallbackArtworkRole(contract);
  const preferredRole = semantic
    ? contract.artworkRoles.find((candidate) => candidate.id === semantic.primaryArtworkRole)
    : undefined;
  const role = preferredRole && validArtworkRole(preferredRole) ? preferredRole : fallbackRole;
  const artworkSources = collectArtworkSources(source);
  const selectedArtwork = selectArtworkSource(artworkSources, role.id);
  const state = source.userState;
  const subtitle = normalizedText(source.subtitle) ?? normalizedText(source.parentTitle);
  const title = normalizedText(source.title) ?? source.id;
  const libraryId = normalizedText(source.libraryId);
  const summary = normalizedText(source.summary);
  const year = finiteNumber(source.year);
  const durationSeconds = finiteNumber(source.durationSeconds);
  const rating = finiteNumber(state?.rating);
  const reaction = normalizedText(state?.reaction);
  const lastPlayedAt = normalizedText(state?.lastPlayedAt);
  const availability = resolveMediaAvailability(source);

  return {
    id: source.id,
    ...(libraryId ? { libraryId } : {}),
    kind,
    sourceKind,
    title,
    ...(subtitle ? { subtitle } : {}),
    ...(summary ? { summary } : {}),
    ...(year !== undefined ? { year } : {}),
    ...(durationSeconds !== undefined ? { durationSeconds } : {}),
    accessibilityLabel: title,
    destination: {
      kind: semantic?.defaultDestination ?? "detail",
      entityId: source.id
    },
    artwork: {
      role: role.id,
      ...(selectedArtwork ? { sourceRole: selectedArtwork.role, url: selectedArtwork.url } : {}),
      purpose: role.purpose,
      accessibilityLabel: title,
      shape: { aspectRatio: role.aspectRatio, fit: role.fit }
    },
    semantics: {
      known: semantic !== undefined,
      container: semantic?.container ?? false,
      playable: semantic?.playable ?? false,
      parentKinds: semantic ? [...semantic.parentKinds] : [],
      childKinds: semantic ? [...semantic.childKinds] : [],
      childOrder: semantic ? [...semantic.childOrder] : []
    },
    actionIds: [...(source.actions ?? [])],
    state: {
      watched: state?.watched ?? false,
      watchlisted: state?.watchlisted ?? false,
      favorite: state?.favorite ?? false,
      progressSeconds: finiteNumber(state?.progressSeconds) ?? 0,
      ...(rating !== undefined ? { rating } : {}),
      ...(reaction ? { reaction } : {}),
      ...(lastPlayedAt ? { lastPlayedAt } : {})
    },
    ...(availability ? { availability } : {}),
    fields: { ...(source.fields ?? {}) }
  };
}

/** Compile-time and runtime bridge for canonical browse/search cards. */
export function resolveMediaCardViewModel(contract: ProductContract, card: MediaCard): MediaViewModel {
  return resolveMediaViewModel(contract, {
    id: card.id,
    libraryId: card.libraryId,
    entityKind: card.entityKind,
    title: card.title,
    subtitle: card.subtitle,
    summary: card.summary,
    year: card.year,
    durationSeconds: card.durationSeconds,
    artwork: card.artwork,
    actions: card.actions,
    fields: card.fields,
    userState: {
      watched: card.userState.watched,
      watchlisted: card.userState.watchlisted,
      favorite: card.userState.favorite,
      progressSeconds: card.userState.progressSeconds,
      lastPlayedAt: card.userState.lastPlayedAt
    },
    availability: {
      status: card.availability.status,
      fileCount: card.availability.fileCount,
      missingFileCount: card.availability.missingFileCount
    }
  });
}

/** Compile-time and runtime bridge for full media-detail resources. */
export function resolveMediaDetailViewModel(contract: ProductContract, item: MediaDetailViewSource): MediaViewModel {
  return resolveMediaViewModel(contract, {
    id: item.id,
    libraryId: item.libraryId,
    entityKind: item.entityKind,
    title: item.title,
    parentTitle: item.parentTitle,
    summary: item.summary,
    year: item.year,
    durationSeconds: item.durationSeconds,
    artwork: { ...item.images, ...item.artwork },
    actions: item.actions,
    fields: item.typedMetadata,
    userState: {
      watched: item.state.watched,
      watchlisted: item.state.watchlisted,
      favorite: item.state.favorite,
      progressSeconds: item.state.progressSeconds,
      rating: item.state.rating,
      reaction: item.state.reaction,
      lastPlayedAt: item.state.lastPlayedAt
    },
    availability: {
      status: item.missing ? "unavailable" : (item.missingFileCount ?? 0) > 0 ? "partial" : "available",
      fileCount: item.fileCount,
      missingFileCount: item.missingFileCount
    }
  });
}

/** Resolves the one canonical availability shape used by every client surface. */
export function resolveMediaAvailability(source: Pick<MediaViewSource, "availability">): MediaViewAvailabilitySource | undefined {
  const fileCount = finiteNumber(source.availability?.fileCount);
  const missingFileCount = finiteNumber(source.availability?.missingFileCount);
  const status = source.availability?.status;
  if (!status) return undefined;
  return {
    status,
    ...(fileCount !== undefined ? { fileCount } : {}),
    ...(missingFileCount !== undefined ? { missingFileCount } : {})
  };
}

function resolveFallbackArtworkRole(contract: ProductContract): ResolvedArtworkRole {
  const candidates = [
    contract.artworkRoles.find((candidate) => candidate.id === "poster"),
    contract.artworkRoles.find((candidate) => candidate.id === "thumb"),
    ...contract.artworkRoles
  ];
  return candidates.find(validArtworkRole) ?? safeArtworkRole;
}

function validArtworkRole(value: ProductContract["artworkRoles"][number] | undefined): value is ProductContract["artworkRoles"][number] {
  return value !== undefined
    && typeof value.id === "string"
    && value.id.trim().length > 0
    && Number.isFinite(value.aspectRatio)
    && value.aspectRatio > 0
    && (value.fit === "cover" || value.fit === "contain")
    && typeof value.purpose === "string";
}

function collectArtworkSources(source: MediaViewSource): Readonly<Record<string, string>> {
  const sources: Record<string, string> = {};
  for (const [role, url] of Object.entries(source.artwork ?? {})) {
    const normalizedURL = normalizedText(url);
    if (normalizedURL) sources[role] = normalizedURL;
  }
  return sources;
}

function selectArtworkSource(sources: Readonly<Record<string, string>>, preferredRole: string): { role: string; url: string } | undefined {
  const fallbackRoles = preferredRole === "backdrop" || preferredRole === "banner"
    ? [preferredRole, "backdrop", "thumb", "poster"]
    : preferredRole === "square" || preferredRole === "logo" || preferredRole === "thumb"
      ? [preferredRole, "thumb", "poster", "backdrop"]
      : [preferredRole, "poster", "thumb", "backdrop"];
  const orderedRoles = [...new Set([...fallbackRoles, ...Object.keys(sources).sort()])];
  for (const role of orderedRoles) {
    const url = normalizedText(sources[role]);
    if (url) return { role, url };
  }
  return undefined;
}

function normalizedText(value: string | undefined): string | undefined {
  const normalized = value?.trim();
  return normalized ? normalized : undefined;
}

function finiteNumber(value: number | undefined): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}
