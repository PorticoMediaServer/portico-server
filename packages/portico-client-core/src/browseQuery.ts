import type {
  BrowseExpression,
  BrowseLibraryRequest,
  LibraryBrowseCapabilities,
  SavedView,
  SavedViewCreateRequest
} from "./types.js";

export type BrowseSort = NonNullable<BrowseLibraryRequest["sort"]>[number];
export type LibraryPivotCapability = LibraryBrowseCapabilities["pivots"][number];
export type LibraryFieldCapability = LibraryBrowseCapabilities["fields"][number];
export type LibrarySortCapability = LibraryBrowseCapabilities["sorts"][number];
export type LibraryPresentation = "shelves" | "grid" | "compact-grid" | "list" | "table" | "facets";

export type FilterConditionNode = {
  id: string;
  kind: "condition";
  field: string;
  operator: string;
  rawValue: string;
  /** Lossless editor state for list operators; values may contain commas. */
  rawValues?: string[];
  negated: boolean;
};

export type FilterGroupNode = {
  id: string;
  kind: "group";
  mode: "all" | "any";
  negated: boolean;
  children: FilterNode[];
};

export type FilterNode = FilterConditionNode | FilterGroupNode;

export type BrowseQueryChip = {
  key: string;
  label: string;
  path: number[];
};

export type BrowseQueryValidationIssue = {
  path: string;
  code: "maximum_bytes" | "maximum_depth" | "maximum_clauses" | "unsupported_field" | "unsupported_operator" | "invalid_value";
  message: string;
};

export type BrowseWorkspaceQuery = {
  pivot: LibraryPivotCapability;
  expression?: BrowseExpression;
  expressionInvalid: boolean;
  sorts: BrowseSort[];
  presentation: LibraryPresentation;
};

export type SavedViewDraft = {
  title: string;
  libraryId: string;
  pivot: string;
  query?: BrowseExpression;
  sort?: BrowseSort[];
  presentationFields?: string[];
  isPinned?: boolean;
};

let filterNodeSequence = 0;

function nodeId(prefix: "condition" | "group") {
  filterNodeSequence += 1;
  return `${prefix}-${filterNodeSequence}`;
}

export function formatCapabilityLabel(value: string) {
  return value
    .replaceAll(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replaceAll("-", " ")
    .replaceAll("_", " ")
    .replace(/^./, letter => letter.toLocaleUpperCase());
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function hasOnlyKeys(value: Record<string, unknown>, keys: string[]) {
  const allowed = new Set(keys);
  return Object.keys(value).every(key => allowed.has(key));
}

function isScalar(value: unknown): value is string | number | boolean {
  return typeof value === "string"
    || (typeof value === "number" && Number.isFinite(value))
    || typeof value === "boolean";
}

function hasBrowseExpressionShape(value: unknown, depth: number): value is BrowseExpression {
  if (depth > 64) return false;
  if (!isRecord(value)) return false;
  if ("all" in value) {
    return hasOnlyKeys(value, ["all"])
      && Array.isArray(value.all)
      && value.all.length > 0
      && value.all.every(child => hasBrowseExpressionShape(child, depth + 1));
  }
  if ("any" in value) {
    return hasOnlyKeys(value, ["any"])
      && Array.isArray(value.any)
      && value.any.length > 0
      && value.any.every(child => hasBrowseExpressionShape(child, depth + 1));
  }
  if ("not" in value) {
    return hasOnlyKeys(value, ["not"]) && hasBrowseExpressionShape(value.not, depth + 1);
  }
  if (!hasOnlyKeys(value, ["field", "operator", "value"])) return false;
  if (typeof value.field !== "string" || !value.field || typeof value.operator !== "string" || !value.operator) return false;
  if (value.operator === "is-present" || value.operator === "is-missing") return value.value === null;
  return isScalar(value.value)
    || (Array.isArray(value.value) && value.value.length > 0 && value.value.every(isScalar));
}

export function isBrowseExpression(value: unknown): value is BrowseExpression {
  return hasBrowseExpressionShape(value, 0);
}

export function decodeExpression(value: string | null | undefined): BrowseExpression | undefined {
  if (!value) return undefined;
  try {
    const parsed: unknown = JSON.parse(value);
    return isBrowseExpression(parsed) ? parsed : undefined;
  } catch {
    return undefined;
  }
}

export function encodeExpression(value: BrowseExpression | undefined) {
  return value ? JSON.stringify(value) : undefined;
}

export function decodeSorts(value: string | null | undefined): BrowseSort[] {
  if (!value) return [];
  return value.split(",").flatMap(part => {
    const [field, direction, ...extra] = part.split(":");
    return field && extra.length === 0 && (direction === "asc" || direction === "desc")
      ? [{ field, direction }]
      : [];
  });
}

export function encodeSorts(sorts: BrowseSort[]) {
  return sorts.map(sort => `${sort.field}:${sort.direction}`).join(",");
}

export function availableFields(capabilities: LibraryBrowseCapabilities, pivot: LibraryPivotCapability) {
  return capabilities.fields.filter(field => {
    if (!field.applicableKinds?.length) return true;
    return field.applicableKinds.some(kind => pivot.entityKinds.includes(kind));
  });
}

export function availableSorts(capabilities: LibraryBrowseCapabilities, pivot: LibraryPivotCapability) {
  return capabilities.sorts.filter(sort => {
    if (!sort.applicableKinds?.length) return true;
    return sort.applicableKinds.some(kind => pivot.entityKinds.includes(kind));
  });
}

export function normalizeSorts(
  requested: BrowseSort[],
  capabilities: LibraryBrowseCapabilities,
  pivot: LibraryPivotCapability
) {
  const allowed = new Map(availableSorts(capabilities, pivot).map(sort => [sort.id, sort]));
  const seen = new Set<string>();
  const normalized = requested.filter(sort => {
    if (seen.has(sort.field) || !allowed.get(sort.field)?.directions.includes(sort.direction)) return false;
    seen.add(sort.field);
    return true;
  });
  return normalized.length ? normalized.slice(0, 3) : pivot.defaultSort.map(sort => ({ ...sort }));
}

export function normalizePresentation(requested: string | null | undefined, pivot: LibraryPivotCapability): LibraryPresentation {
  const canonical = (value: string | null | undefined): LibraryPresentation | undefined => {
    if (value === "compact") return "compact-grid";
    if (value === "timeline") return "list";
    return ["shelves", "grid", "compact-grid", "list", "table", "facets"].includes(value ?? "")
      ? value as LibraryPresentation
      : undefined;
  };
  const supported = [...new Set(pivot.supportedViews.map(canonical).filter((value): value is LibraryPresentation => Boolean(value)))];
  const requestedPresentation = canonical(requested);
  if (requestedPresentation && supported.includes(requestedPresentation)) return requestedPresentation;
  const defaultPresentation = canonical(pivot.defaultView);
  if (defaultPresentation && supported.includes(defaultPresentation)) return defaultPresentation;
  return supported[0] ?? "grid";
}

function utf8ByteLength(value: string) {
  let bytes = 0;
  for (const character of value) {
    const codePoint = character.codePointAt(0) ?? 0;
    bytes += codePoint <= 0x7f ? 1 : codePoint <= 0x7ff ? 2 : codePoint <= 0xffff ? 3 : 4;
  }
  return bytes;
}

export function validateBrowseExpression(
  expression: BrowseExpression,
  capabilities: LibraryBrowseCapabilities,
  pivot: LibraryPivotCapability
): BrowseQueryValidationIssue[] {
  const issues: BrowseQueryValidationIssue[] = [];
  const fields = new Map(availableFields(capabilities, pivot).map(field => [field.id, field]));
  let clauses = 0;
  if (utf8ByteLength(JSON.stringify(expression)) > capabilities.queryLimits.maximumBytes) {
    issues.push({ path: "query", code: "maximum_bytes", message: `Query must not exceed ${capabilities.queryLimits.maximumBytes} bytes.` });
  }
  const inspect = (node: BrowseExpression, path: string, depth: number) => {
    if (depth > capabilities.queryLimits.maximumDepth) {
      issues.push({ path, code: "maximum_depth", message: `Query must not exceed ${capabilities.queryLimits.maximumDepth} levels.` });
      return;
    }
    if ("not" in node) {
      inspect(node.not, `${path}.not`, depth + 1);
      return;
    }
    if ("all" in node) {
      node.all.forEach((child, index) => inspect(child, `${path}.all[${index}]`, depth + 1));
      return;
    }
    if ("any" in node) {
      node.any.forEach((child, index) => inspect(child, `${path}.any[${index}]`, depth + 1));
      return;
    }
    clauses += 1;
    if (clauses > capabilities.queryLimits.maximumClauses) {
      issues.push({ path, code: "maximum_clauses", message: `Query must not exceed ${capabilities.queryLimits.maximumClauses} conditions.` });
      return;
    }
    const field = fields.get(node.field);
    if (!field) {
      issues.push({ path: `${path}.field`, code: "unsupported_field", message: `Field ${node.field} is not available for ${pivot.label}.` });
      return;
    }
    if (!field.operators.includes(node.operator)) {
      issues.push({ path: `${path}.operator`, code: "unsupported_operator", message: `Operator ${node.operator} is not available for ${field.label}.` });
      return;
    }
    const value = node.value;
    if (node.operator === "is-present" || node.operator === "is-missing") {
      if (value !== null) issues.push({ path: `${path}.value`, code: "invalid_value", message: "Presence operators require a null value." });
      return;
    }
    const listRequired = ["in", "not-in", "contains-any", "contains-all", "between"].includes(node.operator);
    if (listRequired !== Array.isArray(value) || (Array.isArray(value) && (value.length === 0 || (node.operator === "between" && value.length !== 2)))) {
      issues.push({ path: `${path}.value`, code: "invalid_value", message: listRequired ? "This operator requires a non-empty value list." : "This operator requires one value." });
      return;
    }
    const values = Array.isArray(value) ? value : [value];
    if (field.valueType === "boolean" && values.some(candidate => typeof candidate !== "boolean")) {
      issues.push({ path: `${path}.value`, code: "invalid_value", message: `${field.label} requires true or false.` });
      return;
    }
    if (["number", "duration", "date-number"].includes(field.valueType) && values.some(candidate => typeof candidate !== "number" || !Number.isFinite(candidate))) {
      issues.push({ path: `${path}.value`, code: "invalid_value", message: `${field.label} requires a finite number.` });
      return;
    }
    if (["string", "enum", "date", "identity-set"].includes(field.valueType) && values.some(candidate => typeof candidate !== "string")) {
      issues.push({ path: `${path}.value`, code: "invalid_value", message: `${field.label} requires text values.` });
      return;
    }
    if (field.allowedValues?.length && values.some(candidate => typeof candidate !== "string" || !field.allowedValues?.includes(candidate))) {
      issues.push({ path: `${path}.value`, code: "invalid_value", message: `${field.label} contains an unsupported value.` });
    }
  };
  inspect(expression, "query", 0);
  return issues;
}

export function resolveBrowseWorkspaceQuery(
  parameters: { pivot?: string | null; filters?: string | null; sort?: string | null; view?: string | null },
  capabilities: LibraryBrowseCapabilities
): BrowseWorkspaceQuery | undefined {
  const pivot = capabilities.pivots.find(candidate => candidate.id === parameters.pivot) ?? capabilities.pivots[0];
  if (!pivot) return undefined;
  const expressionTooLarge = Boolean(parameters.filters && utf8ByteLength(parameters.filters) > capabilities.queryLimits.maximumBytes);
  const decodedExpression = expressionTooLarge ? undefined : decodeExpression(parameters.filters);
  const expressionIssues = decodedExpression ? validateBrowseExpression(decodedExpression, capabilities, pivot) : [];
  const expression = expressionIssues.length === 0 ? decodedExpression : undefined;
  return {
    pivot,
    expression,
    expressionInvalid: Boolean(parameters.filters && (!expression || expressionIssues.length > 0)),
    sorts: normalizeSorts(decodeSorts(parameters.sort), capabilities, pivot),
    presentation: normalizePresentation(parameters.view, pivot)
  };
}

function expressionValue(value: unknown) {
  if (Array.isArray(value)) return "";
  if (typeof value === "boolean") return value ? "true" : "false";
  return value == null ? "" : String(value);
}

function expressionToNode(expression: BrowseExpression): FilterNode {
  if ("not" in expression) {
    const child = expressionToNode(expression.not);
    return { ...child, negated: !child.negated };
  }
  if ("all" in expression) {
    return {
      id: nodeId("group"),
      kind: "group",
      mode: "all",
      negated: false,
      children: expression.all.map(expressionToNode)
    };
  }
  if ("any" in expression) {
    return {
      id: nodeId("group"),
      kind: "group",
      mode: "any",
      negated: false,
      children: expression.any.map(expressionToNode)
    };
  }
  return {
    id: nodeId("condition"),
    kind: "condition",
    field: expression.field,
    operator: expression.operator,
    rawValue: expressionValue(expression.value),
    ...(Array.isArray(expression.value) ? { rawValues: expression.value.map(String) } : {}),
    negated: false
  };
}

export function expressionToFilter(expression: BrowseExpression | undefined, fields: LibraryFieldCapability[]): FilterGroupNode {
  if (expression) {
    const node = expressionToNode(expression);
    if (node.kind === "group") return node;
    return { id: nodeId("group"), kind: "group", mode: "all", negated: false, children: [node] };
  }
  const firstField = fields[0];
  return {
    id: nodeId("group"),
    kind: "group",
    mode: "all",
    negated: false,
    children: firstField
      ? [{
          id: nodeId("condition"),
          kind: "condition",
          field: firstField.id,
          operator: firstField.operators[0] ?? "equals",
          rawValue: "",
          negated: false
        }]
      : []
  };
}

function parseConditionValue(node: FilterConditionNode, field: LibraryFieldCapability) {
  if (node.operator === "is-present" || node.operator === "is-missing") return null;
  if (field.valueType === "boolean") return node.rawValue === "true";
  const listOperator = ["in", "not-in", "contains-any", "contains-all", "between"].includes(node.operator);
  if (listOperator) {
    const values = node.rawValues ?? node.rawValue.split(",").map(value => value.trim()).filter(Boolean);
    return values.map(value => {
      return ["number", "duration", "date-number"].includes(field.valueType) && Number.isFinite(Number(value))
        ? Number(value)
        : value;
    });
  }
  if (["number", "duration", "date-number"].includes(field.valueType) && Number.isFinite(Number(node.rawValue))) {
    return Number(node.rawValue);
  }
  return node.rawValue.trim();
}

function compileNode(node: FilterNode, fields: Map<string, LibraryFieldCapability>): BrowseExpression | undefined {
  if (node.kind === "group") {
    const children = node.children.flatMap(child => {
      const compiled = compileNode(child, fields);
      return compiled ? [compiled] : [];
    });
    if (!children.length) return undefined;
    const group: BrowseExpression = node.mode === "all" ? { all: children } : { any: children };
    return node.negated ? { not: group } : group;
  }
  const field = fields.get(node.field);
  if (!field || !field.operators.includes(node.operator)) return undefined;
  const hasValue = node.rawValues ? node.rawValues.length > 0 : Boolean(node.rawValue.trim());
  if (!["is-present", "is-missing"].includes(node.operator) && !hasValue) return undefined;
  const predicate: BrowseExpression = {
    field: node.field,
    operator: node.operator,
    value: parseConditionValue(node, field),
  };
  return node.negated ? { not: predicate } : predicate;
}

export function compileFilter(root: FilterGroupNode, fields: LibraryFieldCapability[]) {
  return compileNode(root, new Map(fields.map(field => [field.id, field])));
}

export function countConditions(node: FilterNode): number {
  return node.kind === "condition"
    ? 1
    : node.children.reduce((total, child) => total + countConditions(child), 0);
}

export function queryChips(expression: BrowseExpression | undefined, fields: LibraryFieldCapability[]) {
  if (!expression) return [];
  const fieldMap = new Map(fields.map(field => [field.id, field]));
  const chips: BrowseQueryChip[] = [];
  const visit = (node: BrowseExpression, negated = false, path: number[] = []) => {
    if ("not" in node) {
      visit(node.not, !negated, path);
      return;
    }
    if ("all" in node) {
      node.all.forEach((child, index) => visit(child, negated, [...path, index]));
      return;
    }
    if ("any" in node) {
      node.any.forEach((child, index) => visit(child, negated, [...path, index]));
      return;
    }
    const field = fieldMap.get(node.field);
    const value = expressionValue(node.value);
    chips.push({
      key: `${chips.length}-${node.field}-${node.operator}`,
      label: `${negated ? "Not " : ""}${field?.label ?? formatCapabilityLabel(node.field)} ${formatCapabilityLabel(node.operator).toLocaleLowerCase()}${value ? ` ${value}` : ""}`,
      path
    });
  };
  visit(expression);
  return chips;
}

export function removeExpressionAtPath(expression: BrowseExpression, path: number[]): BrowseExpression | undefined {
  if (!path.length) return undefined;
  if ("not" in expression) {
    const child = removeExpressionAtPath(expression.not, path);
    return child ? { not: child } : undefined;
  }
  if (!("all" in expression) && !("any" in expression)) return expression;
  const [target, ...rest] = path;
  const source = "all" in expression ? expression.all : expression.any;
  const children = source.flatMap((child, index) => {
    if (index !== target) return [child];
    if (!rest.length) return [];
    const next = removeExpressionAtPath(child, rest);
    return next ? [next] : [];
  });
  if (!children.length) return undefined;
  return "all" in expression ? { all: children } : { any: children };
}

export function sortLabel(sort: BrowseSort, capabilities: LibrarySortCapability[]) {
  const capability = capabilities.find(candidate => candidate.id === sort.field);
  return capability?.label ?? formatCapabilityLabel(sort.field);
}

export function serializeSavedViewDraft(draft: SavedViewDraft): SavedViewCreateRequest {
  const title = draft.title.trim();
  const libraryId = draft.libraryId.trim();
  const pivot = draft.pivot.trim();
  if (!title) throw new TypeError("A saved view title is required.");
  if (title.length > 160) throw new RangeError("A saved view title cannot exceed 160 characters.");
  if (!libraryId) throw new TypeError("A saved view library is required.");
  if (!pivot) throw new TypeError("A saved view pivot is required.");
  if (draft.sort && draft.sort.length > 3) throw new RangeError("A saved view supports at most three sort fields.");
  if (draft.query && !isBrowseExpression(draft.query)) throw new TypeError("A saved view query must use the browse expression contract.");
  return {
    title,
    libraryId,
    pivot,
    ...(draft.query ? { query: draft.query } : {}),
    ...(draft.sort?.length ? { sort: [...draft.sort] } : {}),
    ...(draft.presentationFields ? { presentation: { fields: [...new Set(draft.presentationFields)] } } : {}),
    isPinned: Boolean(draft.isPinned)
  };
}

export function savedViewWorkspaceQuery(view: SavedView): Pick<BrowseLibraryRequest, "pivot" | "query" | "sort" | "presentation"> {
  return {
    pivot: view.pivot,
    query: view.query,
    sort: [...view.sort],
    presentation: { fields: [...(view.presentation.fields ?? [])] }
  };
}
