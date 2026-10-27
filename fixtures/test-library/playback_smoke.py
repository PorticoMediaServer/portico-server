#!/usr/bin/env python3
"""Exercise actual media bodies and a bounded ten-client API/playback smoke.

Credentials are read only from a private local file. Grant URLs and sessions
remain in memory; reports contain counts, modes and failures, never secrets.
Run after seed_state.py. This is service qualification, not device acceptance.
"""
import argparse, concurrent.futures, json, os, shutil, subprocess, tempfile, time, urllib.request, urllib.error, uuid
from pathlib import Path
from seed_state import Client, private_json, sign_in, select_profile, items, Refusal


def capabilities():
    return {'version':2,'client':{'family':'web','form':'browser'},
        'display':{'dynamicRanges':['sdr']},'audioOutput':{'maxChannels':8},
        'engines':{'native':{'name':'ffmpeg','evidence':'declared',
            'video':[{'codec':'h264','bitDepths':[8],'maxWidth':3840,'maxHeight':2160,'maxFrameRate':60,'dynamicRanges':['sdr']}],
            'audio':[{'codec':c,'maxChannels':8} for c in ['aac','mp3','flac','alac','opus','ac3','eac3','dts']],
            'transports':[
                {'transport':'direct','containers':['mp4','mkv','matroska','mp3','flac','ogg','mpegts','webm','m4a','m4b'], 'video':['h264'],'audio':['aac','mp3','flac','alac','opus','ac3','eac3','dts']},
                {'transport':'hls_ts','containers':['mpegts'],'video':['h264'],'audio':['aac','mp3']},
                {'transport':'hls_fmp4','containers':['fmp4'],'video':['h264'],'audio':['aac','mp3']}],
            'subtitleRendering':['text','ass'],'embeddedAudioSwitching':True}}}


def request(client, method, path, body=None, headers=None, ok=(200,201,204), timeout=60):
    h={'Accept':'application/json','Content-Type':'application/json',
       'X-Portico-Installation-Id':client.installation,'Authorization':'Bearer '+client.token}
    h.update(headers or {})
    req=urllib.request.Request(client.base+path,json.dumps(body).encode() if body is not None else None,h,method=method)
    try:r=client.opener.open(req,timeout=timeout)
    except urllib.error.HTTPError as e:r=e
    with r:
        data=r.read(4*1024*1024)
        doc=json.loads(data) if data else None
        if r.status not in ok:
            code=doc.get('error',{}).get('code') if isinstance(doc,dict) else 'http'
            raise Refusal(code or 'http',r.status)
        return doc


def connect(base,cfg):
    client=Client(base)
    signed,client.token=sign_in(client,cfg['owner']['username'],cfg['owner']['password'])
    client.device=signed.get('session',signed)['deviceId']
    primary=next(p for p in signed['profiles'] if p.get('primary'))
    client.token=select_profile(client,primary['id'])
    client.call('PUT','/v1/me/devices/current/capabilities',capabilities(),ok=(204,))
    return client


def start(client,item,converted=False):
    body={'itemId':item,'startFrom':'beginning'}
    if converted: body['quality']={'mode':'limit','maxVideoBitrateKbps':180,'maxHeight':360,'maxAudioBitrateKbps':32}
    key={'Idempotency-Key':str(uuid.uuid4())}
    deadline=time.monotonic()+90
    while True:
        session=request(client,'POST','/v1/playback/sessions',body,key,ok=(201,202))
        if (session.get('presentation') or {}).get('url') and not session.get('retryAfterMs'):return session
        if time.monotonic()>=deadline:
            stop(client,session)
            raise RuntimeError('playback_preparation_timeout')
        time.sleep(1)


def stop(client,session):
    if session:client.call('DELETE','/v1/playback/sessions/'+session['id'],ok=(204,))


def decode(ffmpeg,url,log,seconds=2):
    with log.open('wb') as stream:
        r=subprocess.run([ffmpeg,'-hide_banner','-loglevel','error','-nostdin','-threads','1',
            '-rw_timeout','20000000','-i',url,'-t',str(seconds),'-f','null','-'],stdout=subprocess.DEVNULL,stderr=stream,timeout=60)
    return r.returncode==0


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--base',default='http://localhost:49300')
    p.add_argument('--credentials',type=Path,default=Path('/home/justin/projects/Portico/.private/walks/credentials.json'))
    p.add_argument('--report',type=Path,required=True)
    p.add_argument('--ffmpeg',default=shutil.which('ffmpeg'))
    p.add_argument('--concurrency-seconds',type=int,default=600)
    p.add_argument('--clients',type=int,default=10)
    args=p.parse_args(); cfg=private_json(args.credentials)
    args.report.parent.mkdir(parents=True,exist_ok=True)
    scratch=Path(tempfile.mkdtemp(prefix='playback-smoke-',dir=args.report.parent))
    report={'checks':[],'concurrency':{},'errors':[]};client=None
    try:
        client=connect(args.base,cfg)
        libraries=items(client.read('/v1/libraries'))
        films=[];episodes=[];songs=[];books=[]
        from seed_state import library_items
        for lib in libraries:
            if lib.get('kind') not in ('movie','tv','anime','music','audiobook'):continue
            values=library_items(client,lib,maximum=3000)
            for entry in values:
                kind=entry.get('kind')
                if kind=='movie':films.append(entry)
                elif kind=='episode':episodes.append(entry)
                elif kind=='song':songs.append(entry)
                elif kind in ('audiobook_file','part'):books.append(entry)
        selected=[('film',films[0] if films else None),('episode',episodes[0] if episodes else None),
                  ('song',songs[0] if songs else None),('audiobook',books[0] if books else None)]
        for kind,entry in selected:
            if not entry:
                report['errors'].append({'step':kind,'error':'missing_item'});continue
            for converted in [False,True]:
                s=None
                try:
                    s=start(client,entry['id'],converted)
                    presentation=s['presentation'];mode=presentation['mode']
                    url=client.base+presentation['url']
                    passed=decode(args.ffmpeg,url,scratch/(kind+str(converted)+'.log'))
                    report['checks'].append({'kind':kind,'requested':'conversion' if converted else 'original','mode':mode,'decoded':passed})
                    if not passed:report['errors'].append({'step':kind,'error':'decode_failed','converted':converted})
                except Exception as e:
                    report['errors'].append({'step':kind,'converted':converted,'error':getattr(e,'code',type(e).__name__),'status':getattr(e,'status',None)})
                finally:stop(client,s)
        if args.concurrency_seconds and films:
            def worker(index):
                c=None;s=None;requests=0;failures=[];decoder=None;log=None
                begun=time.monotonic()
                try:
                    c=connect(args.base,cfg);s=start(c,films[index%len(films)]['id'])
                    log=(scratch/f'client-{index}.log').open('wb')
                    decoder=subprocess.Popen([args.ffmpeg,'-hide_banner','-loglevel','error','-nostdin','-threads','1',
                        '-stream_loop','-1','-re','-i',c.base+s['presentation']['url'],'-t',str(args.concurrency_seconds),'-f','null','-'],
                        stdout=subprocess.DEVNULL,stderr=log)
                    seq=0
                    while time.monotonic()-begun<args.concurrency_seconds:
                        c.read('/v1/libraries');requests+=1
                        c.read('/v1/home');requests+=1
                        seq+=1
                        request(c,'POST','/v1/playback/sessions/'+s['id']+'/timeline',
                            {'seq':seq,'generation':s['presentation']['generation'],'state':'playing','positionMs':1000,'rate':1},ok=(204,))
                        requests+=1
                        if decoder.poll() is not None:
                            failures.append('decoder_ended_early');break
                        time.sleep(min(5,max(.01,args.concurrency_seconds-(time.monotonic()-begun))))
                except Exception as e:failures.append(getattr(e,'code',type(e).__name__))
                finally:
                    if decoder and decoder.poll() is None:
                        decoder.terminate()
                        try:decoder.wait(timeout=5)
                        except subprocess.TimeoutExpired:decoder.kill();decoder.wait()
                    if log:log.close()
                    if c:
                        try:stop(c,s)
                        finally:c.call('DELETE','/v1/devices/'+c.device,ok=(204,))
                return {'client':index,'requests':requests,'durationSeconds':round(time.monotonic()-begun,2),'errors':failures}
            with concurrent.futures.ThreadPoolExecutor(max_workers=args.clients) as pool:
                results=list(pool.map(worker,range(args.clients)))
            report['concurrency']={'clients':args.clients,'targetSeconds':args.concurrency_seconds,'results':results,
                'passed':all(not r['errors'] and r['durationSeconds']>=args.concurrency_seconds for r in results)}
    finally:
        if client:
            try:client.call('DELETE','/v1/devices/'+client.device,ok=(204,))
            except Exception:report['errors'].append({'step':'cleanup','error':'device_retirement_failed'})
        shutil.rmtree(scratch)
        args.report.write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps({'checks':len(report['checks']),'errors':len(report['errors']),'concurrency':report['concurrency'].get('passed')}))
    return 1 if report['errors'] or report['concurrency'].get('passed') is False else 0


if __name__=='__main__':raise SystemExit(main())
