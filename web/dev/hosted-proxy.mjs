// Development only. Forwards a local origin to the production Portico Account
// service so the browser handoff flow (device code approved on the account
// site) works from 127.0.0.1. The account cookie is rewritten to this host.
// Never used in a build; the real deployment serves the web from the account
// origin and needs no proxy.
import http from 'node:http';
import https from 'node:https';
const UPSTREAM = process.env.HOSTED_UPSTREAM ?? 'https://web.getportico.tv';
const ALLOW = new Set((process.env.DEV_ORIGINS ?? 'http://127.0.0.1:19430,http://localhost:19430').split(','));
const PORT = Number(process.env.PORT ?? 19410);
const up = new URL(UPSTREAM);
http.createServer((req, res) => {
  const origin = req.headers.origin;
  const cors = origin && ALLOW.has(origin) ? {'Access-Control-Allow-Origin': origin, 'Access-Control-Allow-Credentials': 'true', 'Access-Control-Allow-Headers': 'Authorization, Content-Type', 'Access-Control-Allow-Methods': 'GET, POST, PUT, PATCH, DELETE, OPTIONS', 'Vary': 'Origin'} : {};
  if (req.method === 'OPTIONS') { res.writeHead(204, cors); res.end(); return; }
  const headers = {...req.headers, host: up.host, origin: 'https://web.getportico.tv', referer: 'https://web.getportico.tv/'};
  delete headers['accept-encoding'];
  const out = https.request({hostname: up.hostname, port: 443, path: req.url, method: req.method, headers}, r => {
    const h = {...r.headers, ...cors};
    delete h['access-control-allow-origin']; delete h['access-control-allow-credentials'];
    Object.assign(h, cors);
    if (h['set-cookie']) h['set-cookie'] = [].concat(h['set-cookie']).map(c => c.replace(/;\s*Domain=[^;]*/i, '').replace(/;\s*Secure/i, '').replace(/;\s*SameSite=[^;]*/i, '; SameSite=Lax'));
    res.writeHead(r.statusCode ?? 502, h);
    r.pipe(res);
  });
  out.on('error', e => { res.writeHead(502, cors); res.end(JSON.stringify({error: {code: 'proxy_failed', message: e.message}})); });
  req.pipe(out);
}).listen(PORT, '127.0.0.1', () => console.log(`hosted proxy ${PORT} -> ${UPSTREAM}`));
