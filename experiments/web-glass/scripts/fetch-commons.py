import json, urllib.request, urllib.parse, sys, os, time, ssl
ssl._create_default_https_context = lambda: ssl.create_default_context(cafile="/etc/ssl/cert.pem")
UA="PorticoFixtureBuilder/1.0 (offline fixture artwork build)"
out=os.path.join(os.path.dirname(__file__),"..",".scratch","commons"); os.makedirs(out,exist_ok=True); meta={}
for line in open(os.path.join(os.path.dirname(__file__),"commons-list.txt")):
    line=line.strip()
    if not line: continue
    key,fn,w=line.split("|")
    q=urllib.parse.urlencode({"action":"query","titles":"File:"+fn,"prop":"imageinfo","iiprop":"url|size|extmetadata","iiurlwidth":w,"format":"json"})
    req=urllib.request.Request("https://commons.wikimedia.org/w/api.php?"+q,headers={"User-Agent":UA})
    d=json.load(urllib.request.urlopen(req))
    p=list(d["query"]["pages"].values())[0]
    if "imageinfo" not in p: print("MISSING",key,fn); continue
    ii=p["imageinfo"][0]
    url=ii.get("thumburl") or ii["url"]
    em=ii.get("extmetadata",{})
    lic=em.get("LicenseShortName",{}).get("value","?"); art=em.get("Artist",{}).get("value","?")
    ext=os.path.splitext(url.split("?")[0])[1].lower()
    dest=f"{out}/{key}{ext}"
    for attempt in range(4):
        try:
            r=urllib.request.urlopen(urllib.request.Request(url,headers={"User-Agent":UA}))
            open(dest,"wb").write(r.read()); break
        except Exception as e:
            print("retry",key,e); time.sleep(3)
    meta[key]={"file":fn,"license":lic,"artist":art[:200],"source":"https://commons.wikimedia.org/wiki/File:"+fn.replace(" ","_"),"path":dest}
    print(key,ii["width"],ii["height"],lic)
    time.sleep(0.4)
json.dump(meta,open(os.path.join(os.path.dirname(__file__),"..","src","data","generated","commons-credits.json"),"w"),indent=1)
