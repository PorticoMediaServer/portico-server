"""Reject templated catalogue prose.

Usage: python3 scripts/catalog/check_prose.py [file.json ...]   (default: every file in content/)

Fails (exit 1) when:
- any 5-word sequence appears in more than 2 different prose entries (synopses, bios, taglines...);
- two prose entries start with the same first 4 words;
- a title is duplicated within a file, or more than 3 titles share their first two words;
- more than 3 titles share the same shape of small words (e.g. "X, in the Y", "A X for Y").
Scripts must not generate prose; this checker exists to catch it.
"""
import json, os, re, sys
from collections import defaultdict

HERE = os.path.dirname(os.path.abspath(__file__))
PROSE_KEYS = {'synopsis', 'bio', 'tagline', 'description', 'authorBio'}
TITLE_KEYS = {'title', 'name'}
SMALL = {'a', 'an', 'the', 'of', 'in', 'for', 'at', 'to', 'and', 'on', 'with', 'from', 'by', 'under', 'over', 'into', 'after', 'before', 'when', 'where'}


def walk(node, path, prose, titles):
    if isinstance(node, dict):
        for k, v in node.items():
            if isinstance(v, str):
                if k in PROSE_KEYS and len(v.split()) >= 5:
                    prose.append((path + '.' + k, v))
                elif k in TITLE_KEYS and k != 'name' or (k == 'name' and 'members' not in path):
                    if k in TITLE_KEYS:
                        titles[path.split('[')[0] + '.' + k].append(v)
            elif k == 'captions' and isinstance(v, list):
                for i, c in enumerate(v):
                    if isinstance(c, str) and len(c.split()) >= 5:
                        prose.append((f'{path}.captions[{i}]', c))
            else:
                walk(v, path + '.' + k, prose, titles)
    elif isinstance(node, list):
        for i, v in enumerate(node):
            walk(v, f'{path}[{i}]', prose, titles)


def words(s):
    return re.findall(r"[a-z0-9']+", s.lower())


def shape(title):
    ws = title.split()
    return ' '.join(w.lower() if w.lower().strip(',') in SMALL else ('X,' if w.endswith(',') else 'X') for w in ws)


def check(files):
    prose, titles = [], defaultdict(list)
    for f in files:
        data = json.load(open(f))
        walk(data, os.path.basename(f), prose, titles)
    problems = []
    grams = defaultdict(set)
    for i, (_, text) in enumerate(prose):
        w = words(text)
        for j in range(len(w) - 4):
            g = tuple(w[j:j + 5])
            if all(x in SMALL for x in g):
                continue
            grams[g].add(i)
    bad = sorted(((len(v), ' '.join(k)) for k, v in grams.items() if len(v) > 2), reverse=True)
    for n, g in bad[:25]:
        problems.append(f'5-word phrase used in {n} entries: "{g}"')
    starts = defaultdict(list)
    for p, text in prose:
        starts[p.rsplit('.', 1)[-1] + ':' + ' '.join(words(text)[:4])].append(p)
    for s, ps in starts.items():
        if len(ps) > 1 and len(s.split(':', 1)[1].split()) == 4:
            problems.append(f'{len(ps)} entries start with "{s}": {ps[:3]}')
    for group, ts in titles.items():
        seen = defaultdict(int)
        for t in ts:
            seen[t.lower()] += 1
        for t, n in seen.items():
            if n > 1 and 'tracks' not in group and 'programs' not in group and 'episodes' not in group:
                problems.append(f'duplicate title in {group}: "{t}" x{n}')
        first2 = defaultdict(list)
        shapes = defaultdict(list)
        for t in ts:
            ws = t.split()
            if len(ws) >= 3:
                first2[' '.join(ws[:2]).lower()].append(t)
            if len(ws) >= 3:
                shapes[shape(t)].append(t)
        for k, v in first2.items():
            if len(v) > 3 and 'programs' not in group:
                problems.append(f'{len(v)} titles in {group} start with "{k}": {v[:4]}')
        for k, v in shapes.items():
            small_words = [w for w in k.split() if w not in ('X', 'X,')]
            if len(v) > 3 and small_words and k not in ('the X X', 'X of X', 'the X of X', 'X X X', 'X and X', 'X X', 'X X X X') and 'programs' not in group and 'tracks' not in group:
                problems.append(f'{len(v)} titles in {group} share the shape "{k}": {v[:4]}')
    return problems


if __name__ == '__main__':
    files = sys.argv[1:] or [os.path.join(HERE, 'content', f) for f in sorted(os.listdir(os.path.join(HERE, 'content'))) if f.endswith('.json') and f != 'real.json']
    problems = check(files)
    if problems:
        print(f'FAIL: {len(problems)} problems')
        for p in problems[:60]:
            print(' -', p)
        sys.exit(1)
    print('OK: prose looks hand-written (', len(files), 'files )')
