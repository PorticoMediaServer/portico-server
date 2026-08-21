import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { App } from './App';
import { DataProvider } from './data/DataProvider';
import { FixturePorticoDataSource } from './data/fixtureSource';

function LocationProbe() {
  const location = useLocation();
  return <output aria-label="Current route">{location.pathname}{location.search}</output>;
}

function renderShell(source = new FixturePorticoDataSource()) {
  return render(
    <DataProvider source={source}>
      <MemoryRouter initialEntries={['/library/fixture-tv?pivot=shows']}>
        <App />
        <LocationProbe />
      </MemoryRouter>
    </DataProvider>,
  );
}

describe('application shell', () => {
	it('does not expose mobile-only downloads in Web navigation', async () => {
		renderShell();
		await screen.findAllByRole('navigation', { name: 'Primary navigation' });
		expect(screen.queryByRole('link', { name: 'Downloads' })).not.toBeInTheDocument();
	});

	it('keeps manual sign-in available when optional remembered-account discovery fails', async () => {
		class BrowserAccountLookupFailureSource extends FixturePorticoDataSource {
			override async browserAccounts(_signal: AbortSignal): Promise<never> {
				throw new TypeError('Remembered account storage is temporarily unavailable.');
			}
		}
		const source = new BrowserAccountLookupFailureSource({
			authenticated: false,
			setupRequired: false,
			serverName: 'Family Media',
		});
		render(
			<DataProvider source={source}>
				<MemoryRouter><App /></MemoryRouter>
			</DataProvider>,
		);

		expect(await screen.findByRole('heading', { name: 'Sign in to Family Media' })).toBeInTheDocument();
		expect(screen.getByLabelText('Username or email')).toBeEnabled();
		expect(screen.queryByText('Server session could not be loaded')).not.toBeInTheDocument();
	});

	it('contains focus in the mobile navigation drawer and restores its trigger', async () => {
		const originalMatchMedia = window.matchMedia;
		window.matchMedia = (query: string): MediaQueryList => ({
			matches: query === '(max-width: 900px)',
			media: query,
			onchange: null,
			addEventListener: () => undefined,
			removeEventListener: () => undefined,
			addListener: () => undefined,
			removeListener: () => undefined,
			dispatchEvent: () => true,
		});
		try {
			renderShell();
			const trigger = await screen.findByRole('button', { name: 'Open navigation' });
			const sidebar = document.querySelector<HTMLElement>('.sidebar');
			if (!sidebar) throw new Error('Missing application sidebar');
			expect(sidebar).toHaveAttribute('inert');
			expect(sidebar).toHaveAttribute('aria-hidden', 'true');

			fireEvent.click(trigger);
			const drawer = await screen.findByRole('dialog', { name: 'Primary navigation' });
			const close = within(drawer).getByRole('button', { name: 'Close navigation' });
			await waitFor(() => expect(close).toHaveFocus());
			expect(document.querySelector('.app-frame')).toHaveAttribute('inert');

			fireEvent.keyDown(window, { key: 'Tab', shiftKey: true });
			expect(within(drawer).getByRole('button', { name: 'Library options for Movies' })).toHaveFocus();
			fireEvent.keyDown(window, { key: 'Tab' });
			expect(close).toHaveFocus();

			fireEvent.keyDown(window, { key: 'Escape' });
			await waitFor(() => expect(trigger).toHaveFocus());
			expect(sidebar).toHaveAttribute('inert');
			expect(document.querySelector('.app-frame')).not.toHaveAttribute('inert');

			fireEvent.click(trigger);
			const backdrop = await waitFor(() => {
				const element = document.querySelector<HTMLElement>('.mobile-sidebar-backdrop');
				if (!element) throw new Error('Missing mobile navigation backdrop');
				return element;
			});
			fireEvent.pointerDown(backdrop);
			await waitFor(() => expect(trigger).toHaveFocus());
			expect(sidebar).toHaveAttribute('inert');
		} finally {
			window.matchMedia = originalMatchMedia;
		}
	});

  it('shows live search results in the overlay root and Enter opens full search', async () => {
    renderShell();
    const input = await screen.findByRole('combobox', { name: 'Quick search' });
    fireEvent.focus(input);
    fireEvent.change(input, { target: { value: 'Fargo' } });
    const overlays = document.getElementById('portico-overlays');
    if (!overlays) throw new Error('Missing overlay root');
    const result = await within(overlays).findByRole('link', { name: 'Open Fargo' });
    expect(overlays).toContainElement(result);
    fireEvent.keyDown(input, { key: 'Enter' });
    await waitFor(() => expect(screen.getByLabelText('Current route')).toHaveTextContent('/search?q=Fargo'));
  });

  it('opens the profile menu through the shared overlay layer', async () => {
    renderShell();
    const profileButton = await screen.findByRole('button', { name: /Open profile menu for Portico Review/i });
    await waitFor(() => expect(profileButton.querySelector('.notification-count-badge')).toHaveTextContent('1'));
    expect(profileButton).toHaveAccessibleName(/1/);
    expect(screen.queryByRole('button', { name: 'Notifications' })).not.toBeInTheDocument();
    fireEvent.click(profileButton);
    const menu = await screen.findByRole('menu');
    expect(document.getElementById('portico-overlays')).toContainElement(menu);
    expect(screen.getByRole('menuitem', { name: /^Notifications/ })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: 'Account settings' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('menuitem', { name: 'Watch With Friends' }));
    expect(await screen.findByRole('heading', { name: 'Watch With Friends' })).toBeInTheDocument();
    expect(screen.getByLabelText('Current route')).toHaveTextContent('/watch-with-friends');
  });

  it('resizes the desktop rail within product bounds and restores its default width', async () => {
    localStorage.removeItem('portico.sidebar-width.v1');
    renderShell();
    const resizeHandle = await screen.findByRole('separator', { name: 'Resize navigation' });
    const shell = document.querySelector<HTMLElement>('.shell');
    if (!shell) throw new Error('Missing application shell');

    expect(shell.style.getPropertyValue('--sidebar-expanded-width')).toBe('278px');
    fireEvent.keyDown(resizeHandle, { key: 'Home' });
    expect(shell.style.getPropertyValue('--sidebar-expanded-width')).toBe('140px');
    fireEvent.keyDown(resizeHandle, { key: 'End' });
    expect(shell.style.getPropertyValue('--sidebar-expanded-width')).toBe('348px');
    fireEvent.doubleClick(resizeHandle);
    expect(shell.style.getPropertyValue('--sidebar-expanded-width')).toBe('278px');
    localStorage.removeItem('portico.sidebar-width.v1');
  });
});
