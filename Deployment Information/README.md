# Deployment Information

Operator records that aren't code.

- **`Hosted Services/`** holds the real secrets and recovery material: provider API credentials (Cloudflare `.env` files), VPS provider recovery codes, and PKI material. It stays on this machine: `.gitignore` excludes it, and it's never committed or published.
- **How the hosts are set up and operated** lives with the deployment files, not here:
  - `../deployment/hosted/TOPOLOGY.md`: hosts, units, ports, firewall, directories
  - `../deployment/hosted/KEYS.md`: key custody and rotation
  - `../deployment/hosted/BACKUPS.md` and `RUNBOOK.md`: backups, restore, incidents
  - `../Project Architecture/08 Operations.md`: releases, demo deploys, testing

Earlier documents from this folder (the pre-23 September hosting design, release runbooks from the retired build) are in `___BACKUP COPIES/Portico pre-split 2026-09-25/deployment-information-docs.tar.gz`.
