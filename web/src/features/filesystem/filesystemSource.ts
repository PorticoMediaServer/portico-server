import type { FilesystemBrowseResponse, PorticoClient } from '@porticomediaserver/client-core';

export interface FilesystemPickerSource {
  browse(path?: string, signal?: AbortSignal): Promise<FilesystemBrowseResponse>;
  createDirectory(path: string, signal?: AbortSignal): Promise<FilesystemBrowseResponse>;
}

export type FilesystemClient = Pick<PorticoClient, 'browseFilesystem' | 'createFilesystemDirectory'>;

export type FilesystemFailureKind = 'permission' | 'offline' | 'failure';

export interface FilesystemFailure {
  kind: FilesystemFailureKind;
  message: string;
}

function errorStatus(reason: unknown) {
  if (!reason || typeof reason !== 'object' || !('status' in reason)) return 0;
  return typeof reason.status === 'number' ? reason.status : 0;
}

function errorCode(reason: unknown) {
  if (!reason || typeof reason !== 'object' || !('code' in reason)) return '';
  return typeof reason.code === 'string' ? reason.code.toLowerCase() : '';
}

export function classifyFilesystemFailure(reason: unknown): FilesystemFailure {
  const status = errorStatus(reason);
  const code = errorCode(reason);
  const message = reason instanceof Error && reason.message.trim() ? reason.message : '';
  if (status === 401 || status === 403 || code === 'forbidden' || code.includes('permission')) {
    return { kind: 'permission', message: message || 'This account cannot browse folders on the server host.' };
  }
  const offline = typeof navigator !== 'undefined' && navigator.onLine === false;
  if (offline || reason instanceof TypeError || code.includes('network') || code.includes('offline')) {
    return { kind: 'offline', message: 'Portico could not reach the server filesystem. Save the server connection and try again.' };
  }
  return { kind: 'failure', message: message || 'The server could not open this folder.' };
}
