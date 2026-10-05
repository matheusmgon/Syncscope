#!/usr/bin/env python3
"""Regenerates internal/argocd/api.go (the API interface + Unsupported base)
from the exported methods of argocd.Client. Run from the repo root after
adding methods to the client:  python3 scripts/gen-argocd-api.py && gofmt -w internal/argocd/api.go
"""
import glob, re

def balanced(s, i):
    depth = 0
    for j in range(i, len(s)):
        if s[j] == '(':
            depth += 1
        elif s[j] == ')':
            depth -= 1
            if depth == 0:
                return j + 1
    raise ValueError('unbalanced parentheses')

methods = []
for f in sorted(glob.glob('internal/argocd/*.go')):
    if f.endswith('_test.go') or f.endswith('api.go'):
        continue
    src = open(f).read()
    for m in re.finditer(r'^func \(c \*Client\) ([A-Z]\w*)\(', src, re.M):
        start = m.end() - 1
        end = balanced(src, start)
        methods.append((m.group(1), src[start + 1:end - 1], src[end:src.index('{', end)].strip()))

ZERO = {'error': 'ErrUnsupported', 'string': '""', 'bool': 'false', 'int': '0', 'int64': '0', 'Credentials': 'Credentials{}'}
def zero(t):
    t = t.strip()
    if t in ZERO:
        return ZERO[t]
    if t.startswith(('*', '[]', 'map[')):
        return 'nil'
    return t + '{}'

out = ['// Code generated from the methods of Client by scripts/gen-argocd-api.py; DO NOT EDIT.', '', 'package argocd', '',
       'import (\n\t"context"\n\t"errors"\n)', '',
       '// ErrUnsupported is returned by back-ends that cannot perform an operation',
       '// (core mode talks to Kubernetes directly, without an Argo CD API server).',
       'var ErrUnsupported = errors.New("not available in core mode (needs an Argo CD API server)")', '',
       '// API is everything Syncscope needs from an Argo CD back-end.', 'type API interface {']
out += [f'\t{n}({p}) {r}'.rstrip() for n, p, r in methods]
out += ['}', '', 'var _ API = (*Client)(nil)', '', '// Unsupported implements API with ErrUnsupported; embed it and override what works.', 'type Unsupported struct{}', '']
for n, p, r in methods:
    if not r:
        body = ''
    elif r.startswith('('):
        body = 'return ' + ', '.join(zero(x) for x in r[1:-1].split(','))
    else:
        body = 'return ' + zero(r)
    out.append(f'func (Unsupported) {n}({p}) {r} {{ {body} }}')
open('internal/argocd/api.go', 'w').write('\n'.join(out) + '\n')
