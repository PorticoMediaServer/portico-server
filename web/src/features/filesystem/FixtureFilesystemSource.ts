import type { FilesystemBrowseEntry, FilesystemBrowseResponse, FilesystemRoot } from '@portico/client-core';
import { filesystemBreadcrumbs, joinFilesystemPath, stripTrailingPathSeparators } from './filesystemPath';
import type { FilesystemPickerSource } from './filesystemSource';

type FailureResolver = unknown | ((path: string) => unknown | undefined);

export interface FixtureFilesystemSourceOptions {
  roots?: FilesystemRoot[];
  directories?: FilesystemBrowseResponse[];
  defaultPath?: string;
  browseFailure?: FailureResolver;
  createFailure?: FailureResolver;
}

function cloneResponse(response: FilesystemBrowseResponse): FilesystemBrowseResponse {
  return {
    ...response,
    entries: response.entries.map((entry) => ({ ...entry })),
    roots: response.roots?.map((root) => ({ ...root })),
  };
}

function failureFor(resolver: FailureResolver | undefined, path: string) {
  return typeof resolver === 'function' ? resolver(path) : resolver;
}

function fixtureError(status: number, code: string, message: string) {
  return Object.assign(new Error(message), { status, code });
}

export class FixtureFilesystemSource implements FilesystemPickerSource {
  private readonly directories = new Map<string, FilesystemBrowseResponse>();
  private readonly roots: FilesystemRoot[];
  private readonly defaultPath: string;

  constructor(private readonly options: FixtureFilesystemSourceOptions = {}) {
    this.roots = (options.roots ?? options.directories?.find((item) => item.roots?.length)?.roots ?? []).map((root) => ({ ...root }));
    this.defaultPath = options.defaultPath?.trim() || this.roots[0]?.path || options.directories?.[0]?.path || '';
    for (const directory of options.directories ?? []) {
      const response = cloneResponse({ ...directory, roots: directory.roots ?? this.roots });
      this.directories.set(this.key(directory.path), response);
    }
  }

  async browse(path?: string, signal?: AbortSignal) {
    this.checkSignal(signal);
    const requested = path?.trim() || this.defaultPath;
    const failure = failureFor(this.options.browseFailure, requested);
    if (failure) throw failure;
    if (!requested) return { path: '', roots: this.roots.map((root) => ({ ...root })), entries: [] };
    const response = this.directories.get(this.key(requested));
    if (!response) throw fixtureError(400, 'folder_unavailable', 'Folder could not be opened.');
    return cloneResponse(response);
  }

  async createDirectory(path: string, signal?: AbortSignal) {
    this.checkSignal(signal);
    const requested = stripTrailingPathSeparators(path);
    const failure = failureFor(this.options.createFailure, requested);
    if (failure) throw failure;
    if (this.directories.has(this.key(requested))) throw fixtureError(409, 'folder_exists', 'Folder already exists.');
    const parent = this.parentFor(requested);
    const parentResponse = this.directories.get(this.key(parent));
    if (!parentResponse) throw fixtureError(400, 'parent_unavailable', 'Parent folder does not exist or could not be opened.');
    const name = requested.slice(parent.length).replace(/^[\\/]+/, '');
    const entry: FilesystemBrowseEntry = { kind: 'directory', name, path: requested, readable: true };
    parentResponse.entries = [...parentResponse.entries.filter((item) => this.key(item.path) !== this.key(requested)), entry]
      .sort((left, right) => left.name.localeCompare(right.name));
    const response: FilesystemBrowseResponse = {
      path: requested,
      parent,
      roots: this.roots.map((root) => ({ ...root })),
      entries: [],
    };
    this.directories.set(this.key(requested), response);
    return cloneResponse(response);
  }

  snapshot(path: string) {
    const response = this.directories.get(this.key(path));
    return response ? cloneResponse(response) : undefined;
  }

  private parentFor(path: string) {
    const breadcrumbs = filesystemBreadcrumbs(path);
    return breadcrumbs.length > 1 ? breadcrumbs[breadcrumbs.length - 2]?.path ?? '' : '';
  }

  private key(path: string) {
    const normalized = stripTrailingPathSeparators(path);
    return /^[A-Za-z]:[\\/]/.test(normalized) || /^(?:\\\\|\/\/)/.test(normalized)
      ? normalized.toLocaleLowerCase()
      : normalized;
  }

  private checkSignal(signal?: AbortSignal) {
    if (signal?.aborted) throw new DOMException('The request was cancelled.', 'AbortError');
  }
}

export function fixtureDirectory(path: string, entries: Array<{ name: string; readable?: boolean }> = [], roots?: FilesystemRoot[]): FilesystemBrowseResponse {
  const parentIndex = Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\'));
  const parent = parentIndex > 0 ? path.slice(0, parentIndex) : undefined;
  return {
    path,
    ...(parent ? { parent } : {}),
    roots,
    entries: entries.map((entry) => ({
      kind: 'directory',
      name: entry.name,
      path: joinFilesystemPath(path, entry.name),
      readable: entry.readable ?? true,
    })),
  };
}
