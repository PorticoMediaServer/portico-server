// Taps the emulator's touch screen through its gRPC control service (what Android Studio uses).
// usage: node emu-touch.mjs <avd-name> <x> <y>   (display pixels)
import http2 from 'node:http2';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
const [avd, x, y] = process.argv.slice(2);
const dir = path.join(os.homedir(), 'Library/Caches/TemporaryItems/avd/running');
let port, token;
for (const f of fs.readdirSync(dir).filter(f => f.endsWith('.ini'))) {
  const ini = Object.fromEntries(fs.readFileSync(path.join(dir, f), 'utf8').split('\n').filter(l => l.includes('=')).map(l => [l.slice(0, l.indexOf('=')), l.slice(l.indexOf('=') + 1).trim()]));
  if (ini['avd.name'] === avd) { port = ini['grpc.port']; token = ini['grpc.token']; }
}
if (!port) throw new Error('no running emulator named ' + avd);
const varint = n => { const out = []; let v = n >>> 0; while (v > 127) { out.push((v & 127) | 128); v >>>= 7; } out.push(v); return out; };
const touch = (px, py, pressure) => { const t = [0x08, ...varint(px), 0x10, ...varint(py), 0x18, 0, 0x20, ...varint(pressure), 0x28, ...varint(pressure ? 5 : 0)]; return Buffer.from([0x0a, ...varint(t.length), ...t]); };
const call = (client, message) => new Promise((resolve, reject) => {
  const request = client.request({':method': 'POST', ':path': '/android.emulation.control.EmulatorController/sendTouch', 'content-type': 'application/grpc', te: 'trailers', ...(token ? {authorization: 'Bearer ' + token} : {})});
  const frame = Buffer.alloc(5 + message.length); frame.writeUInt32BE(message.length, 1); message.copy(frame, 5);
  let status = '';
  request.on('trailers', h => { status = h['grpc-status'] + ' ' + (h['grpc-message'] ?? ''); });
  request.on('response', h => { if (h['grpc-status']) status = h['grpc-status'] + ' ' + (h['grpc-message'] ?? ''); });
  request.on('data', () => {}); request.on('end', () => resolve(status)); request.on('error', reject);
  request.end(frame);
});
const client = http2.connect('http://127.0.0.1:' + port);
client.on('error', e => { console.error(String(e)); process.exit(1); });
const down = await call(client, touch(+x, +y, 1));
await new Promise(r => setTimeout(r, 120));
const up = await call(client, touch(+x, +y, 0));
console.log('down:', down.trim(), 'up:', up.trim());
client.close();
