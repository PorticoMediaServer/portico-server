import { describe, expect, it } from 'vitest';
import {
  filesystemBreadcrumbs,
  isAbsoluteFilesystemPath,
  joinFilesystemPath,
  sameFilesystemPath,
  validateNewFolderName,
} from './filesystemPath';

describe('filesystem path helpers', () => {
  it('recognizes POSIX, Windows drive, and UNC absolute paths', () => {
    expect(isAbsoluteFilesystemPath('/srv/media')).toBe(true);
    expect(isAbsoluteFilesystemPath('D:\\Media\\TV')).toBe(true);
    expect(isAbsoluteFilesystemPath('\\\\nas.local\\Media\\TV')).toBe(true);
    expect(isAbsoluteFilesystemPath('//nas.local/Media/TV')).toBe(true);
    expect(isAbsoluteFilesystemPath('media/TV')).toBe(false);
  });

  it('joins and compares paths using their host syntax', () => {
    expect(joinFilesystemPath('/srv/media', 'TV')).toBe('/srv/media/TV');
    expect(joinFilesystemPath('/', 'TV')).toBe('/TV');
    expect(joinFilesystemPath('D:\\', 'TV')).toBe('D:\\TV');
    expect(sameFilesystemPath('D:\\MEDIA\\', 'd:\\media')).toBe(true);
    expect(sameFilesystemPath('/srv/Media', '/srv/media')).toBe(false);
  });

  it('builds navigable breadcrumbs for server and network paths', () => {
    expect(filesystemBreadcrumbs('/srv/media/Movies')).toEqual([
      { label: 'Root', path: '/' },
      { label: 'srv', path: '/srv' },
      { label: 'media', path: '/srv/media' },
      { label: 'Movies', path: '/srv/media/Movies' },
    ]);
    expect(filesystemBreadcrumbs('D:\\Media\\TV').at(-1)).toEqual({ label: 'TV', path: 'D:\\Media\\TV' });
    expect(filesystemBreadcrumbs('\\\\nas.local\\Media\\TV')).toEqual([
      { label: 'nas.local\\Media', path: '\\\\nas.local\\Media' },
      { label: 'TV', path: '\\\\nas.local\\Media\\TV' },
    ]);
  });

  it('validates portable folder names without blocking valid POSIX punctuation', () => {
    expect(validateNewFolderName('', '/srv/media')).toBe('Enter a folder name.');
    expect(validateNewFolderName('../TV', '/srv/media')).toMatch(/path separators/);
    expect(validateNewFolderName('Trailers: 4K', '/srv/media')).toBe('');
    expect(validateNewFolderName('Trailers: 4K', 'D:\\Media')).toMatch(/Windows folder name/);
    expect(validateNewFolderName('CON', 'D:\\Media')).toMatch(/reserved/);
  });
});
