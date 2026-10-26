#!/usr/bin/env python3
"""Install only this campaign's user-level machine timers; never agent wakes."""
import argparse,hashlib,json,os,subprocess
from pathlib import Path

ROOT=Path('/home/justin/projects/Portico')
def atomic(path,raw):
    temporary=path.with_name(path.name+'.new');temporary.write_bytes(raw);temporary.chmod(0o600);temporary.replace(path)
def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--config',type=Path,required=True);parser.add_argument('--android',action='store_true',help='enable qualified native machine flows');args=parser.parse_args();os.umask(0o077)
    if args.config.resolve()!=ROOT/'.private/v3-checkpoints/config.json':parser.error('fixed private campaign config required')
    args.config=args.config.resolve()
    cfg=json.loads(args.config.read_text());directory=args.config.parent;source=Path(__file__).with_name('run.py');installed=directory/'run.py';atomic(installed,source.read_bytes())
    native=directory/'native-tools/checkpoints'
    if args.android:
        native.mkdir(parents=True,exist_ok=True)
        for name in ['native-build.py','native-checkpoint.py','android.py','avd_custody.py','device_runtime.py','phone_flows.py','tv_flows.py','run.py','firetv-artifact.py']:atomic(native/name,source.with_name(name).read_bytes())
        helper=native.parent/'visual-qa/auth-driver/private_driver.py';helper.parent.mkdir(parents=True,exist_ok=True);atomic(helper,source.parent.parent.joinpath('visual-qa/auth-driver/private_driver.py').read_bytes())
    units=Path.home()/'.config/systemd/user';units.mkdir(parents=True,exist_ok=True)
    jobs=[('portico-v3-checkpoints','','*-*-* 00,04,08,12,16,20:47:00 UTC','75min'),('portico-v3-ci-audit',' --audit-ci','*-*-* *:50:00 UTC','5min'),('portico-v3-ledger',' --refresh-ledger','*-*-* 00,02,04,06,08,10,12,14,16,18,20,22:35:00 UTC','5min')]
    if args.android:jobs.append(('portico-v3-android','','*-*-* 01,03,05,07,09,11,13,15,17,19,21,23:17:00 UTC','140min'))
    for name,flags,calendar,timeout in jobs:
        entry=native/'native-checkpoint.py' if name=='portico-v3-android' else installed
        service=f'''[Unit]
Description=Portico v3 unattended {'M0 checkpoints' if not flags else 'ledger refresh' if flags==' --refresh-ledger' else 'hourly CI failure inbox'}
[Service]
Type=oneshot
WorkingDirectory={ROOT}
ExecStart=/usr/bin/python3 {entry} --config {args.config}{flags}
Environment=PATH=/home/justin/Android/Sdk/platform-tools:/home/justin/.local/bin:/usr/local/bin:/usr/bin:/bin
TimeoutStartSec={timeout}
KillMode=control-group
UMask=0077
StandardOutput=append:{directory}/{name}.log
StandardError=append:{directory}/{name}.log
'''
        timer=f'''[Unit]
Description=Portico v3 {'two-hour' if name=='portico-v3-android' else 'four-hour' if not flags else 'two-hour' if flags==' --refresh-ledger' else 'hourly'} machine checks
[Timer]
OnCalendar={calendar}
Persistent=false
AccuracySec=1min
Unit={name}.service
[Install]
WantedBy=timers.target
'''
        atomic(units/(name+'.service'),service.encode());atomic(units/(name+'.timer'),timer.encode())
    subprocess.run(['systemctl','--user','daemon-reload'],check=True)
    subprocess.run(['systemctl','--user','enable','--now','portico-v3-checkpoints.timer','portico-v3-ci-audit.timer','portico-v3-ledger.timer',*(['portico-v3-android.timer'] if args.android else [])],check=True,stdout=subprocess.DEVNULL)
    evidence=Path(cfg['evidence']);evidence.mkdir(parents=True,exist_ok=True)
    receipt={'coordinatorSHA256':hashlib.sha256(installed.read_bytes()).hexdigest(),'sourcePrivateMain':subprocess.check_output(['git','rev-parse','HEAD'],cwd=source.parent,text=True).strip(),'nativeToolsSHA256':{f.name:hashlib.sha256(f.read_bytes()).hexdigest() for f in native.glob('*.py')} if args.android else {},'timers':[]}
    for name,_,_,_ in jobs:
        result=subprocess.check_output(['systemctl','--user','show',name+'.timer','--property=ActiveState,NextElapseUSecRealtime'],text=True).strip();receipt['timers'].append({'name':name,'state':result})
    (evidence/'machine-timers.json').write_text(json.dumps(receipt,indent=2)+'\n');print(json.dumps(receipt))
if __name__=='__main__':main()
