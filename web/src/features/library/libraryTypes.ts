import type {
  BrowseExpression,
  BrowseLibraryRequest,
  BrowseSort,
  BrowseFacetOption,
  BrowseFacetSource,
  LibraryFieldCapability,
  LibraryBrowseCapabilities,
  LibraryPivotCapability,
  LibraryPresentation,
  LibrarySortCapability,
  SavedView,
  SavedViewCreateRequest,
} from '@porticomediaserver/client-core';
import type { MediaItem } from '../../data/models';

export type LibraryWorkspaceKind = LibraryBrowseCapabilities['library']['kind'];
export type {
  BrowseExpression,
  BrowseSort,
  LibraryFieldCapability,
  LibraryPivotCapability,
  LibraryPresentation,
  LibrarySortCapability,
};

export type LibraryWorkspaceLibrary = {
  id: string;
  name: string;
  kind: LibraryWorkspaceKind;
  itemCount: number;
};

export type LibraryPivotSection = {
  id: string;
  title: string;
  detail?: string;
  items: MediaItem[];
};

export type LibraryFacet = {
  id: string;
  title: string;
  detail?: string;
  count: number;
  artwork?: string;
  pivotId?: string;
  query: BrowseExpression;
};

export type LibraryPivotPage = {
  items: MediaItem[];
  sections?: LibraryPivotSection[];
  facets?: LibraryFacet[];
  total?: number;
  nextCursor?: string | null;
  hasMore: boolean;
  applied: {
    pivot: string;
    sort: BrowseSort[];
    presentationFields: string[];
  };
  presentation: LibraryPresentation;
};

export type BrowseLibrarySeekRequest = BrowseLibraryRequest & { seek?: { prefix: string } };

export type LibraryPivotInput = {
  libraryId: string;
  libraryKind: LibraryWorkspaceKind;
  request: BrowseLibrarySeekRequest;
  pivot: LibraryPivotCapability;
};

export interface LibraryWorkspaceSource {
  libraryBrowseCapabilities(libraryId: string, signal: AbortSignal): Promise<LibraryBrowseCapabilities>;
  libraryPivot(input: LibraryPivotInput, signal: AbortSignal): Promise<LibraryPivotPage>;
  libraryFacetOptions(libraryId: string, facetSource: NonNullable<BrowseFacetSource>, signal: AbortSignal): Promise<BrowseFacetOption[]>;
  createSavedView(input: SavedViewCreateRequest, signal: AbortSignal): Promise<SavedView>;
}
