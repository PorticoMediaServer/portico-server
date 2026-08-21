import type { FilesystemClient, FilesystemPickerSource } from './filesystemSource';

function abortError() {
  return new DOMException('The request was cancelled.', 'AbortError');
}

function requestWithSignal<T>(start: () => Promise<T>, signal?: AbortSignal): Promise<T> {
  if (signal?.aborted) return Promise.reject(abortError());
  const request = start();
  if (!signal) return request;
  return new Promise<T>((resolve, reject) => {
    const abort = () => reject(abortError());
    signal.addEventListener('abort', abort, { once: true });
    request.then(resolve, reject).finally(() => signal.removeEventListener('abort', abort));
  });
}

export class HttpFilesystemSource implements FilesystemPickerSource {
  constructor(private readonly client: FilesystemClient) {}

  browse(path?: string, signal?: AbortSignal) {
    return requestWithSignal(() => this.client.browseFilesystem(path), signal);
  }

  createDirectory(path: string, signal?: AbortSignal) {
    return requestWithSignal(() => this.client.createFilesystemDirectory({ path }), signal);
  }
}
