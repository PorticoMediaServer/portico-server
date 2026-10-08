import {defineConfig, mergeConfig} from 'vite';
import {writeFileSync, mkdirSync, statSync} from 'node:fs';
import base from './vite.config';

/** Dev-only: serves audio-conformance.html and accepts its report at POST /__audio-report
 * (written to $AUDIO_REPORT_DIR/<browser>.json). Not used by the app build. */
export default mergeConfig(base, defineConfig({
  plugins: [{
    name: 'audio-report',
    apply: 'serve',
    configureServer(server) {
      // The page reloads itself when this token changes (touch $AUDIO_REPORT_DIR/run), so one
      // browser tab can be reused for every run.
      server.middlewares.use('/__audio-run', (_req, res) => {
        let token = '0';
        try { token = String(statSync(`${process.env.AUDIO_REPORT_DIR}/run`).mtimeMs); } catch { /* none yet */ }
        res.setHeader('Content-Type', 'text/plain'); res.setHeader('Cache-Control', 'no-store'); res.end(token);
      });
      server.middlewares.use('/__audio-report', (req, res) => {
        let body = '';
        req.on('data', c => { body += c; });
        req.on('end', () => {
          const dir = process.env.AUDIO_REPORT_DIR;
          if (dir) {
            mkdirSync(dir, {recursive: true});
            const name = String(JSON.parse(body).browser ?? 'unknown').replace(/[^a-z0-9-]/gi, '_');
            writeFileSync(`${dir}/${name}.json`, body);
          }
          res.statusCode = 204; res.end();
        });
      });
    },
  }],
}));
