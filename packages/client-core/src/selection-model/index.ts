export * from './types.ts';
export {
  selectableSelectionKind, selectionActions, selectionCapabilities, selectionFourthAction,
  selectionKey, selectionKindHasBulkFields, selectionKindHasWatched, summarizeSelection,
} from './capabilities.ts';
export {SelectionModel} from './model.ts';
export {selectionQuerySelector, type SelectionQuerySelector} from './targets.ts';
export {defaultSelectionLimits, SelectionInputError} from './validation.ts';
export {createCataloguePersonalSelection, catalogueSelectionSegments, playCatalogueSelection, type CatalogueSelectionLifetime} from '../catalogue-selection.ts';
