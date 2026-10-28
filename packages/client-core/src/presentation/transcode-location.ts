import type {TemporaryDirectory} from '../telemetry.ts';

/** `streaming.transcodeLocation`: one stored conversion directory drawn as three choices. */
export type TranscodeLocation = 'separate' | 'memory' | 'folder';

/** The choice the stored directory makes. Until the runtime form has loaded, the capacity
 * document's own facts answer. "In memory" is not offered: the server does not place
 * conversions in memory yet and writes them to the separate folder instead, so offering it
 * would promise storage the server does not use. A directory already saved there still reads
 * as "memory", so the page shows what is stored. */
export function transcodeLocationChoice(directory: Pick<TemporaryDirectory, 'path' | 'fallback' | 'memoryPath'>, storedDirectory?: string): Readonly<{value: TranscodeLocation; memoryAvailable: boolean}> {
  const memory = directory.memoryPath, path = storedDirectory ?? (directory.fallback ? '' : directory.path);
  const value: TranscodeLocation = path === '' ? 'separate' : memory && (path === memory || path.startsWith(memory + '/')) ? 'memory' : 'folder';
  return Object.freeze({value, memoryAvailable: false});
}

/** The `temporaryDirectory` a choice writes; `folder` writes nothing and reveals the path row. */
export function transcodeLocationTarget(choice: TranscodeLocation, directory: Pick<TemporaryDirectory, 'memoryPath'> | undefined): string | undefined {
  return choice === 'separate' ? '' : choice === 'memory' && directory?.memoryPath ? directory.memoryPath : undefined;
}
