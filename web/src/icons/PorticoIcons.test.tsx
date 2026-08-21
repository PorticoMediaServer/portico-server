import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { Heart, PorticoIcon, semanticIcon } from './PorticoIcons';

describe('PorticoIcon', () => {
  it('renders generated geometry for a semantic ID', () => {
    const { container } = render(<PorticoIcon aria-label="Play" id="playback.play" size={32} />);
    const icon = container.querySelector('svg');

    expect(icon).toHaveAttribute('aria-label', 'Play');
    expect(icon).toHaveAttribute('height', '32');
    expect(icon).toHaveAttribute('width', '32');
    expect(icon).toHaveClass('portico-icon', 'lucide-play');
    expect(icon?.querySelectorAll('path, circle, line, polyline, polygon, rect').length).toBeGreaterThan(0);
  });

  it('uses the governed selected fill for favorites', () => {
    const { container } = render(<PorticoIcon color="#fff" id="action.favorite" state="selected" />);

    expect(container.querySelector('svg')).toHaveAttribute('fill', '#fff');
  });

  it('routes compatibility-shaped exports through semantic IDs', () => {
    const { container } = render(<Heart aria-label="Favorite" />);

    expect(container.querySelector('svg')).toHaveClass('lucide-heart', 'portico-icon');
  });

  it('fails closed for an unknown semantic ID', () => {
    const Unknown = semanticIcon('not.a.real.icon' as never);

    expect(() => render(<Unknown />)).toThrow(/Unknown Portico semantic icon ID/);
  });
});
