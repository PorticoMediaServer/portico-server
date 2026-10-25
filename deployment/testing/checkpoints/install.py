#!/usr/bin/env python3
"""Install only this campaign's user-level machine timers; never agent wakes."""
import argparse,hashlib,json,os,subprocess
from pathlib import Path

ROOT=Path('/home/justin/projects/Portico')
def atomic(path,raw):
    temporary=path.with_name(path.name+'.new');temporary.write_bytes(raw);temporary.chmod(0o600);temporary.replace(path)
def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--config',type=Path,required=True);args=parser.parse_args();os.umask(0o077)
    if args.config.resolve()!=ROOT/'.private/v3-checkpoints/config.json':parser.error('fixed private campaign config required')
    cfg=json.loads(args.config.read_text());directory=args.config.parent;source=Path(__file__).with_name('run.py');installed=directory/'run.py';atomic(installed,source.read_bytes())
    units=Path.home()/'.config/systemd/user';units.mkdir(parents=True,exist_ok=True)
    for name,flags,calendar,timeout in [('portico-v3-checkpoints','','*-*-* 00,04,08,12,16,20:47:00 UTC','75min'),('portico-v3-ci-audit',' --audit-ci','*-*-* *:50:00 UTC','5min')]:
        service=f'''[Unit]
Description=Portico v3 unattended {'M0 checkpoints' if not flags else 'hourly CI failure inbox'}
[Service]
Type=oneshot
WorkingDirectory={ROOT}
ExecStart=/usr/bin/python3 {installed} --config {args.config}{flags}
Environment=PATH=/home/justin/.local/bin:/usr/local/bin:/usr/bin:/bin
TimeoutStartSec={timeout}
KillMode=control-group
UMask=0077
StandardOutput=append:{directory}/{name}.log
StandardError=append:{directory}/{name}.log
'''
        timer=f'''[Unit]
Description=Portico v3 {'four-hour' if not flags else 'hourly'} machine checks
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
    subprocess.run(['systemctl','--user','enable','--now','portico-v3-checkpoints.timer','portico-v3-ci-audit.timer'],check=True,stdout=subprocess.DEVNULL)
    evidence=Path(cfg['evidence']);evidence.mkdir(parents=True,exist_ok=True)
    receipt={'coordinatorSHA256':hashlib.sha256(installed.read_bytes()).hexdigest(),'sourcePrivateMain':subprocess.check_output(['git','rev-parse','HEAD'],cwd=source.parent,text=True).strip(),'timers':[]}
    for name in ['portico-v3-checkpoints','portico-v3-ci-audit']:
        result=subprocess.check_output(['systemctl','--user','show',name+'.timer','--property=ActiveState,NextElapseUSecRealtime'],text=True).strip();receipt['timers'].append({'name':name,'state':result})
    (evidence/'machine-timers.json').write_text(json.dumps(receipt,indent=2)+'\n');print(json.dumps(receipt))
if __name__=='__main__':main()
