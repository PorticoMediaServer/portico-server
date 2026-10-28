/** Radix owns roving focus/selection. Keep its navigation out of player shortcuts. */
export function stopRadioNavigation(event: {key: string; stopPropagation(): void}) {
  if (['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key)) event.stopPropagation();
}
