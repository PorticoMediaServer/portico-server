import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { readFile } from 'node:fs/promises';

type Stream = { id: string; kind: string; codec?: string; language?: string; displayTitle?: string };
type Playback = { sessionId: string; sourceUrl: string; streamFormat: string; directPlay: boolean; generation: number; playbackRevision: number; mediaGrant: { token: string; expiresAt: string }; decision: { mode: string; protocol: string; generation: number }; audioStreams: Stream[]; subtitleStreams: Stream[]; selectedAudioStreamId?: string; selectedSubtitleStreamId?: string; selectedSubtitleMode?: string };
type FixtureManifest = { schema: string; user: { login: string; password: string }; media: Record<string, string>; control: { path: string; secret: string } };

function hlsSourcePattern(browserName: string): RegExp {
	// WEB-018: Chromium uses managed MediaSource/HLS and therefore exposes a
	// blob URL; WebKit exercises the native HLS source branch. The latter is
	// Playwright WebKit evidence, not a claim about physical Safari devices.
	if (browserName === 'webkit') return /\.m3u8(?:[?#]|$)/;
	if (browserName === 'chromium') return /^blob:/;
	throw new Error(`Playback conformance has no HLS source assertion for ${browserName}.`);
}

async function expectHlsSource(page: Page, browserName: string) {
	const video = page.locator('video').first();
	await expect.poll(() => video.evaluate((element: HTMLVideoElement) => element.currentSrc || element.src), {
		message: `expected ${browserName} playback to expose its configured HLS source adapter`,
	}).toMatch(hlsSourcePattern(browserName));
}

const manifestPath = process.env.PORTICO_PLAYBACK_MANIFEST;
if (!manifestPath) throw new Error('PORTICO_PLAYBACK_MANIFEST is required');
const manifest = JSON.parse(await readFile(manifestPath, 'utf8')) as FixtureManifest;

async function json(request: APIRequestContext, method: 'get' | 'post' | 'delete', path: string, data?: unknown) {
	const response = await request[method](path, data === undefined ? undefined : { data, headers: { 'X-Portico-CSRF': '1' } });
	return { response, body: await response.json().catch(() => undefined) };
}

async function start(request: APIRequestContext, mediaId: string, clientProfile: unknown) {
	const result = await json(request, 'post', '/api/playback-sessions', { mediaId, clientInstanceId: `playwright-fixture-${mediaId}`, clientProfile, skipPreroll: true });
	expect(result.response.ok(), JSON.stringify(result.body)).toBeTruthy();
	return result.body as Playback;
}

async function watchThroughPlayer(page: Page, mediaId: string) {
	const planResponse = page.waitForResponse((response) => {
		if (!response.url().endsWith('/api/playback-sessions') || response.request().method() !== 'POST') return false;
		try { return (response.request().postDataJSON() as { mediaId?: string }).mediaId === mediaId; } catch { return false; }
	});
	await page.goto(`/watch/${mediaId}`);
	const response = await planResponse;
	const playback = await response.json() as Playback;
	expect(response.ok(), JSON.stringify(playback)).toBeTruthy();
	return playback;
}

test('real server playback, grants, truthful failures, and one-shot recovery', async ({ page, browserName }) => {
	await page.goto('/');
	await page.getByRole('textbox', { name: 'Username or email' }).fill(manifest.user.login);
	await page.getByRole('textbox', { name: 'Password', exact: true }).fill(manifest.user.password);
	await page.getByRole('button', { name: 'Sign in with This Server' }).click();
	await expect(page.getByText('Playback Conformance').first()).toBeVisible();
	const request = page.context().request;
	const uiPlanResponse = page.waitForResponse((response) => response.url().endsWith('/api/playback-sessions') && response.request().method() === 'POST');
	await page.goto(`/watch/${manifest.media.direct}`);
	const uiPlan = await uiPlanResponse;
	const uiPlayback = await uiPlan.json() as Playback;
	expect(uiPlan.ok(), JSON.stringify(uiPlayback)).toBeTruthy();
	const browserProfile = (uiPlan.request().postDataJSON() as { clientProfile: unknown }).clientProfile;
	expect(uiPlayback.sourceUrl).toContain(`/api/media/${manifest.media.direct}/stream`);
	const video = page.locator('video').first();
	await expect(video).toBeVisible();
	await expect.poll(() => video.evaluate((element: HTMLVideoElement) => element.currentSrc || element.src)).toContain(`/api/media/${manifest.media.direct}/stream`);

	const direct = await start(request, manifest.media.direct, browserProfile);
	expect(direct.directPlay).toBe(true);
	expect(direct.sourceUrl).toContain(`/api/media/${manifest.media.direct}/stream`);
	const directBytes = await request.get(direct.sourceUrl, { headers: { Range: 'bytes=0-1023', Authorization: `PorticoMedia ${direct.mediaGrant.token}` } });
	expect([200, 206]).toContain(directBytes.status());
	expect((await directBytes.body()).byteLength).toBeGreaterThan(0);

	const incompatible = await start(request, manifest.media.remux, browserProfile);
	expect(incompatible.sourceUrl).toContain('/hls/');
	const hls = await request.get(incompatible.sourceUrl, { headers: { Authorization: `PorticoMedia ${incompatible.mediaGrant.token}` } });
	expect(hls.ok(), await hls.text()).toBeTruthy();
	expect(await hls.text()).toContain('#EXTM3U');

	// Exercise the real PlayerSurface and managed HLS adapter, rather than
	// treating successful API planning as browser playback evidence.
	const playerManifest = page.waitForResponse((response) => response.url().includes(`/api/media/${manifest.media.remux}/hls/`) && new URL(response.url()).pathname.endsWith('.m3u8'));
	const playerRemux = await watchThroughPlayer(page, manifest.media.remux);
	const playerManifestResponse = await playerManifest;
	expect(playerManifestResponse.ok()).toBeTruthy();
	await expectHlsSource(page, browserName);
	await video.evaluate((element: HTMLVideoElement) => element.play()).catch(() => undefined);
	await expect.poll(() => video.evaluate((element: HTMLVideoElement) => element.readyState)).toBeGreaterThanOrEqual(2);
	const sourceBeforeQueue = await video.evaluate((element: HTMLVideoElement) => element.currentSrc);
	const positionBeforeQueue = await video.evaluate((element: HTMLVideoElement) => element.currentTime);
	await page.getByRole('button', { name: 'Queue' }).click();
	const repeat = page.getByRole('button', { name: /Repeat (off|all|one)/ });
	if (await repeat.isEnabled()) {
		await repeat.click();
		await expect.poll(() => video.evaluate((element: HTMLVideoElement) => element.currentSrc)).toBe(sourceBeforeQueue);
		await expect.poll(() => video.evaluate((element: HTMLVideoElement) => element.currentTime)).toBeGreaterThanOrEqual(Math.max(0, positionBeforeQueue - 0.15));
	}
	await page.keyboard.press('Escape');
	const position = page.getByRole('slider', { name: 'Playback position' });
	if (await position.isVisible()) {
		await position.fill('1');
		await expect.poll(() => video.evaluate((element: HTMLVideoElement) => element.currentTime)).toBeGreaterThanOrEqual(0.85);
	}

	const codecTranscode = await start(request, manifest.media.transcode, browserProfile);
	expect(codecTranscode.directPlay).toBe(false);
	expect(codecTranscode.decision.mode).toContain('transcode');
	expect(codecTranscode.sourceUrl).toContain('/hls/');
	const transcodeManifest = await request.get(codecTranscode.sourceUrl, { headers: { Authorization: `PorticoMedia ${codecTranscode.mediaGrant.token}` } });
	expect(transcodeManifest.ok(), await transcodeManifest.text()).toBeTruthy();
	expect(await transcodeManifest.text()).toContain('#EXTM3U');
	const playerTranscodeManifest = page.waitForResponse((response) => response.url().includes(`/api/media/${manifest.media.transcode}/hls/`) && new URL(response.url()).pathname.endsWith('.m3u8'));
	await watchThroughPlayer(page, manifest.media.transcode);
	expect((await playerTranscodeManifest).ok()).toBeTruthy();
	await expectHlsSource(page, browserName);

	const multiTrack = await start(request, manifest.media.multiTrack, browserProfile);
	expect(multiTrack.audioStreams).toHaveLength(2);
	expect(multiTrack.subtitleStreams.filter((stream) => stream.id !== 'sub_none')).toHaveLength(1);
	const french = multiTrack.audioStreams.find((stream) => stream.language === 'fra');
	const englishSubtitle = multiTrack.subtitleStreams.find((stream) => stream.language === 'eng');
	expect(french).toBeDefined();
	expect(englishSubtitle).toBeDefined();
	const renegotiated = await json(request, 'post', `/api/playback-sessions/${multiTrack.sessionId}/renegotiate`, {
		requestId: 'playwright-multitrack-selection-v1',
		expectedRevision: multiTrack.playbackRevision,
		audioStreamId: french!.id,
	});
	expect(renegotiated.response.ok(), JSON.stringify(renegotiated.body)).toBeTruthy();
	const selected = renegotiated.body as Playback;
	expect(selected.playbackRevision).toBe(multiTrack.playbackRevision + 1);
	expect(selected.generation).toBe(multiTrack.generation);
	expect(selected.decision.generation).toBe(multiTrack.decision.generation + 1);
	expect(selected.selectedAudioStreamId).toBe(french!.id);
	expect(selected.mediaGrant.token).not.toBe(multiTrack.mediaGrant.token);
	const subtitleSelection = await json(request, 'post', `/api/playback-sessions/${multiTrack.sessionId}/renegotiate`, {
		requestId: 'playwright-multitrack-subtitle-v1',
		expectedRevision: selected.playbackRevision,
		subtitleStreamId: englishSubtitle!.id,
		subtitleMode: 'text',
	});
	if (browserName === 'webkit') {
		// Playwright WebKit currently reports Safari 26.4, outside Portico's
		// reviewed Safari 17–19 compatibility band. The server must therefore
		// reject text-subtitle renegotiation instead of adopting an unreviewed
		// HLS capability claim. This is fail-closed engine evidence, not a
		// physical Safari-device compatibility claim.
		expect(subtitleSelection.response.status()).toBe(400);
		expect(subtitleSelection.body).toMatchObject({
			code: 'renegotiation_failed',
			detail: 'the title cannot be delivered within the selected playback policy',
		});
	} else {
		expect(subtitleSelection.response.ok(), JSON.stringify(subtitleSelection.body)).toBeTruthy();
		const subtitled = subtitleSelection.body as Playback;
		expect(subtitled.generation).toBe(selected.generation);
		expect(subtitled.decision.generation).toBe(selected.decision.generation + 1);
		expect(subtitled.selectedSubtitleStreamId).toBe(englishSubtitle!.id);
		expect(subtitled.selectedSubtitleMode).toBe('text');
	}

	// Select the alternate audio stream through the real player controls. The
	// alternate rendition is plan-described but its grant remains bound to the
	// active selection. The player must renegotiate and atomically adopt the
	// returned revision, generation, resources, and grant before rebuilding HLS.
	const playerMultiTrack = await watchThroughPlayer(page, manifest.media.multiTrack);
	const effectivePlayerAudioId = playerMultiTrack.selectedAudioStreamId || playerMultiTrack.audioStreams[0]?.id;
	const playerAlternateAudio = playerMultiTrack.audioStreams.find((stream) => stream.id !== effectivePlayerAudioId);
	expect(playerAlternateAudio?.displayTitle).toBeTruthy();
	await page.getByRole('button', { name: 'Playback settings' }).click();
	await page.getByRole('combobox', { name: 'Audio' }).click();
	const sourceBeforeAudioChange = await video.evaluate((element: HTMLVideoElement) => element.currentSrc);
	const playerAudioSelection = page.waitForResponse((response) => response.url().endsWith(`/api/playback-sessions/${playerMultiTrack.sessionId}/renegotiate`) && response.request().method() === 'POST');
	const playerAlternateManifest = page.waitForResponse((response) => response.url().includes(`/api/media/${manifest.media.multiTrack}/hls/`) && new URL(response.url()).pathname.endsWith('.m3u8'));
	await page.getByRole('listbox', { name: 'Audio' }).getByRole('option', { name: new RegExp(playerAlternateAudio!.displayTitle!, 'i') }).click();
	const playerAudioResponse = await playerAudioSelection;
	const playerAudioPlayback = await playerAudioResponse.json() as Playback;
	expect(playerAudioResponse.ok(), JSON.stringify(playerAudioPlayback)).toBeTruthy();
	expect(playerAudioPlayback.selectedAudioStreamId).toBe(playerAlternateAudio!.id);
	expect(playerAudioPlayback.playbackRevision).toBe(playerMultiTrack.playbackRevision + 1);
	expect(playerAudioPlayback.decision.generation).toBe(playerMultiTrack.decision.generation + 1);
	expect((await playerAlternateManifest).ok()).toBeTruthy();
	if (browserName === 'chromium') {
		// The managed HLS adapter exposes a fresh blob URL after renegotiation.
		await expect.poll(() => video.evaluate((element: HTMLVideoElement) => element.currentSrc)).not.toBe(sourceBeforeAudioChange);
	} else {
		// Native HLS keeps the manifest URL stable; the successful renegotiation
		// response and fresh manifest request are the authoritative selection
		// evidence for this engine.
		await expect.poll(() => video.evaluate((element: HTMLVideoElement) => element.currentSrc)).toBe(sourceBeforeAudioChange);
	}
	await expectHlsSource(page, browserName);

	const music = await start(request, manifest.media.music, browserProfile);
	expect(music.audioStreams).toHaveLength(1);
	expect(music.audioStreams[0]?.codec).toBe('mp3');
	const musicBytes = await request.get(music.sourceUrl, { headers: { Range: 'bytes=0-511', Authorization: `PorticoMedia ${music.mediaGrant.token}` } });
	expect([200, 206]).toContain(musicBytes.status());
	expect((await musicBytes.body()).byteLength).toBeGreaterThan(0);
	await watchThroughPlayer(page, manifest.media.music);
	await expect.poll(() => video.evaluate((element: HTMLVideoElement) => element.currentSrc || element.src)).toContain(`/api/media/${manifest.media.music}/stream`);

	const missing = await json(request, 'post', '/api/playback-sessions', { mediaId: manifest.media.missing, clientInstanceId: `playwright-fixture-${manifest.media.missing}`, clientProfile: browserProfile, skipPreroll: true });
	expect(missing.response.ok()).toBeFalsy();
	expect([404, 409, 410, 422]).toContain(missing.response.status());

	const renewed = await json(request, 'post', `/api/playback-sessions/${direct.sessionId}/media-grant`, {});
	expect(renewed.response.ok()).toBeTruthy();
	expect(renewed.body.token).not.toBe(direct.mediaGrant.token);
	const revoked = await request.get(direct.sourceUrl, { headers: { Authorization: `PorticoMedia ${direct.mediaGrant.token}` } });
	expect(revoked.status()).toBe(401);

	const delayMS = 250;
	const armDelay = await request.post(manifest.control.path, { data: { path: direct.sourceUrl, delayMs: delayMS }, headers: { Authorization: `Bearer ${manifest.control.secret}` } });
	expect(armDelay.ok()).toBeTruthy();
	const delayedAt = Date.now();
	const delayed = await request.get(direct.sourceUrl, { headers: { Range: 'bytes=0-255', Authorization: `PorticoMedia ${renewed.body.token}` } });
	expect([200, 206]).toContain(delayed.status());
	expect(Date.now() - delayedAt).toBeGreaterThanOrEqual(delayMS - 25);

	// A one-shot manifest failure must surface through the actual player and a
	// manual Retry must rebuild/restart managed HLS with the fresh grant.
	await watchThroughPlayer(page, manifest.media.direct);
	const manifestPathname = new URL(playerRemux.sourceUrl, page.url()).pathname;
	const armPlayerFailure = await request.post(manifest.control.path, { data: { path: manifestPathname, status: 410 }, headers: { Authorization: `Bearer ${manifest.control.secret}` } });
	expect(armPlayerFailure.ok()).toBeTruthy();
	const failedManifest = page.waitForResponse((response) => new URL(response.url()).pathname === manifestPathname && response.status() === 410);
	await watchThroughPlayer(page, manifest.media.remux);
	await failedManifest;
	const retry = page.getByRole('button', { name: 'Try again' }).first();
	await expect(retry).toBeVisible();
	const recoveredManifest = page.waitForResponse((response) => new URL(response.url()).pathname === manifestPathname && response.ok());
	await retry.click();
	await recoveredManifest;
	await expectHlsSource(page, browserName);

	const arm = await request.post(manifest.control.path, { data: { path: direct.sourceUrl, status: 410 }, headers: { Authorization: `Bearer ${manifest.control.secret}` } });
	expect(arm.ok()).toBeTruthy();
	expect((await request.get(direct.sourceUrl, { headers: { Authorization: `PorticoMedia ${renewed.body.token}` } })).status()).toBe(410);
	const recovered = await start(request, manifest.media.direct, browserProfile);
	expect([200, 206]).toContain((await request.get(recovered.sourceUrl, { headers: { Range: 'bytes=0-255', Authorization: `PorticoMedia ${recovered.mediaGrant.token}` } })).status());
});
