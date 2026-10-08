import {analysisSelectionComplete, type AnalysisMatrix, type LibrarySettings} from '../administration.ts';

/** Toggle the real runner operation and preserve its complete dependency chain. */
export function setLibraryAnalysis(settings: LibrarySettings, matrix: AnalysisMatrix, operation: string, enabled: boolean): LibrarySettings {
  const selected = new Set(settings.analysis);
  if (enabled) {
    selected.add(operation);
    for (;;) {
      const missing = analysisSelectionComplete(matrix, [...selected]);
      if (!missing.length) break;
      for (const id of missing) selected.add(id);
    }
  } else {
    selected.delete(operation);
    for (;;) {
      const invalid = matrix.operations.filter(op => selected.has(op.id) && op.requires.some(id => !selected.has(id)));
      if (!invalid.length) break;
      for (const op of invalid) selected.delete(op.id);
    }
  }
  return {...settings, analysis: [...selected], navigation: {...settings.navigation, chapterThumbnailMode: selected.has('chapter_images') ? 'generated' : 'embedded'}};
}
