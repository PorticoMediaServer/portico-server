#!/usr/bin/env node
import {writeFileSync} from 'node:fs';
import path from 'node:path';
import {bytes, picture, pgsPresentation, pgsWindows, pgsPalette, pgsObject, pgsEnd} from '../helpers/subtitle-samples.ts';
const root=process.argv[2];
if(!root)throw new Error('Pass the generated browser media directory');
const art=picture(...Array.from({length:24},(_,y)=>Array.from({length:200},(_,x)=>(y<3||y>20||x<3||x>196||Math.floor(x/12)%2===0)?'1':'0').join('')));
writeFileSync(path.join(root,'movies/Browser Film (2024)/Browser Film (2024).en.sup'),bytes(
 pgsPresentation(0,[640,360],'epoch',0,[{id:0,x:220,y:290}]),pgsWindows(0,[[0,220,290,200,24]]),pgsPalette(0,0,[[1,235,128,128,255]]),pgsObject(0,0,art),pgsEnd(0),
 pgsPresentation(40000,[640,360],'normal',0,[]),pgsEnd(40000),
));
