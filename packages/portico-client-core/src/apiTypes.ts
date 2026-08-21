import type { components as hostedComponents } from "./hosted-openapi-types.js";
import type { components as serverComponents, operations as serverOperations } from "./openapi-types.js";

export type ApiSchema<Name extends keyof serverComponents["schemas"]> = serverComponents["schemas"][Name];
export type HostedApiSchema<Name extends keyof hostedComponents["schemas"]> = hostedComponents["schemas"][Name];
export type ApiOperation<Name extends keyof serverOperations> = serverOperations[Name];
export type ApiOperationQuery<Name extends keyof serverOperations> =
  serverOperations[Name] extends { parameters: { query?: infer Query } } ? Exclude<Query, undefined> : never;
export type ApiOperationPath<Name extends keyof serverOperations> =
  serverOperations[Name] extends { parameters: { path?: infer Path } } ? Exclude<Path, undefined> : never;
export type ApiOperationJSONBody<Name extends keyof serverOperations> =
  serverOperations[Name] extends { requestBody: { content: { "application/json": infer Body } } } ? Body : never;
