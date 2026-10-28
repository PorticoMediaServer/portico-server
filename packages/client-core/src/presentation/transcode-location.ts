import type {TemporaryDirectory} from '../telemetry.ts';

/** `streaming.transcodeLocation`: one stored conversion directory drawn as three choices. */
export type TranscodeLocation = 'separate' | 'memory' | 'folder';

/** The choice the stored directory makes. Until the runtime form has loaded, the capacity
 * document's own facts answer. Memory is offered only where the server has a RAM file system;
 * the server keeps conversions there within `streaming.memoryCap` and spills to the separate
 * folder when it is full. */
export function transcodeLocationChoice(directory: Pick<TemporaryDirectory, 'path' | 'fallback' | 'memoryPath'>, storedDirectory?: string): Readonly<{value: TranscodeLocation; memoryAvailable: boolean}> {
  const memory = directory.memoryPath, path = storedDirectory ?? (directory.fallback ? '' : directory.path);
  const value: TranscodeLocation = path === '' ? 'separate' : memory && (path === memory || path.startsWith(memory + '/')) ? 'memory' : 'folder';
  return Object.freeze({value, memoryAvailable: !!memory});
}

/** The `temporaryDirectory` a choice writes; `folder` writes nothing and reveals the path row. */
export function transcodeLocationTarget(choice: TranscodeLocation, directory: Pick<TemporaryDirectory, 'memoryPath'> | undefined): string | undefined {
  return choice === 'separate' ? '' : choice === 'memory' && directory?.memoryPath ? directory.memoryPath : undefined;
}
