import type { components as MainComponents } from "./openapi-types.js";
import { productMessageIdForProblemCode } from "./productLanguage.js";

type Schema<Name extends keyof MainComponents["schemas"]> = MainComponents["schemas"][Name];

export type LibraryChannel = Schema<"LibraryChannel">;
export type LibraryChannelRule = Schema<"LibraryChannelRule">;
export type LibraryChannelBlock = Schema<"LibraryChannelBlock">;
export type LibraryChannelListResponse = Schema<"LibraryChannelListResponse">;
export type LibraryChannelSummary = Schema<"LibraryChannelSummary">;
export type AdminLibraryChannelListResponse = Schema<"AdminLibraryChannelListResponse">;
export type LibraryChannelAggregate = Schema<"LibraryChannelAggregate">;
export type LibraryChannelConfigurationRequest = Schema<"LibraryChannelConfigurationRequest">;
export type LibraryChannelGuide = Schema<"LibraryChannelGuide">;
export type LibraryChannelsGuide = Schema<"LibraryChannelsGuide">;
export type LibraryChannelTuneRequest = Schema<"LibraryChannelTuneRequest">;
export type LibraryChannelTuneResponse = Schema<"LibraryChannelTuneResponse">;
export type LibraryChannelTemplatesResponse = Schema<"LibraryChannelTemplatesResponse">;
export type LibraryChannelBlockPreset = Schema<"LibraryChannelBlockPreset">;
export type LibraryChannelApplicabilityResponse = Schema<"LibraryChannelApplicabilityResponse">;
export type LibraryChannelRestoreDefaultsRequest = Schema<"LibraryChannelRestoreDefaultsRequest">;
export type LibraryChannelRestoreDefaultsResponse = Schema<"LibraryChannelRestoreDefaultsResponse">;
export type LibraryChannelAggregateListResponse = Schema<"LibraryChannelAggregateListResponse">;
export type LibraryChannelReorderRequest = Schema<"LibraryChannelReorderRequest">;
export type LibraryChannelGeneration = Schema<"LibraryChannelGeneration">;
export type LibraryChannelHealthResponse = Schema<"LibraryChannelHealthResponse">;
export type LibraryChannelLogoAsset = Schema<"LibraryChannelLogoAsset">;

export interface LibraryChannelGuideParams {
  from?: string;
  to?: string;
  cursor?: string;
  limit?: number;
}

export interface LibraryChannelListParams {
  cursor?: string;
  limit?: number;
}

// Error codes remain transport-stable while product wording is resolved from
// the shared Product Language catalog by every client shell.
export function libraryChannelMessageIdForError(code: string): string | undefined {
  return productMessageIdForProblemCode(code);
}
