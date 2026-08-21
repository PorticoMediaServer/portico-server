import type { FilesystemBrowseResponse } from '@porticomediaserver/client-core';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { FilesystemPickerDialog } from './FilesystemPickerDialog';
import { FixtureFilesystemSource, fixtureDirectory } from './FixtureFilesystemSource';
import type { FilesystemPickerSource } from './filesystemSource';

const roots = [
  { name: 'Media volume', path: '/srv/media' },
  { name: 'Archive NAS', path: '/mnt/archive' },
];

function fixture() {
  return new FixtureFilesystemSource({
    roots,
    defaultPath: '/srv/media',
    directories: [
      fixtureDirectory('/srv/media', [
        { name: 'Movies' },
        { name: 'TV Shows' },
        { name: 'Private', readable: false },
      ], roots),
      fixtureDirectory('/srv/media/Movies', [], roots),
      fixtureDirectory('/srv/media/TV Shows', [], roots),
      fixtureDirectory('/mnt/archive', [{ name: 'Classics' }], roots),
      fixtureDirectory('/mnt/archive/Classics', [], roots),
    ],
  });
}

function renderPicker(source: FilesystemPickerSource, options: Partial<Parameters<typeof FilesystemPickerDialog>[0]> = {}) {
  const onSelect = vi.fn();
  const onCancel = vi.fn();
  const result = render(<FilesystemPickerDialog
    source={source}
    title="Choose library folder"
    description="Select a folder stored on the server host."
    onSelect={onSelect}
    onCancel={onCancel}
    {...options}
  />);
  return { ...result, onSelect, onCancel };
}

describe('FilesystemPickerDialog', () => {
  it('navigates roots, parents, and folders while returning only the validated absolute path', async () => {
    const source = fixture();
    const { onSelect } = renderPicker(source);
    const dialog = await screen.findByRole('dialog', { name: 'Choose library folder' });
    expect(within(dialog).getByRole('button', { name: 'Open folder Movies' })).toBeEnabled();
    expect(within(dialog).getByRole('button', { name: 'Folder Private is not readable' })).toBeDisabled();
    expect(within(dialog).queryByRole('button', { name: 'New folder' })).not.toBeInTheDocument();

    const movies = within(dialog).getByRole('button', { name: 'Open folder Movies' });
    const tv = within(dialog).getByRole('button', { name: 'Open folder TV Shows' });
    movies.focus();
    fireEvent.keyDown(within(dialog).getByRole('list', { name: 'Folders in /srv/media' }), { key: 'ArrowDown' });
    expect(tv).toHaveFocus();
    fireEvent.click(tv);
    await waitFor(() => expect(within(dialog).getByText('/srv/media/TV Shows')).toBeInTheDocument());

    fireEvent.click(within(dialog).getByRole('button', { name: 'Open parent folder' }));
    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Open folder Movies' })).toBeInTheDocument());
    fireEvent.click(within(dialog).getByRole('button', { name: /Archive NAS/ }));
    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Open folder Classics' })).toBeInTheDocument());
    fireEvent.click(within(dialog).getByRole('button', { name: 'Select folder' }));
    expect(onSelect).toHaveBeenCalledWith('/mnt/archive');
  });

  it('treats a manually typed path as navigation until the server validates it', async () => {
    const source = fixture();
    const browse = vi.spyOn(source, 'browse');
    const { onSelect } = renderPicker(source);
    const dialog = await screen.findByRole('dialog', { name: 'Choose library folder' });
    const pathInput = within(dialog).getByRole('textbox', { name: 'Server path' });
    await waitFor(() => expect(pathInput).toHaveValue('/srv/media'));

    fireEvent.change(pathInput, { target: { value: 'relative/media' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Open path' }));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('absolute server path');
    expect(browse).toHaveBeenCalledTimes(1);

    fireEvent.change(pathInput, { target: { value: '/mnt/archive' } });
    fireEvent.submit(pathInput.closest('form') as HTMLFormElement);
    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Open folder Classics' })).toBeInTheDocument());
    fireEvent.click(within(dialog).getByRole('button', { name: 'Select folder' }));
    expect(onSelect).toHaveBeenLastCalledWith('/mnt/archive');
  });

  it('validates and creates a real child folder before selecting it', async () => {
    const source = fixture();
    const createDirectory = vi.spyOn(source, 'createDirectory');
    const { onSelect } = renderPicker(source, { canCreateDirectory: true });
    const dialog = await screen.findByRole('dialog', { name: 'Choose library folder' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'New folder' }));
    const input = within(dialog).getByRole('textbox', { name: /New folder inside/ });
    fireEvent.change(input, { target: { value: 'Invalid/Name' } });
    fireEvent.submit(input.closest('form') as HTMLFormElement);
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('path separators');
    expect(createDirectory).not.toHaveBeenCalled();

    fireEvent.change(input, { target: { value: 'Documentaries' } });
    fireEvent.submit(input.closest('form') as HTMLFormElement);
    await waitFor(() => expect(createDirectory).toHaveBeenCalledWith('/srv/media/Documentaries', expect.any(AbortSignal)));
    expect((await within(dialog).findAllByText('/srv/media/Documentaries')).length).toBeGreaterThan(0);
    fireEvent.click(within(dialog).getByRole('button', { name: 'Select folder' }));
    expect(onSelect).toHaveBeenCalledWith('/srv/media/Documentaries');
  });

  it('renders loading and empty-folder states without inventing host content', async () => {
    let resolveBrowse!: (response: FilesystemBrowseResponse) => void;
    const source: FilesystemPickerSource = {
      browse: vi.fn(() => new Promise<FilesystemBrowseResponse>((resolve) => { resolveBrowse = resolve; })),
      createDirectory: vi.fn(async () => { throw new Error('Directory creation is not available in this state fixture.'); }),
    };
    renderPicker(source);
    expect(screen.getByText('Loading server folders')).toBeInTheDocument();
    await act(async () => resolveBrowse({ path: '/srv/empty', parent: '/srv', roots: [{ name: 'Empty', path: '/srv/empty' }], entries: [] }));
    expect(await screen.findByText('No folders here')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Select folder' })).toBeEnabled();
  });

  it.each([
    { label: 'permission', error: Object.assign(new Error('Only the server owner can browse host folders.'), { status: 403, code: 'forbidden' }), title: 'Folder access is unavailable' },
    { label: 'offline', error: new TypeError('Failed to fetch'), title: 'Server connection lost' },
    { label: 'failure', error: Object.assign(new Error('Folder could not be opened.'), { status: 400, code: 'folder_unavailable' }), title: 'Folder could not be opened' },
  ])('renders a truthful $label failure state', async ({ error, title }) => {
    const source = new FixtureFilesystemSource({ browseFailure: error });
    renderPicker(source);
    expect(await screen.findByText(title)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Select folder' })).toBeDisabled();
  });

  it('keeps the last validated folder selected when later navigation fails', async () => {
    const source = new FixtureFilesystemSource({
      roots,
      defaultPath: '/srv/media',
      directories: [fixtureDirectory('/srv/media', [], roots)],
      browseFailure: (path: string) => path === '/missing' ? Object.assign(new Error('Folder could not be opened.'), { status: 400, code: 'folder_unavailable' }) : undefined,
    });
    const { onSelect } = renderPicker(source);
    const dialog = await screen.findByRole('dialog', { name: 'Choose library folder' });
    const input = within(dialog).getByRole('textbox', { name: 'Server path' });
    await waitFor(() => expect(input).toHaveValue('/srv/media'));
    fireEvent.change(input, { target: { value: '/missing' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Open path' }));
    expect(await within(dialog).findByText('Folder could not be opened')).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Select folder' }));
    expect(onSelect).toHaveBeenCalledWith('/srv/media');
  });
});
