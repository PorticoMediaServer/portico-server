import type { MediaItem } from "./types.js";

export type PlayHandler = (item: MediaItem, options?: { queue?: MediaItem[] }) => void;

export function isPlayableMedia(item: MediaItem): boolean {
  return !["show", "anime", "season", "album", "artist"].includes(item.type);
}

export function selectionCanShareMetadataEditor(items: MediaItem[]): boolean {
  if (!items.length) return false;
  return new Set(items.map(metadataEditorScopeKey)).size === 1;
}

export function metadataEditorDisabledReason(items: MediaItem[], canEdit: boolean): string {
  if (!canEdit) return "Metadata editing is not available for this account.";
  if (!items.length) return "Select items before editing metadata.";
  if (!selectionCanShareMetadataEditor(items)) return "Select items from one library and media type to edit metadata together.";
  return "Edit metadata";
}

function metadataEditorScopeKey(item: MediaItem): string {
  return `${item.libraryId || "unknown-library"}:${item.type || "unknown-type"}`;
}
