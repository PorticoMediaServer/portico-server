"""Tiny Wikimedia Commons helper (uses curl for TLS)."""
import json, subprocess, urllib.parse, sys
API='https://commons.wikimedia.org/w/api.php'
UA='PorticoCinemaMockup/0.1 (fixture artwork; justinehler@icloud.com)'
def get(params):
    q=urllib.parse.urlencode({**params,'format':'json'})
    out=subprocess.run(['curl','-sfL','-A',UA,API+'?'+q],capture_output=True,text=True).stdout
    return json.loads(out) if out else {}
def search(term, limit=8):
    d=get({'action':'query','generator':'search','gsrsearch':term,'gsrnamespace':6,'gsrlimit':limit,'prop':'imageinfo','iiprop':'url|size|extmetadata','iiurlwidth':800})
    res=[]
    for p in (d.get('query',{}).get('pages',{}) or {}).values():
        ii=(p.get('imageinfo') or [{}])[0]; md=ii.get('extmetadata',{})
        res.append({'title':p['title'],'w':ii.get('width'),'h':ii.get('height'),'thumb':ii.get('thumburl'),'url':ii.get('url'),'license':md.get('LicenseShortName',{}).get('value'),'index':p.get('index',0)})
    return sorted(res,key=lambda r:r['index'])
def download(url, path):
    return subprocess.run(['curl','-sfL','-A',UA,'-o',path,url]).returncode==0
if __name__=='__main__':
    for r in search(' '.join(sys.argv[1:])): print(r['license'],r['w'],r['h'],r['title'])
