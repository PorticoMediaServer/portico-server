#!/usr/bin/env python3
"""Re-sync the schema digest and known-object list after an installer change."""
import re,subprocess,pathlib
root=pathlib.Path(__file__).resolve().parent.parent/'server'
p=root/'internal/persistence'
for _ in range(3):
    out=subprocess.run(['go','test','./internal/persistence/','-run','Schema|KnownSchema'],cwd=root,capture_output=True,text=True).stdout
    changed=False
    m=re.search(r'set schemaSourceDigest in this file to:\s+([0-9a-f]{64})',out)
    if m:
        t=(p/'schema_test.go');s=t.read_text();s=re.sub(r'const schemaSourceDigest = "[0-9a-f]{64}"','const schemaSourceDigest = "%s"'%m.group(1),s);t.write_text(s);changed=True
    m=re.search(r'with the contents of:\s+(\S+known_schema.txt)',out)
    if m:
        new=pathlib.Path(m.group(1)).read_text().rstrip('\n')
        t=(p/'known_schema.go');s=t.read_text();s=re.sub(r'const knownSchemaObjects = `[^`]*`','const knownSchemaObjects = `'+new+'`',s,count=1);t.write_text(s);changed=True
    if not changed:
        print(out[-600:]);break
