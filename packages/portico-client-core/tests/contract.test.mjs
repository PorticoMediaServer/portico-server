import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import ts from "typescript";

import { viewerPreferenceLimitsV1 } from "../dist/preferences.js";

const packageRoot = path.resolve(import.meta.dirname, "..");
const clientSourcePath = path.join(packageRoot, "src/client.ts");
const serverOpenAPIPath = path.resolve(packageRoot, "../../api/openapi/portico-server.openapi.json");
const hostedOpenAPIPath = path.resolve(packageRoot, "../../api/openapi/hosted/portico-hosted.openapi.json");

const clientSource = fs.readFileSync(clientSourcePath, "utf8");
const sourceFile = ts.createSourceFile(clientSourcePath, clientSource, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
const serverOpenAPI = JSON.parse(fs.readFileSync(serverOpenAPIPath, "utf8"));
const hostedOpenAPI = JSON.parse(fs.readFileSync(hostedOpenAPIPath, "utf8"));

const requestCallNames = new Set(["request", "formRequest", "hostedFormRequest", "apiUrl", "resourceUrl"]);

function callName(expression) {
  if (ts.isIdentifier(expression)) return expression.text;
  if (ts.isPropertyAccessExpression(expression)) return expression.name.text;
  return "";
}

function routeText(expression) {
  if (ts.isStringLiteralLike(expression)) return expression.text;
  if (!ts.isTemplateExpression(expression)) return "";
  let result = expression.head.text;
  for (const span of expression.templateSpans) {
    const expressionText = span.expression.getText(sourceFile);
    if (expressionText.startsWith("hostedCursorQuery(") || expressionText.startsWith("apiQuery(")) result += "";
    else if (/['"`]\?/.test(expressionText) && !result.endsWith("/")) result += "?query";
    else result += "{value}";
    result += span.literal.text;
  }
  return result;
}

function methodFromObject(expression) {
  if (!expression || !ts.isObjectLiteralExpression(expression)) return "";
  for (const property of expression.properties) {
    if (!ts.isPropertyAssignment(property)) continue;
    const name = property.name.getText(sourceFile).replace(/["']/g, "");
    if (name !== "method" || !ts.isStringLiteralLike(property.initializer)) continue;
    return property.initializer.text.toLowerCase();
  }
  return "";
}

function enclosingFunctionName(node) {
  for (let current = node.parent; current; current = current.parent) {
    if (ts.isFunctionDeclaration(current) && current.name) return current.name.text;
  }
  return "";
}

function enclosingClientMethod(node) {
  for (let current = node.parent; current; current = current.parent) {
    if (ts.isPropertyAssignment(current) || ts.isMethodDeclaration(current)) {
      return current.name?.getText(sourceFile).replace(/["']/g, "") ?? "unknown";
    }
    if (ts.isFunctionDeclaration(current)) return current.name?.text ?? "unknown";
  }
  return "unknown";
}

function collectClientOperations() {
  const operations = [];
  const visit = (node) => {
    if (ts.isCallExpression(node)) {
      const name = callName(node.expression);
      if (requestCallNames.has(name)) {
        const route = routeText(node.arguments[0]);
        if (route.startsWith("/api/")) {
          const hosted = name === "hostedFormRequest" || enclosingFunctionName(node) === "createHostedServicesClient";
          let method = "get";
          if (name === "request") method = methodFromObject(node.arguments[1]) || "get";
          if (name === "formRequest" || name === "hostedFormRequest") {
            method = node.arguments[2] && ts.isStringLiteralLike(node.arguments[2]) ? node.arguments[2].text.toLowerCase() : "post";
          }
          operations.push({
            contract: hosted ? "hosted" : "server",
            method,
            owner: enclosingClientMethod(node),
            route: route.split("?", 1)[0]
          });
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(sourceFile);
  return operations;
}

function collectClientMethods() {
  const methods = new Map();
  const visit = (node) => {
    if (ts.isPropertyAssignment(node) && (ts.isArrowFunction(node.initializer) || ts.isFunctionExpression(node.initializer))) {
      const name = node.name.getText(sourceFile).replace(/["']/g, "");
      methods.set(name, node.initializer);
    }
    ts.forEachChild(node, visit);
  };
  visit(sourceFile);
  return methods;
}

function normalizedPath(value, contract) {
  let route = value.replace(/\{[^}]+\}/g, "{}");
  if (contract === "server" && !route.startsWith("/api/")) route = `/api${route}`;
  return route.replace(/\/+$/, "") || "/";
}

function contractOperations(document, contract) {
  const operations = new Map();
  for (const [route, pathItem] of Object.entries(document.paths ?? {})) {
    const normalized = normalizedPath(route, contract);
    operations.set(normalized, new Set(Object.keys(pathItem).map((method) => method.toLowerCase())));
  }
  return operations;
}

test("every client-owned request path and method exists in canonical OpenAPI", () => {
  const contracts = {
    server: contractOperations(serverOpenAPI, "server"),
    hosted: contractOperations(hostedOpenAPI, "hosted")
  };
  const clientOperations = collectClientOperations();
  assert.ok(clientOperations.length >= 200, `Expected the full client operation surface, found ${clientOperations.length}`);

  const missing = [];
  for (const operation of clientOperations) {
    const route = normalizedPath(operation.route, operation.contract);
    const methods = contracts[operation.contract].get(route);
    if (!methods?.has(operation.method)) {
      missing.push(`${operation.contract} ${operation.method.toUpperCase()} ${operation.route} (${operation.owner})`);
    }
  }
  assert.deepEqual(missing, []);
});

test("release-candidate preference, profile, notification, and feedback operations all have client wrappers", () => {
  const operations = collectClientOperations();
  const required = {
    viewerPreferenceBundle: ["get", "/api/preferences"],
    patchViewerPreferenceDocument: ["patch", "/api/preferences/{value}"],
    recordViewerProfileActivation: ["post", "/api/preferences/profile-activation"],
    accountProfiles: ["get", "/api/account/profiles"],
    createProfileAdministrationProof: ["post", "/api/account/profile-admin-proofs"],
    createAccountProfile: ["post", "/api/account/profiles"],
    updateAccountProfile: ["patch", "/api/account/profiles/{value}"],
    deleteAccountProfile: ["delete", "/api/account/profiles/{value}"],
    reorderAccountProfiles: ["put", "/api/account/profiles/order"],
    setAccountProfilePIN: ["put", "/api/account/profiles/{value}/pin"],
    clearAccountProfilePIN: ["delete", "/api/account/profiles/{value}/pin"],
    createAutomaticProfileTrust: ["post", "/api/account/profile-trusts"],
    redeemAutomaticProfileTrust: ["post", "/api/account/profile-trusts/redeem"],
    revokeAutomaticProfileTrusts: ["delete", "/api/account/profile-trusts"],
    viewerNotifications: ["get", "/api/notifications"],
    updateViewerNotificationReceipts: ["post", "/api/notifications/receipts"],
    markAllViewerNotificationsRead: ["post", "/api/notifications/read-all"],
    viewerFeedbackCapabilities: ["get", "/api/feedback/capabilities"],
    submitViewerFeedback: ["post", "/api/feedback"],
    ownerViewerFeedback: ["get", "/api/admin/viewer-feedback"],
    ownerViewerNotificationRecipients: ["get", "/api/admin/viewer-notification-recipients"],
    updateOwnerViewerFeedback: ["patch", "/api/admin/viewer-feedback/{value}"],
    createOwnerViewerNotice: ["post", "/api/admin/viewer-notifications"]
  };
  const missing = [];
  for (const [owner, [method, route]] of Object.entries(required)) {
    if (!operations.some(operation => operation.contract === "server" && operation.owner === owner && operation.method === method && operation.route === route)) {
      missing.push(`${owner}: ${method.toUpperCase()} ${route}`);
    }
  }
  assert.deepEqual(missing, []);
});

test("saved, settings, and release-candidate product methods expose explicit AbortSignal input", () => {
  const methods = collectClientMethods();
  const cancellableMethods = [
    "favorites", "setFavorite",
    "savedShareCandidates",
    "playlists", "playlist", "playlistItems", "createPlaylist", "updatePlaylist", "deletePlaylist", "mutatePlaylistItems",
    "collections", "collection", "collectionItems", "createCollection", "updateCollection", "deleteCollection", "mutateCollectionMemberships",
    "savedViews", "savedView", "createSavedView", "updateSavedView", "deleteSavedView", "browseSavedView",
    "settings", "settingsSummary", "updateSettings",
    "liveTvGuide", "liveTvChannels", "dvrStatus", "adminDvrOperationalStatus",
    "viewerPreferenceBundle", "patchViewerPreferenceDocument", "recordViewerProfileActivation",
    "accountProfiles", "createProfileAdministrationProof", "createAccountProfile", "updateAccountProfile", "deleteAccountProfile",
    "reorderAccountProfiles", "setAccountProfilePIN", "clearAccountProfilePIN", "createAutomaticProfileTrust", "redeemAutomaticProfileTrust", "revokeAutomaticProfileTrusts",
    "viewerNotifications", "updateViewerNotificationReceipts", "markAllViewerNotificationsRead",
    "viewerFeedbackCapabilities", "submitViewerFeedback", "ownerViewerFeedback", "ownerViewerNotificationRecipients", "updateOwnerViewerFeedback", "createOwnerViewerNotice"
  ];

  const missing = [];
  const untyped = [];
  for (const name of cancellableMethods) {
    const method = methods.get(name);
    if (!method) {
      missing.push(name);
      continue;
    }
    const init = method.parameters.find((parameter) => parameter.name.getText(sourceFile) === "init");
    if (!init || !init.type || !/RequestSignal/.test(init.type.getText(sourceFile))) untyped.push(name);
  }

  assert.deepEqual(missing, []);
  assert.deepEqual(untyped, []);
});

test("client-owned source contains no retired transport or route compatibility surface", () => {
  const sourcePaths = fs.readdirSync(path.join(packageRoot, "src"))
    .filter((name) => name.endsWith(".ts") && name !== "openapi-types.ts" && name !== "hosted-openapi-types.ts")
    .map((name) => path.join(packageRoot, "src", name));
  const source = sourcePaths.map((file) => fs.readFileSync(file, "utf8")).join("\n");

  assert.doesNotMatch(source, /\/api\/v1(?:\/|["'`])/);
  assert.doesNotMatch(source, /\b(?:iframeBridge|createIframeBridge|IframeBridge)\b/);
  assert.doesNotMatch(source, /\b(?:relayTransport|createRelayTransport|RelayTransport)\b/);
  assert.doesNotMatch(source, /\b(?:PorticoServicesClient|createPorticoServicesClient)\b/);
  assert.doesNotMatch(source, /RuntimeMode\s*\|\s*["']local["']/);
  assert.doesNotMatch(source, /\b(?:SettingsMap|saveSettings)\b/);
});

test("generated wire types identify their canonical documents", () => {
  const serverTypes = fs.readFileSync(path.join(packageRoot, "src/openapi-types.ts"), "utf8");
  const hostedTypes = fs.readFileSync(path.join(packageRoot, "src/hosted-openapi-types.ts"), "utf8");
  assert.match(serverTypes, /"\/libraries\/\{id\}"/);
  assert.match(hostedTypes, /"\/api\/server-claims"/);
  assert.doesNotMatch(serverTypes, /"\/api\/server-claims"/);
  assert.doesNotMatch(hostedTypes, /"\/libraries\/\{id\}"/);
  const publicTypes = fs.readFileSync(path.join(packageRoot, "src/types.ts"), "utf8");
  for (const schema of [
    "ViewerPreferenceBundle", "ViewerPreferencePatch", "ManagedProfileDirectory", "ProfileAdministrationProofRequest",
    "AutomaticProfileTrustRequest", "ViewerNotificationPage", "NotificationReceiptMutation", "ViewerFeedbackSubmission",
    "OwnerFeedbackPage", "OwnerFeedbackUpdateRequest", "OwnerNotificationRecipientDirectory", "OwnerNoticeRequest"
  ]) assert.match(publicTypes, new RegExp(`MainSchema<"${schema}">`));
  assert.match(serverTypes, /OwnerFeedbackReporter: \{[\s\S]*authority: "local";[\s\S]*\} \| \{[\s\S]*authority: "hosted";/);
  assert.match(serverTypes, /OwnerNoticeRequest: \{[\s\S]*audience: "profile";[\s\S]*\} \| \{[\s\S]*audience: "account-admin";/);
});

test("Client Core preference normalization limits match the published V1 API contract", () => {
  const schemas = serverOpenAPI.components.schemas;
  const playback = schemas.ProfileServerPreferences.properties.playback.properties;
  const search = schemas.ProfileServerPreferences.properties.search.properties.recentQueries;
  const appearance = schemas.ProfileDeviceClassPreferences.properties.appearance.properties;
  const quality = schemas.QualityPreference.properties;
  assert.deepEqual(viewerPreferenceLimitsV1.playedThresholdPercent, {
    minimum: playback.playedThresholdPercent.minimum,
    maximum: playback.playedThresholdPercent.maximum
  });
  assert.deepEqual(viewerPreferenceLimitsV1.cardSizePercent, {
    minimum: appearance.cardSizePercent.minimum,
    maximum: appearance.cardSizePercent.maximum
  });
  assert.deepEqual(viewerPreferenceLimitsV1.audioBitrateKbps, {
    minimum: quality.maxAudioBitrateKbps.minimum,
    maximum: quality.maxAudioBitrateKbps.maximum
  });
  assert.deepEqual(viewerPreferenceLimitsV1.videoHeights, quality.maxVideoHeight.enum);
  assert.deepEqual(viewerPreferenceLimitsV1.searchHistory, {
    maximumItems: search.maxItems,
    maximumQueryRunes: search.items.maxLength
  });
});

test("media hierarchy truncation is published through OpenAPI and generated Core types", () => {
  const mediaItem = serverOpenAPI.components.schemas.MediaItem;
  assert.equal(mediaItem.properties.childrenTruncated.type, "boolean");
  assert.equal(mediaItem.properties.chapters.items.$ref, "#/components/schemas/Chapter");
  assert.deepEqual(serverOpenAPI.components.schemas.Chapter.required, ["id", "title", "startSeconds"]);
  const mediaCard = serverOpenAPI.components.schemas.MediaCard;
  assert.equal(mediaCard.properties.summary.type, "string");
  assert.ok(!mediaCard.required.includes("summary"));
  const serverTypes = fs.readFileSync(path.join(packageRoot, "src/openapi-types.ts"), "utf8");
  assert.match(serverTypes, /childrenTruncated\?: boolean;/);
  assert.match(serverTypes, /chapters\?: components\["schemas"\]\["Chapter"\]\[\];/);
  assert.match(serverTypes, /MediaCard: \{[\s\S]*?summary\?: string;/);
});

test("Live TV and DVR viewer and management audiences are explicit in the canonical contract", () => {
  const guideOperation = serverOpenAPI.paths["/live-tv/sources/{sourceId}/guide"].get;
  const channelOperation = serverOpenAPI.paths["/live-tv/sources/{sourceId}/channels"].get;
  const statusOperation = serverOpenAPI.paths["/dvr/status"].get;
  const adminStatusOperation = serverOpenAPI.paths["/admin/dvr/status"].get;
  const parameterNames = (operation) => operation.parameters.map((parameter) => parameter.name);

  assert.deepEqual(parameterNames(guideOperation), ["sourceId", "from", "hours", "limit", "cursor", "count", "query", "filter", "sort", "order", "group"]);
  assert.deepEqual(parameterNames(channelOperation), ["sourceId", "limit", "cursor", "count", "query", "favoritesOnly", "group"]);
  assert.equal(guideOperation["x-portico-permission"], "view-live-tv");
  assert.equal(channelOperation["x-portico-permission"], "view-live-tv");
  assert.equal(statusOperation["x-portico-permission"], "view-dvr");
  assert.equal(statusOperation["x-portico-audience"], "viewer");
  assert.equal(statusOperation["x-portico-client-core-method"], "dvrStatus");
  assert.equal(statusOperation.responses["200"].content["application/json"].schema.$ref, "#/components/schemas/DVRConsumerStatus");
  assert.equal(adminStatusOperation["x-portico-audience"], "management");
  assert.equal(adminStatusOperation["x-portico-client-core-method"], "adminDvrOperationalStatus");
  assert.equal(adminStatusOperation.responses["200"].content["application/json"].schema.$ref, "#/components/schemas/DVROperationalStatus");
  assert.equal(serverOpenAPI["x-portico-client-core-method-map"]["GET /live-tv"].clientCoreMethod, "liveTv");
  assert.equal(serverOpenAPI["x-portico-client-core-schema-map"].getDVRStatus, "DVRConsumerStatus");
  assert.ok(serverOpenAPI.components.schemas.LiveTVGuide.required.includes("channelGroups"));
  assert.ok(serverOpenAPI.components.schemas.LiveTVChannelListResponse.required.includes("groups"));
});

test("full search exposes one canonical sort and direction contract", () => {
  const request = serverOpenAPI.components.schemas.SearchRequest;
  const response = serverOpenAPI.components.schemas.SearchResponse;
  const productContract = serverOpenAPI.components.schemas.ProductContract;
  const semanticContract = serverOpenAPI.components.schemas.SearchContract;
  const operation = serverOpenAPI.paths["/search"].post;

  assert.deepEqual(request.properties.sort.enum, ["relevance", "title", "releaseYear", "dateAdded"]);
  assert.deepEqual(request.properties.direction.enum, ["asc", "desc"]);
  assert.deepEqual(response.properties.sort.enum, request.properties.sort.enum);
  assert.deepEqual(response.properties.direction.enum, request.properties.direction.enum);
  assert.ok(response.required.includes("sort"));
  assert.ok(response.required.includes("direction"));
  assert.match(operation.description, /Live TV is relevance-desc only/);
  assert.ok(productContract.required.includes("search"));
  assert.equal(productContract.properties.search.$ref, "#/components/schemas/SearchContract");
  assert.deepEqual(semanticContract.properties.groupOrder.items.enum, ["movies", "shows", "episodes", "people", "music", "audiobooks", "live-tv"]);
  assert.ok(request.properties.entityKinds.items.enum.includes("person"));
  assert.equal(request.properties.recordHistory.type, "boolean");
  assert.equal(semanticContract.properties.facetMode.enum[0], "none");
  assert.equal(semanticContract.properties.limits.$ref, "#/components/schemas/SearchLimits");
  assert.equal(semanticContract.properties.cursor.$ref, "#/components/schemas/SearchCursorSemantics");
});
