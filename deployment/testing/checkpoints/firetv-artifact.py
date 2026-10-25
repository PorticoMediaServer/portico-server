#!/usr/bin/env python3
"""Read a built Fire OS APK's declared DEX classes and manifest metadata only."""
import argparse,hashlib,json,re,struct,zipfile
from pathlib import Path

def audit(apk,inventory):
 counts={'gmsClasses':0,'castClasses':0,'porticoCastClasses':0};files=0;total=0
 with zipfile.ZipFile(apk) as archive:
  manifest=archive.read('AndroidManifest.xml')
  metadata=[name for name in ['PorticoCastApplicationID','com.google.android.gms.cast.framework.OPTIONS_PROVIDER_CLASS_NAME','PorticoCastOptions'] if name.encode() in manifest or name.encode('utf-16le') in manifest]
  for entry in archive.infolist():
   if not re.fullmatch(r'classes[0-9]*\.dex',entry.filename):continue
   files+=1;total+=entry.file_size
   if files>32 or entry.file_size>128*1024*1024 or total>256*1024*1024:raise ValueError('dex-budget')
   data=archive.read(entry)
   if len(data)<112 or data[:4]!=b'dex\n':raise ValueError('dex-format')
   def word(offset):
    if offset<0 or offset+4>len(data):raise ValueError('dex-bound')
    return struct.unpack_from('<I',data,offset)[0]
   if word(40)!=0x12345678:raise ValueError('dex-endian')
   ns,strings=word(56),word(60);nt,types=word(64),word(68);nc,classes=word(96),word(100)
   if max(ns,nt,nc)>2000000 or strings+4*ns>len(data) or types+4*nt>len(data) or classes+32*nc>len(data):raise ValueError('dex-table-bound')
   for index in range(nc):
    typ=word(classes+32*index)
    if typ>=nt:raise ValueError('dex-type-index')
    string=word(types+4*typ)
    if string>=ns:raise ValueError('dex-string-index')
    offset=word(strings+4*string)
    for _ in range(5):
     if offset>=len(data):raise ValueError('dex-string-bound')
     byte=data[offset];offset+=1
     if byte<128:break
    else:raise ValueError('dex-length')
    end=data.find(b'\0',offset,min(offset+2048,len(data)))
    if end<0:raise ValueError('dex-class-name-bound')
    descriptor=data[offset:end]
    if descriptor.startswith(b'Lcom/google/android/gms/'):counts['gmsClasses']+=1
    if descriptor.startswith(b'Lcom/google/android/gms/cast/'):counts['castClasses']+=1
    if descriptor.startswith(b'Ltv/getportico/portico/native/PorticoCast'):counts['porticoCastClasses']+=1
 if not files:raise ValueError('no-dex')
 entries=[line.strip() for line in inventory.read_text().splitlines() if line.strip() and not line.startswith('#')]
 result={'apkSHA256':hashlib.sha256(apk.read_bytes()).hexdigest(),'declaredDexFiles':files,**counts,'castMetadataPresent':bool(metadata),'runtimeInventoryHasGMS':any(line.startswith('com.google.android.gms:') for line in entries),'qualification':'Static actual APK and checked release inventory only; Menu/Back/runtime parity separate.'}
 result['status']='pass' if not any(counts.values()) and not metadata and not result['runtimeInventoryHasGMS'] else 'fail'
 return result

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--apk',type=Path,required=True);p.add_argument('--inventory',type=Path,required=True);p.add_argument('--out',type=Path,required=True);a=p.parse_args();result=audit(a.apk,a.inventory);a.out.write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result));return int(result['status']!='pass')
if __name__=='__main__':raise SystemExit(main())
