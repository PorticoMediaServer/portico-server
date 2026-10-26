import fs from 'node:fs';
import {pathToFileURL} from 'node:url';
import {createSession} from './browser/harness.mjs';
import {runWeb} from './browser/web.mjs';
import {runHosted} from './browser/hosted.mjs';

const input=JSON.parse(fs.readFileSync(process.env.PORTICO_CHECKPOINT_INPUT,'utf8'));
const {chromium}=await import(pathToFileURL(process.env.PORTICO_CHECKPOINT_BROWSER));
const {privacy}=await import(pathToFileURL(process.env.PORTICO_RUNNER_SECONDARY_SOURCE+'/scripts/review-smoke-privacy.mjs'));
const out=process.env.PORTICO_RUNNER_OUTPUT;
const session=createSession({input,chromium,privacy,out});
try{
 await session.start();
 if(input.surface==='web')await runWeb(session);
 else if(input.surface==='hosted')await runHosted(session);
 else throw new Error('unsupported checkpoint surface');
}catch(error){session.crash(error);}
await session.finish();
