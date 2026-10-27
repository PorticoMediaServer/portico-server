"""Synthetic local live TV source: three channels and an original XMLTV guide."""
from pathlib import Path
from datetime import datetime, timedelta, timezone
from common import run, art
import xml.etree.ElementTree as ET


def generate(root: Path, ffmpeg: str, full: bool = True) -> list[dict]:
    live = root / 'Live'
    live.mkdir(parents=True, exist_ok=True)
    media = live / 'source.mp4'
    if not media.exists():
        run([ffmpeg,'-y','-f','lavfi','-i','testsrc2=size=640x360:rate=24',
             '-f','lavfi','-i','sine=frequency=330:sample_rate=48000','-t','12',
             '-c:v','libx264','-preset','ultrafast','-pix_fmt','yuv420p','-g','48',
             '-c:a','aac','-b:a','96k','-shortest',media])
    anchor = datetime(2026,10,27,tzinfo=timezone.utc)
    xml = ET.Element('tv', {'generator-info-name':'Portico original synthetic source'})
    playlist = ['#EXTM3U']
    manifest = []
    for i, name in enumerate(['Harbour Workshop','Orbit Classroom','Nocturne Radio'], 1):
        cid = 'synthetic-'+str(i)
        channel = ET.SubElement(xml,'channel',{'id':cid})
        ET.SubElement(channel,'display-name').text = name
        art(live / f'channel-{i}.png', name, (400,225), seed=i)
        playlist += [f'#EXTINF:-1 tvg-id="{cid}" tvg-name="{name}" tvg-logo="http://127.0.0.1:32511/channel-{i}.png" group-title="Original Synthetic",{name}',
                     'http://127.0.0.1:32511/live.m3u8']
        for hour in range(24*14):
            start = anchor + timedelta(hours=hour)
            programme = ET.SubElement(xml,'programme',{'channel':cid,'start':start.strftime('%Y%m%d%H%M%S +0000'),
                'stop':(start+timedelta(hours=1)).strftime('%Y%m%d%H%M%S +0000')})
            ET.SubElement(programme,'title',{'lang':'en'}).text = f'{name}: Session {hour+1}'
            ET.SubElement(programme,'desc').text = 'Original generated picture and synthesized tone. No external content.'
            ET.SubElement(programme,'category').text = ['Education','Science','Music'][i-1]
        manifest.append({'kind':'live-channel','title':name,'path':f'Live/channel-{i}.png',
                         'features':['local-m3u','rolling-hls','xmltv-14-days','synthetic-av']})
    (live/'channels.m3u').write_text('\n'.join(playlist)+'\n')
    ET.indent(xml)
    (live/'guide.xml').write_bytes(ET.tostring(xml,encoding='utf-8',xml_declaration=True))
    return manifest
