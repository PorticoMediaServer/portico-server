/** A callback captured by an old dialog render cannot act after replacement,
 * including the microtask between a successful save and its .then callback. */
export function dialogAction<T extends {isActive: () => boolean}>(owner: {current: T}, captured: T, action: () => void) {
  return () => {if (owner.current === captured && captured.isActive()) action();};
}
