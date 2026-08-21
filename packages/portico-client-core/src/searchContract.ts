import type { ProductContract, SearchGroup, SearchRequest } from "./types.js";

export type SearchContract = ProductContract["search"];
export type SearchGroupCapability = SearchContract["groups"][number];
export type SearchSortCapability = SearchContract["sorts"][number];

export interface SearchRequestOptions {
  mode?: "quick" | "full";
  group?: SearchRequest["group"];
  entityKinds?: SearchRequest["entityKinds"];
  libraryIds?: SearchRequest["libraryIds"];
  sort?: SearchRequest["sort"];
  direction?: SearchRequest["direction"];
  cursor?: string;
  limit?: number;
  recordHistory?: boolean;
}

export interface ResolvedSearchResultSemantic {
  resultKind: string;
  entityKind: string;
  destination: ProductContract["entitySemantics"][number]["defaultDestination"];
  parentKinds: string[];
  childKinds: string[];
  childOrder: string[];
  artworkRole: string;
}

export function searchGroup(contract: ProductContract, id: string): SearchGroupCapability | undefined {
  return contract.search.groups.find((candidate) => candidate.id === id);
}

export function searchSort(contract: ProductContract, id: string): SearchSortCapability | undefined {
  return contract.search.sorts.find((candidate) => candidate.id === id);
}

/**
 * Produces a server-valid request from the published search vocabulary. This
 * owns semantic defaults only; clients remain responsible for their own UI.
 */
export function resolveSearchRequest(
  contract: ProductContract,
  query: string,
  options: SearchRequestOptions = {}
): SearchRequest {
  const normalizedQuery = query.trim();
  const limits = contract.search.limits;
  if (normalizedQuery.length < limits.minimumQueryLength || normalizedQuery.length > limits.maximumQueryLength) {
    throw new Error(`Search queries must be ${limits.minimumQueryLength}-${limits.maximumQueryLength} characters.`);
  }

  const group = options.group ? searchGroup(contract, options.group) : undefined;
  if (options.group && !group) throw new Error(`Search group ${options.group} is not published by Product Contract ${contract.search.revision}.`);
  if (options.cursor && !group) throw new Error("Search continuation requires one result group.");

  const sort = searchSort(contract, options.sort ?? "relevance");
  if (!sort) throw new Error(`Search sort ${String(options.sort)} is not published by Product Contract ${contract.search.revision}.`);
  if (group && !sort.applicableGroups.some((id) => id === group.id)) throw new Error(`Search sort ${sort.id} is not available for ${group.title}.`);
  const direction = options.direction ?? sort.defaultDirection;
  if (!sort.directions.includes(direction)) throw new Error(`Search sort ${sort.id} does not support ${direction}.`);

  const allowedEntityKinds = new Set(contract.search.filters.find((filter) => filter.id === "entityKinds")?.allowedValues ?? []);
  const requestedKinds = options.entityKinds?.filter((value, index, values) => value && values.indexOf(value) === index);
  const unknownKind = requestedKinds?.find((value) => !allowedEntityKinds.has(value));
  if (unknownKind) throw new Error(`Search result type ${unknownKind} is not published by Product Contract ${contract.search.revision}.`);
  const entityKinds = requestedKinds?.length
    ? requestedKinds
    : sort.id === "relevance"
      ? undefined
      : [...sort.applicableGroups];

  const typedEntityKinds = entityKinds as SearchRequest["entityKinds"];

  const defaultLimit = options.mode === "quick" ? limits.quickInitialGroupLimit : limits.fullDefaultGroupLimit;
  const limit = options.limit ?? defaultLimit;
  if (!Number.isInteger(limit) || limit < 1 || limit > limits.maximumGroupLimit) {
    throw new Error(`Search limit must be between 1 and ${limits.maximumGroupLimit}.`);
  }
  const recordHistory = options.recordHistory ?? (options.mode === "full" && !options.cursor);

  return {
    query: normalizedQuery,
    sort: sort.id,
    direction,
    limit,
    ...(recordHistory ? { recordHistory: true } : {}),
    ...(group ? { group: group.id } : {}),
    ...(typedEntityKinds?.length ? { entityKinds: typedEntityKinds } : {}),
    ...(options.libraryIds?.length ? { libraryIds: [...new Set(options.libraryIds)] } : {}),
    ...(options.cursor ? { cursor: options.cursor } : {})
  };
}

export function orderSearchGroups(contract: ProductContract, groups: readonly SearchGroup[], maximum?: number): SearchGroup[] {
  const order = new Map<string, number>(contract.search.groupOrder.map((id, index) => [id, index]));
  const sorted = [...groups].sort((left, right) =>
    (order.get(left.id) ?? Number.MAX_SAFE_INTEGER) - (order.get(right.id) ?? Number.MAX_SAFE_INTEGER)
  );
  return maximum === undefined ? sorted : sorted.slice(0, Math.max(0, maximum));
}

export function resolveSearchResultSemantic(
  contract: ProductContract,
  resultKind: string
): ResolvedSearchResultSemantic | undefined {
  const mappedKind = contract.search.resultSemantics.kindMappings.find((mapping) => mapping.resultKind === resultKind)?.entityKind ?? resultKind;
  const semantic = contract.entitySemantics.find((candidate) => candidate.id === mappedKind);
  if (!semantic) return undefined;
  return {
    resultKind,
    entityKind: semantic.id,
    destination: semantic.defaultDestination,
    parentKinds: [...semantic.parentKinds],
    childKinds: [...semantic.childKinds],
    childOrder: [...semantic.childOrder],
    artworkRole: semantic.primaryArtworkRole
  };
}
