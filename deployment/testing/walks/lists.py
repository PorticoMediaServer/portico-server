#!/usr/bin/env python3
"""Generate walks/lists/<surface>.md from implemented, in-scope ledger rows.

Usage (always under the claims lock when writing the campaign lists):
    flock /tmp/portico-v3-claims.lock python3 deployment/testing/walks/lists.py

The ledger lives in the campaign (`_program/ledger`); the campaign v3 directory
receives `walks/lists/`. Rows are grouped by their source pointer's frame label.
Only implemented rows in the original v3 scope (v3/scope.py) are walkable. Pure desktop
app and Roku rows, and the four Root ID exclusions, are omitted and never credited.
Fire OS and Vega reuse the TV rows; this script invents no IDs.

The result is a starting point for walkers: they claim a frame group under the
list's "## Claims" section, then write `walks/results/<worker>.md` in the exact
table from WALKS.md.
"""
import argparse,datetime,json,os,re,sys
from pathlib import Path

ROOT=Path('/home/justin/projects/Portico')
DEFAULT_PROGRAM=ROOT/'.worktrees/ui-2026/_program'
SURFACES=['web-desktop','web-phone','ios-phone','ipad','android-phone','tvos','android-tv','fire-os','vega']
OVERRIDES=Path(__file__).with_name('surface-overrides.json')

def overrides(surface):
    '''Per-surface eligibility overrides, proven from the frame caption and markup.

    A record names a surface, a row id, the frame it was audited against, an
    action (include or exclude) and the caption evidence. Nothing here changes a
    ledger row's canonical status, its other surfaces or the ledger itself, and a
    row with no record stays exactly as generated.
    '''
    if not OVERRIDES.is_file():return {}
    try:
        document=json.loads(OVERRIDES.read_text())
    except (OSError,ValueError):
        return {}
    if not isinstance(document,dict) or document.get('schema')!=1:return {}
    records=document.get('overrides')
    if not isinstance(records,list):return {}
    kept={}
    for record in records:
        if not isinstance(record,dict) or record.get('surface')!=surface:continue
        identifier,action=record.get('id'),record.get('action')
        if not isinstance(identifier,str) or action not in ('include','exclude'):continue
        evidence=record.get('evidence') if isinstance(record.get('evidence'),dict) else {}
        kept[identifier]={'action':action,'frame':str(record.get('frame') or ''),
                          'reason':str(record.get('reason') or ''),
                          'caption':str(evidence.get('caption') or ''),
                          'file':str(evidence.get('file') or ''),'label':str(evidence.get('label') or '')}
    return kept

def audit_notes(surface, rows, applied):
    lines=[]
    for row in sorted(rows,key=lambda r:r['id']):
        record=applied.get(row['id'])
        if not record:continue
        lines.append('- `%s` %s `%s` — %s · %s %s' % (row['id'],record['action'],record['frame'],
                     record['file'],record['label'],record['caption']))
    if not lines:return []
    return ['','## Audit notes','',
            'Per-surface eligibility proven from the mock-up frame caption and markup. A ledger row keeps its',
            'canonical status, exclusions and every other surface; these records only decide this surface.', '']+lines
TV_SURFACES={'tvos','android-tv','fire-os','vega'}
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
FRAME=re.compile(r'^([A-Za-z0-9_-]+)#([A-Za-z]?[0-9][A-Za-z0-9.]*)$')
WALKABLE=[
    re.compile(r'^[A-Za-z0-9_-]+#[A-Za-z]?[0-9][A-Za-z0-9.]*$'),
    re.compile(r'^[A-Za-z0-9_.-]+\.html#[A-Za-z0-9]+$'),
    re.compile(r'^[A-Za-z0-9_-]+#[0-9]+/[0-9]+$'),
    re.compile(r'^s/[A-Za-z0-9_-]+#group[0-9]+$'),
]

def frame_group(pointer,embedded=False):
    tokens=pointer.split(';') if embedded else [pointer.split(';',1)[0]]
    for token in tokens:
        token=re.split(r'@',token.strip(),maxsplit=1)[0].strip().strip('`').strip()
        candidates=[token]+re.findall(r'[A-Za-z0-9_./-]+#[A-Za-z0-9][A-Za-z0-9./]*',token)
        for candidate in candidates:
            if any(pattern.match(candidate) for pattern in WALKABLE):return candidate
    return ''

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
    sys.path.insert(0,str(Path(program)/'v3'))
    import scope
    rows=[];excluded=0;unknown=0;not_a_frame=0
    for item in scope.load(program):
        if item['status']!='implemented':continue
        result=scope.verdict(item)
        if result['scope']=='unknown':unknown+=1;continue
        if result['scope']!='in':excluded+=1;continue
        platform=scope.surface_platform(item)
        group=frame_group(item['pointer'],embedded=item['schema']=='compact')
        if not group:not_a_frame+=1;continue
        rows.append({'id':item['id'],'area':item['area'],'platform':platform,'feature':item['feature'],'pointer':item['pointer'],'group':group})
    return rows,excluded,unknown,not_a_frame

def natural_key(value):
    return tuple(int(part) if part.isdigit() else part.casefold() for part in re.split(r'(\d+)',value))

def m0_rank(group,feature):
    for index,(_,pattern) in enumerate(M0):
        if pattern.search(group):return index
    return len(M0)

def clean(text,limit):
    value=' '.join(str(text).replace('|','/').split())
    return value if len(value)<=limit else value[:limit-1]+'…'

def render(surface,rows,claims):
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
    if surface not in TV_SURFACES:
        lines+=['', 'Root60: non-TV sign-in begins at `auth#4`; the TV `connect#40`/`auth#5` path is unchanged. No ledger rows are invented.']
    ranked=[]
    for row in rows:
        rank=m0_rank(row['group'],row['feature'])
        ranked.append((rank,row))
    m0=sorted((item for item in ranked if item[0]<len(M0)),key=lambda item:(item[0],natural_key(item[1]['group']),natural_key(item[1]['id'])))
    other=sorted((item for item in ranked if item[0]==len(M0)),key=lambda item:(natural_key(item[1]['group']),natural_key(item[1]['id'])))
    lines+=['','## M0 flows','']
    if not m0:lines.append('- none in this surface')
    for group,members in grouped([row for _,row in m0]):
        lines.extend(render_group(group,members))
    lines+=['','## Other frames','']
    if not other:lines.append('- none in this surface')
    for group,members in grouped([row for _,row in other]):
        lines.extend(render_group(group,members))
    body=claims.strip('\n') or '- (none yet — one line per claimed frame group: `<frame> — <worker> — <UTC time>`)'
    lines+=['','## Claims','',body,'']
    return '\n'.join(lines)+'\n'

def grouped(rows):
    groups=[]
    for row in rows:
        if not groups or groups[-1][0]!=row['group']:groups.append((row['group'],[]))
        groups[-1][1].append(row)
    return groups

def render_group(group,rows):
    match=FRAME.match(group)
    if match:
        hint=f" — frame `{match.group(1)}.html` {match.group(2)}"
    elif group.startswith('s/'):
        hint=' — frame `settings.html` (settings spec group)'
    else:
        hint=''
    lines=[f'### `{group}`{hint}',f'_{len(rows)} row{"s" if len(rows)!=1 else ""}_','']
    for row in rows:
        source='' if row['pointer']==row['feature'] else f" — source `{clean(row['pointer'],240)}`"
        lines.append(f"- `{row['id']}` — {clean(row['feature'],220)}{source}")
    lines.append('')
    return lines

def read_claims(path):
    if not path.exists():
        return ''
    text=path.read_text(encoding='utf-8')
    match=re.search(r'(?m)^## Claims\n',text)
    if not match:
        return ''
    body=text[match.end():].strip('\n')
    if body.strip().startswith('- (none yet'):
        return ''
    return body

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--program',type=Path,default=DEFAULT_PROGRAM)
    parser.add_argument('--out',type=Path,default=None)
    parser.add_argument('--check',action='store_true')
    args=parser.parse_args()
    program=args.program.resolve();out=(args.out or program/'v3'/'walks'/'lists')
    if not (program/'ledger').is_dir():parser.error('ledger directory not found')
    rows,excluded,unknown,not_a_frame=ledger_rows(program)
    by_surface={surface:[] for surface in SURFACES}
    for row in rows:
        for surface in surfaces_for(row['platform'],row['pointer'],row['group']):
            by_surface[surface].append(row)
    applied_by_surface={surface:overrides(surface) for surface in SURFACES}
    indexed={row['id']:row for row in rows}
    for surface in SURFACES:
        by_surface[surface]=list({row['id']:row for row in by_surface[surface]}.values())
        applied=applied_by_surface[surface]
        if not applied:continue
        by_surface[surface]=[row for row in by_surface[surface]
                             if applied.get(row['id'],{}).get('action')!='exclude']
        present={row['id'] for row in by_surface[surface]}
        for identifier,record in applied.items():
            if record['action']=='include' and identifier not in present and identifier in indexed:
                by_surface[surface].append(indexed[identifier])
    summary={'generated':datetime.datetime.now(datetime.timezone.utc).isoformat(timespec='seconds'),'excluded':excluded,'unknown':unknown,'without_frame':not_a_frame,'surfaces':{}}
    for surface in SURFACES:
        target=out/(surface+'.md')
        content=render(surface,by_surface[surface],read_claims(target))
        applied=applied_by_surface[surface]
        notes=audit_notes(surface,by_surface[surface],applied) if applied else []
        if notes:content=content.rstrip('\n')+'\n'+'\n'.join(notes)+'\n'
        summary['surfaces'][surface]={'rows':len(by_surface[surface]),'groups':len({row['group'] for row in by_surface[surface]})}
        if args.check:continue
        out.mkdir(parents=True,exist_ok=True)
        temporary=out/(surface+'.md.tmp')
        temporary.write_text(content,encoding='utf-8')
        os.replace(temporary,target)
    print(json.dumps(summary,sort_keys=True))
    return 0

if __name__=='__main__':
    raise SystemExit(main())
