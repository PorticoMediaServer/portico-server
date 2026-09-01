import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { clearArtworkFailureCache, MAX_REMEMBERED_ARTWORK_FAILURES, rememberArtworkFailure } from '../../data/artworkFailureCache';
import { StableImage, useStableBackdrop } from './StableImage';

class FakeImage {
  static instances: FakeImage[] = [];
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  src = '';
  decode = vi.fn().mockResolvedValue(undefined);
  constructor() { FakeImage.instances.push(this); }
}

function installImageHarness() {
  FakeImage.instances = [];
  vi.stubGlobal('Image', FakeImage);
  return FakeImage.instances;
}

afterEach(() => {
  clearArtworkFailureCache();
  vi.unstubAllGlobals();
});

describe('StableImage', () => {
  it('shows its geometry-preserving fallback and preserves native lazy loading while the initial image is slow', () => {
    const images = installImageHarness();
    const view = render(<StableImage src="/art/poster.jpg" alt="Poster" loading="lazy" fallback={<span>Poster unavailable</span>} />);
    expect(screen.getByText('Poster unavailable')).toBeInTheDocument();
    expect(view.container.querySelector('img[data-stable-image-stage]')).toHaveAttribute('loading', 'lazy');
    expect(view.container.querySelector('img:not([data-stable-image-stage])')).toBeNull();
    expect(images).toHaveLength(0);
  });

  it('settles a failed initial URL into its fallback across remounts', () => {
    const images = installImageHarness();
    const first = render(<StableImage src="/art/poster.jpg" alt="" fallback={<span>Poster unavailable</span>} />);
    fireEvent.error(first.container.querySelector('img[data-stable-image-stage]')!);
    first.unmount();
    const second = render(<StableImage src="/art/poster.jpg" alt="" fallback={<span>Poster unavailable</span>} />);
    expect(second.container.querySelector('img')).toBeNull();
    expect(screen.getByText('Poster unavailable')).toBeInTheDocument();
    expect(images).toHaveLength(0);
  });

  it('attempts a changed URL after an earlier resource failed', async () => {
    installImageHarness();
    const view = render(<StableImage src="/art/old.jpg" alt="" fallback={<span>Unavailable</span>} />);
    fireEvent.error(view.container.querySelector('img[data-stable-image-stage]')!);
    view.rerender(<StableImage src="/art/new.jpg" alt="" fallback={<span>Unavailable</span>} />);
    expect(view.container.querySelector('img:not([data-stable-image-stage])')).toBeNull();
    fireEvent.load(view.container.querySelector('img[data-stable-image-stage]')!);
    await waitFor(() => expect(view.container.querySelector('img:not([data-stable-image-stage])')).toHaveAttribute('src', '/art/new.jpg'));
  });

  it('retains a successful image while a changed URL is staged and when it fails', async () => {
    installImageHarness();
    const view = render(<StableImage src="/art/current.jpg" alt="Current artwork" fallback={<span>Unavailable</span>} />);
    fireEvent.load(view.container.querySelector('img[data-stable-image-stage]')!);
    await waitFor(() => expect(view.container.querySelector('img:not([data-stable-image-stage])')).toHaveAttribute('alt', 'Current artwork'));
    view.rerender(<StableImage src="/art/replacement.jpg" alt="Current artwork" fallback={<span>Unavailable</span>} />);
    expect(view.container.querySelector('img:not([data-stable-image-stage])')).toHaveAttribute('src', '/art/current.jpg');
    fireEvent.error(view.container.querySelector('img[data-stable-image-stage]')!);
    expect(view.container.querySelector('img:not([data-stable-image-stage])')).toHaveAttribute('src', '/art/current.jpg');
  });

  it('does not reactivate a mounted failure after bounded cache eviction', () => {
    const images = installImageHarness();
    const view = render(<StableImage src="/art/active-miss.jpg" alt="" fallback={<span>Unavailable</span>} />);
    fireEvent.error(view.container.querySelector('img[data-stable-image-stage]')!);
    act(() => {
      for (let index = 0; index <= MAX_REMEMBERED_ARTWORK_FAILURES; index += 1) rememberArtworkFailure(`/art/miss-${index}.jpg`);
    });
    expect(view.container.querySelector('img')).toBeNull();
    expect(screen.getByText('Unavailable')).toBeInTheDocument();
    expect(images).toHaveLength(0);
  });

  it('retries the same URL once when an authoritative revision changes', async () => {
    const images = installImageHarness();
    const view = render(<StableImage src="/art/person.jpg" retryKey={1} alt="" fallback={<span>Unavailable</span>} />);
    fireEvent.error(view.container.querySelector('img[data-stable-image-stage]')!);
    view.rerender(<StableImage src="/art/person.jpg" retryKey={2} alt="" fallback={<span>Unavailable</span>} />);
    await waitFor(() => expect(view.container.querySelector('img[data-stable-image-stage]')).not.toBeNull());
    expect(images).toHaveLength(0);
    fireEvent.load(view.container.querySelector('img[data-stable-image-stage]')!);
    await waitFor(() => expect(view.container.querySelector('img:not([data-stable-image-stage])')).toHaveAttribute('src', '/art/person.jpg'));
  });

});

describe('useStableBackdrop', () => {
  it('keeps the decoded backdrop when its replacement fails', async () => {
    const images = installImageHarness();
    function Probe({ source }: { source: string }) {
      const backgroundImage = useStableBackdrop(source);
      return <output aria-label="backdrop">{backgroundImage}</output>;
    }
    const view = render(<Probe source="/art/ready.jpg" />);
    await act(async () => { images[0].onload?.(); });
    expect(screen.getByLabelText('backdrop')).toHaveTextContent('ready.jpg');
    view.rerender(<Probe source="/art/broken.jpg" />);
    act(() => images[1].onerror?.());
    expect(screen.getByLabelText('backdrop')).toHaveTextContent('ready.jpg');
  });
});
