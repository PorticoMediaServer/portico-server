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

class OverrideError(Exception):
    '''A closed refusal from the published override source.'''
    def __init__(self,code,subject=''):
        super().__init__(code)
        self.code=code;self.subject=subject

def load_overrides():
    '''Load the published per-surface override source, refusing anything malformed.

    A missing or malformed file stops list generation instead of silently
    resurrecting a mapping the audit already denied. Every record needs a known
    surface, a row id, an include or exclude action, the audited frame, a reason
    and caption evidence from the mock-up; duplicate surface+id records are
    refused rather than resolved by order.
    '''
    if not OVERRIDES.is_file():raise OverrideError('missing')
    try:document=json.loads(OVERRIDES.read_text(encoding='utf-8'))
    except (OSError,ValueError):raise OverrideError('unreadable')
    if not isinstance(document,dict) or document.get('schema')!=1:raise OverrideError('schema')
    records=document.get('overrides')
    if not isinstance(records,list):raise OverrideError('schema')
    loaded={surface:{} for surface in SURFACES}
    for index,record in enumerate(records):
        if not isinstance(record,dict):raise OverrideError('record','#%d'%index)
        surface,identifier=record.get('surface'),record.get('id')
        if surface not in SURFACES:raise OverrideError('surface','#%d'%index)
        if not isinstance(identifier,str) or not identifier.strip():raise OverrideError('record','#%d'%index)
        subject=surface+':'+identifier
        if record.get('action') not in ('include','exclude'):raise OverrideError('action',subject)
        frame,reason=record.get('frame'),record.get('reason')
        if not isinstance(frame,str) or not frame.strip():raise OverrideError('frame',subject)
        if not isinstance(reason,str) or not reason.strip():raise OverrideError('reason',subject)
        evidence=record.get('evidence')
        if not isinstance(evidence,dict):raise OverrideError('evidence',subject)
        for key in ('file','label','caption'):
            if not isinstance(evidence.get(key),str) or not evidence[key].strip():
                raise OverrideError('evidence',subject)
        if identifier in loaded[surface]:raise OverrideError('duplicate',subject)
        loaded[surface][identifier]={'action':record['action'],'frame':frame.strip(),'reason':reason.strip(),
            'file':evidence['file'].strip(),'label':evidence['label'].strip(),'caption':evidence['caption'].strip()}
    return loaded

def validate_overrides(loaded,indexed):
    '''Every record must name a real in-scope row and that row's own frame group.'''
    for surface in SURFACES:
        for identifier,record in loaded[surface].items():
            row=indexed.get(identifier)
            subject=surface+':'+identifier
            if row is None:raise OverrideError('unknown-id',subject)
            if row['group']!=record['frame']:raise OverrideError('frame-mismatch',subject)

def audit_notes(surface, indexed, applied):
    '''One line per applied record, including rows this surface no longer carries.'''
    lines=[]
    for identifier in sorted(applied):
        record=applied[identifier]
        if identifier not in indexed:continue
        lines.append('- `%s` %s `%s` — %s · %s %s' % (identifier,record['action'],record['frame'],
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
        '`flock /tmp/portico-v3-claims.lock` by adding one line under "## Claims". Use the preferred',
        'entry and current cutoff in `walks/SERVER.md` and the current build in `walks/BUILDS.md`.',
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
    body=text[match.end():]
    # Claims end at the next top-level heading, so later sections such as audit
    # notes never leak into the claim body on the next regeneration.
    stop=re.search(r'(?m)^## ',body)
    if stop:body=body[:stop.start()]
    body=body.strip('\n')
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
    indexed={row['id']:row for row in rows}
    try:
        applied_by_surface=load_overrides();validate_overrides(applied_by_surface,indexed)
    except OverrideError as error:
        print('lists: overrides-%s %s'%(error.code,error.subject),file=sys.stderr)
        return 2
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
        notes=audit_notes(surface,indexed,applied) if applied else []
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
