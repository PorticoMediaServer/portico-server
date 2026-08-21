import { spawn } from 'node:child_process';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

const webDir = resolve(import.meta.dirname, '..');
const serverDir = resolve(webDir, '..');
const state = await mkdtemp(join(tmpdir(), 'portico-playback-e2e-'));
const manifestPath = join(state, 'manifest.json');
const children = new Set();
let stopping = false;

function start(command, args, options = {}) {
	const child = spawn(command, args, {
		stdio: ['ignore', 'inherit', 'inherit'],
		detached: process.platform !== 'win32',
		...options,
	});
	children.add(child);
	child.once('exit', () => children.delete(child));
	return child;
}

function terminate(child, signal) {
	try {
		if (process.platform === 'win32') child.kill(signal);
		else process.kill(-child.pid, signal);
	} catch {
		try { child.kill(signal); } catch { /* already exited */ }
	}
}

async function waitFor(check, label, timeoutMs = 120_000) {
	const deadline = Date.now() + timeoutMs;
	let last;
	while (Date.now() < deadline) {
		try { return await check(); } catch (error) { last = error; }
		await new Promise((resolveDelay) => setTimeout(resolveDelay, 150));
	}
	throw new Error(`Timed out waiting for ${label}: ${last instanceof Error ? last.message : last}`);
}

async function cleanup() {
	if (stopping) return;
	stopping = true;
	const active = [...children];
	const exits = active.map((child) => new Promise((resolveExit) => {
		if (child.exitCode !== null || child.signalCode !== null) {
			resolveExit();
			return;
		}
		let timer;
		const finished = () => { clearTimeout(timer); resolveExit(); };
		child.once('exit', finished);
		timer = setTimeout(() => { terminate(child, 'SIGKILL'); resolveExit(); }, 5_000);
	}));
	for (const child of active) terminate(child, 'SIGTERM');
	await Promise.all(exits);
	await rm(state, { recursive: true, force: true });
}

for (const signal of ['SIGINT', 'SIGTERM']) process.once(signal, () => { void cleanup().finally(() => process.exit(128)); });

let exitCode = 1;
try {
	start('go', ['run', './cmd/portico-playback-fixture', '--manifest', manifestPath, '--app-data', join(state, 'server')], { cwd: serverDir });
	const manifest = await waitFor(async () => JSON.parse(await readFile(manifestPath, 'utf8')), 'private fixture manifest');
	const webURL = 'http://127.0.0.1:19125';
	start(process.platform === 'win32' ? 'npm.cmd' : 'npm', ['run', 'dev', '--', '--port', '19125', '--strictPort'], {
		cwd: webDir,
		env: { ...process.env, VITE_PORTICO_RUNTIME_MODE: 'bundled', PORTICO_PLAYBACK_FIXTURE_URL: manifest.baseUrl },
	});
	await waitFor(async () => { const response = await fetch(webURL); if (!response.ok) throw new Error(`HTTP ${response.status}`); return true; }, 'Vite');
	const playwright = start(process.platform === 'win32' ? 'npx.cmd' : 'npx', ['playwright', 'test', '--config', 'playwright.playback.config.ts'], {
		cwd: webDir,
		env: { ...process.env, PORTICO_PLAYBACK_WEB_URL: webURL, PORTICO_PLAYBACK_MANIFEST: manifestPath },
	});
	exitCode = await new Promise((resolveExit, reject) => {
		playwright.once('error', reject);
		playwright.once('exit', (code, signal) => resolveExit(code ?? (signal ? 1 : 0)));
	});
} finally {
	await cleanup();
}
process.exitCode = exitCode;
