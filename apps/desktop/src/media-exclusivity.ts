/** One native media engine, one owner at a time.
 *
 * The desktop's media adapter commands the device beneath the WebView. The launch page's
 * local-only player owns that device while it is visible; the online adapter must not send
 * its first command until the local player's actual retirement has settled. Commands are
 * gated on the same task, so a load cannot overtake a release that is still in flight.
 */
export class DesktopMediaExclusivity {
  private gate: Promise<void> = Promise.resolve();
  private settle?: () => void;

  /** The offline player now owns the device; nothing else may command it until it retires. */
  claim(): void {
    if (this.settle) return;
    this.gate = new Promise<void>(resolve => { this.settle = resolve; });
  }

  /** Its real retirement replaces the claim; the next owner waits for it to settle. */
  hold(task: Promise<void>): void {
    this.settle?.();
    this.settle = undefined;
    this.gate = task;
  }

  /** Resolves when no local owner holds the device; a failed retirement stays a refusal. */
  wait(): Promise<void> { return this.gate; }
}
