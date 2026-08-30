import type { Library, LibraryCategory, LibrarySourceGroup } from "./types.js";

export type SortDirection = "asc" | "desc";

export interface LibraryDisplayPreferences {
  tab: string;
  filter: string;
  sort: string;
  order: SortDirection;
}

export interface LibraryCategoryGroup {
  group: string;
  label: string;
  items: LibraryCategory[];
}

export function normalizeLibraryDisplayPreferences(value: unknown, libraryType?: string): LibraryDisplayPreferences {
  const record = objectRecord(value);
  const tabs = libraryTabs(libraryType, true);
  const requestedTab = asString(record.tab, defaultLibraryTab(libraryType));
  const tab = tabs.includes(requestedTab) ? requestedTab : defaultLibraryTab(libraryType);
  const sort = asString(record.sort, "recent");
  const filter = asString(record.filter, "all");
  const rawOrder = asString(record.order, sort === "title" ? "asc" : "desc");
  const order: SortDirection = rawOrder === "asc" ? "asc" : "desc";
  return { tab, filter: filter || "all", sort: sort || "recent", order };
}

export function serializeLibraryDisplayPreferences(value: LibraryDisplayPreferences): Record<string, string> {
  return {
    tab: value.tab,
    filter: value.filter,
    sort: value.sort,
    order: value.order
  };
}

export function libraryTabs(type?: string, includeSources = false): string[] {
  const normalized = (type ?? "").toLowerCase();
  const tabs = normalized === "music"
    ? ["Discover", "Artists", "Albums", "Tracks", "Categories", "Collections"]
    : ["Discover", "Library", "Categories", "Collections"];
  return includeSources ? [...tabs, "Sources"] : tabs;
}

export function defaultLibraryTab(type?: string): string {
  return "Discover";
}

export function typedFilterForTab(tab: string, filter: string, libraryType?: string): string {
  const type = primaryLibraryItemType(libraryType, tab);
  if (!type) return filter || "all";
  return filter === "all" || !filter ? `type:${type}` : `type:${type};${filter}`;
}

function primaryLibraryItemType(libraryType?: string, tab = "Library"): string {
  const normalized = (libraryType ?? "").toLowerCase();
  if (normalized === "music") {
    if (tab === "Discover" || tab === "Albums") return "album";
    if (tab === "Artists") return "artist";
    if (tab === "Tracks") return "track";
    return "";
  }
  if (tab !== "Discover" && tab !== "Library") return "";
  if (normalized === "movie") return "movie";
  if (normalized === "show" || normalized === "anime") return normalized === "anime" ? "anime" : "show";
  if (normalized === "audiobook") return "audiobook";
  return "";
}

export function groupLibraryCategories(categories: LibraryCategory[], limitPerGroup = 12): LibraryCategoryGroup[] {
  const byGroup = new Map<string, LibraryCategory[]>();
  for (const category of categories) {
    const group = category.group || "category";
    const current = byGroup.get(group) ?? [];
    current.push(category);
    byGroup.set(group, current);
  }
  return Array.from(byGroup.entries()).map(([group, items]) => ({
    group,
    label: libraryCategoryGroupLabel(group),
    items: items.sort((left, right) => right.count - left.count || left.name.localeCompare(right.name)).slice(0, limitPerGroup)
  }));
}

export function libraryCategoryGroupLabel(group: string): string {
  return {
    genre: "Genres",
    style: "Styles",
    year: "Years",
    decade: "Decades",
    contentRating: "Ratings",
    studio: "Studios",
    artist: "Artists",
    albumArtist: "Album Artists",
    label: "Labels",
    author: "Authors",
    narrator: "Narrators",
    series: "Series",
    show: "Shows",
    season: "Seasons",
    network: "Networks",
    country: "Countries",
    tag: "Tags",
    category: "Categories"
  }[group] ?? labelFromOption(group);
}

export function libraryFilterLabel(filter: string, categories: LibraryCategory[]): string {
  if (!filter || filter === "all") return "All";
  const filters = splitLibraryFilters(filter);
  if (filters.length > 1) return filters.map((item) => libraryFilterLabel(item, categories)).join(" + ");
  if (filter.startsWith("type:")) return labelFromOption(filter.slice(5));
  if (filter.startsWith("sourcePath:")) return `Source ${sourcePathDisplayLabel(filter.slice(11))}`;
  return categories.find((category) => category.filter === filter)?.name ?? labelFromOption(filter);
}

export function libraryCountLabel(tab: string, libraryType: string, count: number): string {
  const plural = count === 1 ? "" : "s";
  const normalized = (libraryType ?? "").toLowerCase();
  if (normalized === "movie") return `movie${plural}`;
  if (normalized === "show" || normalized === "anime") return `show${plural}`;
  if (normalized === "music") {
    if (tab === "Artists") return `artist${plural}`;
    if (tab === "Tracks") return `track${plural}`;
    return `album${plural}`;
  }
  if (normalized === "audiobook") return `book${plural}`;
  return `visible item${plural}`;
}

export function libraryBaseFilterGroups(): Array<{ label: string; items: string[] }> {
  return [
    { label: "Common", items: ["all", "unwatched", "favorites", "matched", "unmatched"] },
    { label: "Library Health", items: ["needsAttention", "missingArtwork", "unavailable", "duplicates"] },
    { label: "Versions & Sources", items: ["optimized", "source:local", "source:remote"] }
  ];
}

export function splitLibraryFilters(filter: string): string[] {
  return filter.split(";").map((item) => item.trim()).filter((item) => item && item !== "all");
}

export function labelFromOption(option: string): string {
  return {
    all: "All",
    unwatched: "Unwatched",
    favorites: "Favorites",
    matched: "Matched",
    unmatched: "Unmatched",
    needsAttention: "Needs Attention",
    missingArtwork: "Missing Artwork",
    unavailable: "Unavailable",
    duplicates: "Duplicates",
    optimized: "Optimized Versions",
    "source:local": "Local Sources",
    "source:remote": "Remote Sources",
    "1080p-high": "1080p High",
    "1080p-medium": "1080p Medium",
    "720p-high": "720p High",
    "720p-medium": "720p Medium",
    "480p": "480p",
    "328p": "328p",
    recent: "Recently Added",
    title: "Title",
    year: "Release Date",
    runtime: "Runtime",
    progress: "Progress"
  }[option] ?? option;
}

export function sourceKindLabel(source: LibrarySourceGroup): string {
  const type = source.sourceType && source.sourceType !== source.kind ? `${source.sourceType} ` : "";
  return `${type}${source.kind || "source"}`.replace(/^\w/, (match) => match.toUpperCase());
}

export function sourcePathDisplayLabel(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return "Unknown";
  try {
    const parsed = new URL(trimmed);
    if (parsed.host) return parsed.host;
    return parsed.protocol.replace(":", "") || trimmed;
  } catch {
    const parts = trimmed.split(/[\\/]/).filter(Boolean);
    return parts.length ? parts[parts.length - 1] : trimmed;
  }
}

export function defaultLibraryQuery(library?: Library): LibraryDisplayPreferences {
  return normalizeLibraryDisplayPreferences(undefined, library?.type);
}

function objectRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function asString(value: unknown, fallback: string): string {
  return typeof value === "string" && value.trim() ? value : fallback;
}
