/** Q6: one request per admitted menu hold. Browser support is an OS capability. */
export function touchMenuHaptic(): boolean {
  try {return typeof navigator.vibrate === 'function' && navigator.vibrate(30);} catch {return false;}
}
