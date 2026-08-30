import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { ActionFavoriteIcon, PorticoIcon } from './PorticoIcons';

describe('PorticoIcon', () => {
  it('renders generated geometry for a semantic ID', () => {
    const { container } = render(<PorticoIcon aria-label="Play" id="playback.play" size={32} />);
    const icon = container.querySelector('svg');

    expect(icon).toHaveAttribute('aria-label', 'Play');
    expect(icon).toHaveAttribute('height', '32');
    expect(icon).toHaveAttribute('width', '32');
    expect(icon).toHaveClass('portico-icon', 'portico-icon-play');
    expect(icon?.querySelectorAll('path, circle, line, polyline, polygon, rect').length).toBeGreaterThan(0);
  });

  it('uses the governed selected fill for favorites', () => {
    const { container } = render(<PorticoIcon color="#fff" id="action.favorite" state="selected" />);

    expect(container.querySelector('svg')).toHaveAttribute('fill', '#fff');
  });

  it('routes semantic component exports through governed IDs', () => {
    const { container } = render(<ActionFavoriteIcon aria-label="Favorite" />);

    expect(container.querySelector('svg')).toHaveClass('portico-icon-heart', 'portico-icon');
  });

  it('fails closed for an unknown semantic ID', () => {
    expect(() => render(<PorticoIcon id={'not.a.real.icon' as never} />)).toThrow(
      /Unknown Portico semantic icon ID/,
    );
  });
});
