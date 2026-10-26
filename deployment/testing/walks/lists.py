#!/usr/bin/env python3
"""Generate walks/lists/<surface>.md from implemented, in-scope ledger rows.

Usage (always under the claims lock when writing the campaign lists):
    flock /tmp/portico-v3-claims.lock python3 deployment/testing/walks/lists.py

The ledger lives in the campaign (`_program/ledger`); the campaign v3 directory
receives `walks/lists/`. Rows are grouped by their source pointer's frame label.
Only implemented rows are walkable. Root §§36/38/56/59 exclusions are omitted and
never credited. Fire OS and Vega reuse the TV rows; this script invents no IDs.

The result is a starting point for walkers: they claim a frame group under the
list's "## Claims" section, then write `walks/results/<worker>.md` in the exact
table from WALKS.md.
"""
import argparse,datetime,json,os,re
from pathlib import Path

ROOT=Path('/home/justin/projects/Portico')
DEFAULT_PROGRAM=ROOT/'.worktrees/ui-2026/_program'
SURFACES=['web-desktop','web-phone','ios-phone','ipad','android-phone','tvos','android-tv','fire-os','vega']
TV_SURFACES={'tvos','android-tv','fire-os','vega'}
EXCLUDED={'F-MUS-E9621A8','F-MUS-EDF7FE7','S-ACC-997896E','S-ACC-694BA5F'}
NATIVE_PHONE_FRAMES={'phone-app','phone-detail','phone-player','phone-read','phone-guide'}
M0=[
    ('sign-in',re.compile(r'^(auth|account|connect|welcome|signin)(?:#|$)',re.I)),
    ('Home',re.compile(r'^(home|phone-app#1(?:\.|$))',re.I)),
    ('browse',re.compile(r'^(browse|library|libraries|collections|see-all)',re.I)),
    ('detail',re.compile(r'^(detail|phone-detail)',re.I)),
    ('playback',re.compile(r'^(player|phone-player|watch|playback|video|music)',re.I)),
    ('Settings',re.compile(r'^(settings|s/|owner-|server-|remote-|account-)',re.I)),
    ('Read',re.compile(r'^(read|phone-read|book|comic)',re.I)),
]
FRAME=re.compile(r'^([A-Za-z0-9_-]+)#([0-9][A-Za-z0-9.]*)$')
WALKABLE=[
    re.compile(r'^[A-Za-z0-9_-]+#[0-9]+(?:\.[A-Za-z0-9]+)*$'),
    re.compile(r'^[A-Za-z0-9_.-]+\.html#[A-Za-z0-9]+$'),
    re.compile(r'^[A-Za-z0-9_-]+#[0-9]+/[0-9]+$'),
    re.compile(r'^s/[A-Za-z0-9_-]+#group[0-9]+$'),
]

def frame_group(pointer):
    first=pointer.split(';',1)[0].strip()
    if '#' not in first:return ''
    return first if any(pattern.match(first) for pattern in WALKABLE) else ''

def surfaces_for(platform,pointer,group):
    p=platform.strip().lower()
    if p=='web':return ['web-desktop']
    if p=='mweb':return ['web-phone']
    if p=='cast':return ['web-desktop']
    if p=='tablet':return ['ipad']
    if p=='tv':return ['tvos','android-tv','fire-os','vega']
    if p=='phone':
        native='N:' in pointer or group.split('#',1)[0] in NATIVE_PHONE_FRAMES
        return ['ios-phone','android-phone'] if native else ['web-phone']
    if p=='all':
        base=group.split('#',1)[0].removesuffix('.html')
        if group.startswith('s/'):return list(SURFACES)
        if 'N:' in pointer or base in NATIVE_PHONE_FRAMES:return ['ios-phone','android-phone']
        if base.startswith('remote-') or base.startswith('cast-'):return ['web-desktop','web-phone']
        if base.startswith('desktop-'):return ['web-desktop']
        return list(SURFACES)
    return []

def ledger_rows(program):
    rows=[];excluded=0;not_a_frame=0
    for path in sorted((program/'ledger').glob('*.md')):
        for line in path.read_text(encoding='utf-8').splitlines():
            cells=[cell.strip() for cell in line.strip().strip('|').split('|')]
            if len(cells)<7 or cells[0]=='ID' or not re.match(r'^[A-Z]',cells[0]):continue
            rid,area,platform,feature,status,pointer=cells[:6]
            if status.lower()!='implemented':continue
            if rid in EXCLUDED:excluded+=1;continue
            group=frame_group(pointer)
            if not group:not_a_frame+=1;continue
            rows.append({'id':rid,'area':area,'platform':platform,'feature':feature,'pointer':pointer,'group':group})
    return rows,excluded,not_a_frame

def m0_rank(group,feature):
    for index,(_,pattern) in enumerate(M0):
        if pattern.search(group):return index
    return len(M0)

def clean(text,limit):
    value=' '.join(str(text).replace('|','/').split())
    return value if len(value)<=limit else value[:limit-1]+'…'

def render(surface,rows):
    stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%d %H:%M UTC')
    lines=[
        f'# Walk list: {surface}',
        '',
        f'Generated {stamp} from implemented, in-scope ledger rows. Claim one frame group under',
        '`flock /tmp/portico-v3-claims.lock` by adding one line under "## Claims". Use the current',
        'address in `walks/SERVER.md` and the current build in `walks/BUILDS.md`; never the demo server.',
        '',
        'Every OFF-FRAME result must cite the mock-up file and frame, the element or selector, and the',
        "frame's HTML/CSS expected values next to the actual app values (WALKS.md §2b). `LOOK:` alone",
        'never becomes a fix item. Root §36/§38/§56/§59 exclusions are omitted and uncredited.',
    ]
    if surface in TV_SURFACES:
        lines+=['', 'Parity surface: these are the TV ledger rows; no new ledger IDs. Handle platform differences per MANUAL §13.']
    if surface=='fire-os':
        lines+=['', 'Fire OS walks the `firetv` flavour; use the Fire OS build from `walks/BUILDS.md`.']
    if surface=='vega':
        lines+=['', 'Vega walks the shared native app through its shims; use the Vega build and the vega device lock.']
    ranked=[]
    for row in rows:
        rank=m0_rank(row['group'],row['feature'])
        ranked.append((rank,row))
    m0=sorted((item for item in ranked if item[0]<len(M0)),key=lambda item:(item[0],item[1]['group'],item[1]['id']))
    other=sorted((item for item in ranked if item[0]==len(M0)),key=lambda item:(item[1]['group'],item[1]['id']))
    lines+=['','## M0 flows','']
    if not m0:lines.append('- none in this surface')
    for group,members in grouped([row for _,row in m0]):
        lines.extend(render_group(group,members))
    lines+=['','## Other frames','']
    if not other:lines.append('- none in this surface')
    for group,members in grouped([row for _,row in other]):
        lines.extend(render_group(group,members))
    lines+=['','## Claims','',f'- (none yet — one line per claimed frame group: `<frame> — <worker> — <UTC time>`)','']
    return '\n'.join(lines)+'\n'

def grouped(rows):
    groups=[]
    for row in rows:
        if not groups or groups[-1][0]!=row['group']:groups.append((row['group'],[]))
        groups[-1][1].append(row)
    return groups

def render_group(group,rows):
    match=FRAME.match(group)
    hint=f" — frame `{match.group(1)}.html` {match.group(2)}" if match else ''
    lines=[f'### `{group}`{hint}',f'_{len(rows)} row{"s" if len(rows)!=1 else ""}_','']
    for row in rows:
        lines.append(f"- `{row['id']}` — {clean(row['feature'],220)} — source `{clean(row['pointer'],240)}`")
    lines.append('')
    return lines

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--program',type=Path,default=DEFAULT_PROGRAM)
    parser.add_argument('--out',type=Path,default=None)
    parser.add_argument('--check',action='store_true')
    args=parser.parse_args()
    program=args.program.resolve();out=(args.out or program/'v3'/'walks'/'lists')
    if not (program/'ledger').is_dir():parser.error('ledger directory not found')
    rows,excluded,not_a_frame=ledger_rows(program)
    by_surface={surface:[] for surface in SURFACES}
    for row in rows:
        for surface in surfaces_for(row['platform'],row['pointer'],row['group']):
            by_surface[surface].append(row)
    for surface in SURFACES:
        by_surface[surface]=list({row['id']:row for row in by_surface[surface]}.values())
    summary={'generated':datetime.datetime.now(datetime.timezone.utc).isoformat(timespec='seconds'),'excluded':excluded,'without_frame':not_a_frame,'surfaces':{}}
    for surface in SURFACES:
        content=render(surface,by_surface[surface])
        summary['surfaces'][surface]={'rows':len(by_surface[surface]),'groups':len({row['group'] for row in by_surface[surface]})}
        if args.check:continue
        out.mkdir(parents=True,exist_ok=True)
        temporary=out/(surface+'.md.tmp')
        temporary.write_text(content,encoding='utf-8')
        os.replace(temporary,out/(surface+'.md'))
    print(json.dumps(summary,sort_keys=True))
    return 0

if __name__=='__main__':
    raise SystemExit(main())
