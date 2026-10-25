// Same-size, same-state capture of the Library screen's drawn states and their
// desktop-browse.html frames. Run on the external runner with --browser and the
// exact design secondary source. It mounts the genuine DEV specimen and the
// frame's own markup/CSS; it commits no frame, artwork or private input.
// Diagnostic only: a plain side-by-side pair for the lane report, never a
// fidelity verdict and never a second comparator.
import {createRequire} from 'node:module';
import {readFile, writeFile, mkdir} from 'node:fs/promises';
import {existsSync} from 'node:fs';
import {createServer} from 'vite';
import path from 'node:path';

const output = process.env.PORTICO_RUNNER_OUTPUT, design = process.env.PORTICO_RUNNER_SECONDARY_SOURCE;
const executablePath = process.env.PORTICO_CHROMIUM ?? ['/usr/bin/chromium', '/snap/bin/chromium', '/usr/bin/google-chrome'].find(existsSync);
if (!executablePath) throw new Error('No Chromium executable on this host.');
if (!output || !design) throw new Error('Run with PORTICO_RUNNER_OUTPUT and the exact design PORTICO_RUNNER_SECONDARY_SOURCE.');
const root = process.env.PORTICO_RUNNER_SITE ?? path.resolve('.');
const designRoot = path.join(design, 'design/ui-v1');
const require = createRequire(import.meta.url);
const {chromium} = require('playwright-core');
await mkdir(output, {recursive: true});

const mime = {'.jpg':'image/jpeg','.jpeg':'image/jpeg','.png':'image/png','.svg':'image/svg+xml','.webp':'image/webp'};
const server = await createServer({root, configFile:path.join(root,'vite.config.ts'), cacheDir:path.join(process.env.TMPDIR ?? output,'detail-vite'), plugins:[{name:'detail-reference-assets',configureServer(context){context.middlewares.use(async(req,res,next)=>{const pathname=decodeURIComponent(new URL(req.url,'http://localhost').pathname);const prefix=pathname.startsWith('/__design-assets__/assets/')?'/__design-assets__/assets/':pathname.startsWith('/assets/')?'/assets/':null;if(!prefix)return next();const relative=pathname.slice(prefix.length);if(relative.includes('..')||path.isAbsolute(relative))return next();try{res.setHeader('Content-Type',mime[path.extname(relative)]??'application/octet-stream');res.end(await readFile(path.join(designRoot,'assets',relative)));}catch{res.statusCode=404;res.end();}});}}],server:{host:'127.0.0.1',port:0}});
let browser;
try {
 await server.listen(); const origin=`http://127.0.0.1:${server.httpServer.address().port}`;
 browser=await chromium.launch({executablePath,headless:true,args:['--no-sandbox','--disable-dev-shm-usage']});
 const page=await browser.newPage({viewport:{width:1440,height:900},deviceScaleFactor:1});page.setDefaultTimeout(20000);
 await page.goto(`${origin}/src/dev/entries/show.html?frame=6`);await page.waitForSelector('[data-show-frame6-ready]');await page.getByRole('option').filter({hasText:'Season 4'}).focus();await page.evaluate(()=>document.fonts.ready);await page.screenshot({path:`${output}/show6-build.jpg`,type:'jpeg',quality:90});
 const kit=await readFile(path.join(designRoot,'kit.css'),'utf8');
 const source=(await readFile(path.join(designRoot,'desktop-detail.html'),'utf8')).replace('<link rel="stylesheet" href="kit.css">',`<link rel="stylesheet" href="${origin}/src/kit/fonts.css"><style>${kit}</style>`);
 await page.route('**/__detail_reference__',route=>route.fulfill({contentType:'text/html',body:source.replace('<head>','<head><base href="'+origin+'/">')}));
 await page.goto(`${origin}/__detail_reference__?only=6`);await page.evaluate(()=>document.fonts.ready);
 await page.locator('#f6').screenshot({path:`${output}/show6-frame.jpg`,type:'jpeg',quality:90});
 const frame=await readFile(`${output}/show6-frame.jpg`),build=await readFile(`${output}/show6-build.jpg`);
 await page.setViewportSize({width:2880,height:900});await page.setContent(`<body style="margin:0;display:flex;background:#071018"><img width="1440" src="data:image/jpeg;base64,${frame.toString('base64')}"><img width="1440" src="data:image/jpeg;base64,${build.toString('base64')}"></body>`);await page.screenshot({path:`${output}/show6-pair.jpg`,type:'jpeg',quality:90,fullPage:true});
 console.log('show6 pair captured');
} finally {await browser?.close();await server.close();}
