import assert from "node:assert/strict";
import test from "node:test";

import {
  PORTICO_NAVIGATION_CONTRACT_REVISION,
  consumePorticoPendingDestinationIntent,
  createPorticoNavigationRestoration,
  createPorticoPendingDestinationIntent,
  normalizePorticoDestination,
  parsePorticoLink,
  porticoDestinationCapabilityRevision,
  porticoDestinationIdentity,
  porticoDestinationIsAvailable,
  porticoNavigationPolicy,
  porticoNavigationRestorationFence,
  porticoProductContractRevision,
  restorePorticoNavigation,
  serializePorticoLink
} from "../dist/navigationContract.js";

const destinations = [
  {destination: "home"},
  {destination: "library", libraryId: "library 1", pivot: "episodes"},
  {destination: "channels", tab: "library-channels"},
  {destination: "saved", tab: "watchlist"},
  {destination: "downloads"},
  {destination: "search", query: "quiet place"},
  {destination: "settings", section: "playback"},
  {destination: "person", personId: "person/one"},
  {destination: "media-detail", mediaId: "episode one", seasonId: "season-8", episodeId: "episode-15"},
  {destination: "media-detail", mediaId: "channel-1", mediaKind: "live-channel"},
  {destination: "notifications"},
  {destination: "watch-with-friends", groupId: "group-1"},
  {destination: "player", mediaId: "movie-1"},
  {destination: "player", mediaId: "channel-1", context: "live"},
  {destination: "player", mediaId: "movie-1", context: "watch-with-friends", watchWithFriendsGroupId: "group-1"},
  {destination: "player", mediaId: "movie-1", context: "offline", localDownloadId: "download-1"}
];

test("every canonical destination round-trips through custom-scheme and HTTPS links", () => {
  for (const destination of destinations) {
    assert.deepEqual(parsePorticoLink(serializePorticoLink(destination)), destination);
    assert.deepEqual(parsePorticoLink(serializePorticoLink(destination, "https://web.getportico.tv/app/")), destination);
  }
});

test("canonical media and play links remain strict inputs", () => {
  assert.deepEqual(parsePorticoLink("portico://media/movie-1"), {destination: "media-detail", mediaId: "movie-1"});
  assert.deepEqual(parsePorticoLink("https://web.getportico.tv/play/movie-1"), {destination: "player", mediaId: "movie-1"});
  assert.equal(parsePorticoLink("portico://account"), undefined);
  assert.equal(parsePorticoLink("https://untrusted.example/media/movie-1", {allowedWebHosts: ["web.getportico.tv"]}), undefined);
  assert.equal(parsePorticoLink("https://web.getportico.tv:8443/media/movie-1", {allowedWebHosts: ["web.getportico.tv"]}), undefined);
});

test("malformed, sensitive, and operational route parameters fail closed", () => {
  for (const destination of [
    {destination: "player", mediaId: "movie", sourceURL: "file:///private/movie.mkv"},
    {destination: "player", mediaId: "movie", token: "secret"},
    {destination: "player", mediaId: "movie", context: "offline"},
    {destination: "player", mediaId: "movie", localDownloadId: "download"},
    {destination: "player", mediaId: "movie", context: "watch-with-friends"},
    {destination: "media-detail", mediaId: ""},
    {destination: "home", callback() {}},
    {destination: "unknown"}
  ]) assert.equal(normalizePorticoDestination(destination), undefined);
  assert.equal(parsePorticoLink("javascript:alert(1)"), undefined);
  assert.equal(parsePorticoLink("not a link"), undefined);
  assert.equal(parsePorticoLink("portico://play/movie?context=offline"), undefined);
  assert.equal(parsePorticoLink("portico://play/movie?sourceURL=https%3A%2F%2Fprivate.example"), undefined);
  assert.equal(parsePorticoLink("portico://media/movie/extra"), undefined);
  assert.throws(() => serializePorticoLink({destination: "player", mediaId: "movie", sourceURL: "https://private"}), /invalid/);
});

test("identity and history policy deduplicate replace-style selections without collapsing distinct content", () => {
  assert.equal(porticoDestinationIdentity({destination: "library", libraryId: "tv", pivot: "shows"}), porticoDestinationIdentity({destination: "library", libraryId: "tv", pivot: "episodes"}));
  assert.equal(porticoDestinationIdentity({destination: "search", query: "first"}), porticoDestinationIdentity({destination: "search", query: "second"}));
  assert.notEqual(porticoDestinationIdentity({destination: "media-detail", mediaId: "one"}), porticoDestinationIdentity({destination: "media-detail", mediaId: "two"}));
  assert.deepEqual(porticoNavigationPolicy({destination: "home"}), {history: "root", identity: "home"});
  assert.deepEqual(porticoNavigationPolicy({destination: "settings", section: "privacy"}), {history: "focus-or-replace", identity: "settings"});
  assert.deepEqual(porticoNavigationPolicy({destination: "media-detail", mediaId: "one"}), {history: "push", identity: "media-detail:one"});
});

test("platform and capability requirements prevent unsupported destinations", () => {
  assert.equal(porticoDestinationIsAvailable({destination: "downloads"}, "handheld"), true);
  assert.equal(porticoDestinationIsAvailable({destination: "downloads"}, "television"), false);
  assert.equal(porticoDestinationIsAvailable({destination: "downloads"}, "web"), false);
  assert.equal(porticoDestinationIsAvailable({destination: "player", mediaId: "downloaded", context: "offline", localDownloadId: "download-one"}, "television", {downloads: true}), false);
  assert.equal(porticoDestinationIsAvailable({destination: "player", mediaId: "downloaded", context: "offline", localDownloadId: "download-one"}, "handheld", {downloads: false}), false);
  assert.equal(porticoDestinationIsAvailable({destination: "player", mediaId: "downloaded", context: "offline", localDownloadId: "download-one"}, "handheld", {downloads: true}), true);
  assert.equal(porticoDestinationIsAvailable({destination: "channels"}, "handheld", {liveTV: false}), false);
  assert.equal(porticoDestinationIsAvailable({destination: "notifications"}, "handheld", {notifications: false}), false);
  assert.equal(porticoDestinationIsAvailable({destination: "watch-with-friends"}, "television", {watchWithFriends: false}), false);
});

test("route-affecting capability revisions are deterministic and platform fenced", () => {
  const capabilities = {downloads: true, liveTV: false, notifications: true, watchWithFriends: false};
  assert.equal(porticoDestinationCapabilityRevision("handheld", capabilities), porticoDestinationCapabilityRevision("handheld", {...capabilities}));
  assert.notEqual(porticoDestinationCapabilityRevision("handheld", capabilities), porticoDestinationCapabilityRevision("television", capabilities));
  assert.notEqual(porticoDestinationCapabilityRevision("handheld", capabilities), porticoDestinationCapabilityRevision("handheld", {...capabilities, liveTV: true}));
});

test("product contract revisions are deterministic, exact, and limited to navigation-relevant fields", () => {
  const contract = {
    apiVersion: "v1",
    actionRevision: "actions-v1",
    language: {revision: "v1", defaultLocale: "en", supportedLocales: ["en"], endpointTemplate: "/api/product-language/{locale}", iconFamily: "lucide"},
    search: {revision: "v1", groups: [{id: "media"}], maximumResults: 20},
    applicationEvents: {revision: "v1", eventTypes: ["library.changed"]},
  };
  const reordered = {
    applicationEvents: {eventTypes: ["library.changed"], revision: "v1"},
    search: {maximumResults: 20, groups: [{id: "media"}], revision: "v1"},
    language: {iconFamily: "lucide", endpointTemplate: "/api/product-language/{locale}", supportedLocales: ["en"], defaultLocale: "en", revision: "v1"},
    actionRevision: "actions-v1",
    apiVersion: "v1",
  };
  assert.equal(porticoProductContractRevision(contract), porticoProductContractRevision(reordered));
  assert.notEqual(porticoProductContractRevision(contract), porticoProductContractRevision({...contract, actionRevision: "actions-v2"}));
  assert.notEqual(porticoProductContractRevision(contract), porticoProductContractRevision({...contract, search: {...contract.search, revision: "v2"}}));
  const revision = porticoProductContractRevision(contract);
  assert.ok(revision.length <= 128);
  assert.equal(porticoNavigationRestorationFence({
    ...viewer,
    productContractRevision: revision,
    platform: "handheld",
    capabilityRevision: "capabilities-v1",
  }).productContractRevision, revision);
});

const viewer = {authority: "hosted", accountId: "account", serverId: "server", profileId: "profile-a", authorizationRevision: "policy-1"};
const fence = porticoNavigationRestorationFence({
  ...viewer,
  productContractRevision: "product-v1",
  platform: "handheld",
  capabilityRevision: "capabilities-v1"
});

test("bounded restoration accepts only an exact viewer, contract, platform, and capability fence", () => {
  const now = new Date("2026-07-21T12:00:00Z");
  const state = createPorticoNavigationRestoration(fence, {destination: "library", libraryId: "tv", pivot: "shows"}, now);
  assert.equal(state.fence.routeContractRevision, PORTICO_NAVIGATION_CONTRACT_REVISION);
  assert.deepEqual(restorePorticoNavigation(state, fence, {now}), {destination: "library", libraryId: "tv", pivot: "shows"});
  for (const change of [
    {profileId: "profile-b"},
    {serverId: "server-b"},
    {accountId: "account-b"},
    {authorizationRevision: "policy-2"},
    {productContractRevision: "product-v2"},
    {capabilityRevision: "capabilities-v2"},
    {platform: "television"}
  ]) assert.equal(restorePorticoNavigation(state, {...fence, ...change}, {now}), undefined);
  assert.equal(restorePorticoNavigation({...state, destination: {destination: "player", mediaId: "movie"}}, fence, {now}), undefined);
  assert.equal(restorePorticoNavigation(state, fence, {now: new Date("2027-01-01T00:00:00Z"), maxAgeMs: 1000}), undefined);
});

test("downloads restoration additionally respects platform and current capability", () => {
  const now = new Date("2026-07-21T12:00:00Z");
  const state = createPorticoNavigationRestoration(fence, {destination: "downloads"}, now);
  assert.deepEqual(restorePorticoNavigation(state, fence, {now}), {destination: "downloads"});
  assert.equal(restorePorticoNavigation(state, fence, {now, capabilities: {downloads: false}}), undefined);
});

test("pending destination intents expire, remain viewer-bound, and require final authorization", () => {
  const now = new Date("2026-07-21T12:00:00Z");
  const intent = createPorticoPendingDestinationIntent(
    {destination: "media-detail", mediaId: "private-media"},
    {now, ttlMs: 60_000, expectedIdentity: viewer}
  );
  assert.deepEqual(consumePorticoPendingDestinationIntent(intent, viewer, "handheld", {now, authorize: () => true}), {destination: "media-detail", mediaId: "private-media"});
  assert.equal(consumePorticoPendingDestinationIntent(intent, {...viewer, profileId: "profile-b"}, "handheld", {now, authorize: () => true}), undefined);
  assert.equal(consumePorticoPendingDestinationIntent(intent, viewer, "handheld", {now, authorize: () => false}), undefined);
  assert.equal(consumePorticoPendingDestinationIntent(intent, viewer, "handheld", {now: new Date(now.getTime() + 60_001), authorize: () => true}), undefined);
});
