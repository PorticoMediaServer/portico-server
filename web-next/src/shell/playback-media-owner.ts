type MediaRuntime<Element> = Readonly<{mount: (element: Element) => () => void}>;

/** Owns attachment only. React owns the one persistent element; the actual runtime owns disposal. */
export class PlaybackMediaOwner<Element> {
  private element?: Element;
  private current?: {runtime: MediaRuntime<Element>; release: () => void};
  adopt(runtime: MediaRuntime<Element> | undefined, element: Element): (() => void) | undefined {
    if (this.element !== undefined && this.element !== element) throw new Error('playback-element-replaced');
    this.element = element;
    if (runtime && this.current?.runtime === runtime) return this.current.release;
    this.current?.release();
    if (!runtime) return undefined;
    const unmount = runtime.mount(element);
    const attachment = {runtime, release: () => {
      if (this.current !== attachment) return;
      this.current = undefined;
      unmount();
    }};
    this.current = attachment;
    return attachment.release;
  }
}
