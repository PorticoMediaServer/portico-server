// Consumer-owned entry point: the project includes no reader-core source glob
// or ambient declarations. Public imports must carry their complete type graph.
export {ReaderController, createEpubAdapter, createPdfAdapter, createComicAdapter,
  loadLiterataFont} from '@portico/reader-core';
export type {ReaderSource, ReaderLocation, PdfAdapterOptions, ComicAdapterOptions,
  ComicPageProvider} from '@portico/reader-core';
