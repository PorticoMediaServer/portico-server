export const selectionMessages = {
  'next.selection.keepHolding': 'Keep holding to select',
  'next.selection.select': 'Select',
  'next.selection.selected': '{count, number} selected',
  'next.selection.actions': 'Selection actions',
  'next.selection.selection': 'Selection',
} as const satisfies Record<`next.selection.${string}`, string>;
