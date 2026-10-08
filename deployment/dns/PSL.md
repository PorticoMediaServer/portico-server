# Public Suffix List submission draft

Submit to publicsuffix/list, PRIVATE DOMAINS section, after Justin verifies domain control. This file does not submit the request.

Title: Add direct.getportico.tv for isolated Portico server identities

Requested entry:

```
// Portico : https://getportico.tv
// Submitted by: Justin Ehler (use the maintainer's verified address)
direct.getportico.tv
```

Portico assigns an independent random namespace beneath this domain to each self-hosted server identity. Tenants have separate owners, authentication, cookies and TLS keys. Each certificate covers `*.<namespace>.direct.getportico.tv`; its resolvable hosts include an independently random, 128-bit members-only label. Certificate Transparency reveals the namespace but not the label or household address. The legacy `current.<namespace>` name returns NXDOMAIN. Public DNS delegation for ACME remains deliberately public.

The requested boundary makes `<namespace>.direct.getportico.tv` the registrable domain: one tenant cannot set cookies for another tenant through the shared parent. The application must still use host-only cookies; PSL inclusion does not replace authentication or Origin checks.

Submission checklist: supply verified maintainer contact; publish the PSL-requested `_psl.direct.getportico.tv` TXT record pointing at the actual pull request; show two independently owned example namespaces; attach DNS/TLS evidence without publishing member labels or home addresses; remove the proof TXT only when maintainers permit. Never include root or signing private keys.

LAN name: `l-<member label>.<namespace>.direct.getportico.tv`, resolving only the owner's explicitly shared private addresses. WAN name: `c-<member label>.<namespace>.direct.getportico.tv`. Both fit the existing wildcard. DNS rebinding protections can block private DNS answers; clients should report that condition rather than disable router protections automatically.
