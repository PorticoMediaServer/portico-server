// Consumer-owned entry point: the project includes no reader-core source glob
// or ambient declarations. Public imports must carry their complete type graph.
export {ReaderController, createEpubAdapter, createPdfAdapter, createComicAdapter,
  loadLiterataFont, rasterInfo} from '@portico/reader-core';
export type {ReaderSource, ReaderLocation, PdfAdapterOptions, ComicAdapterOptions,
  ComicPageProvider, RasterInfo} from '@portico/reader-core';

import {rasterInfo} from '@portico/reader-core';
import type {ComicPage, RasterInfo} from '@portico/reader-core';
// A provider derives the returned variant's dimensions from bounded bytes,
// before any browser decoder, without importing adapter-private modules.
export function inspectedComicPage(index:number,name:string,bytes:Uint8Array):ComicPage {
  const image:RasterInfo=rasterInfo(bytes);
  return {index,name,bytes,image};
}
