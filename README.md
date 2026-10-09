# portico-internal

Private Portico services, operations and architecture.

For current Ubuntu builds, visible Android/Fire TV/Vega devices and the populated demo VPS, start with [interactive testing](deployment/testing/README.md).

| Path | Contents |
|---|---|
| `hosted-services/hosted` | The Hosted API and Hosted web host (Go, PostgreSQL) |
| `hosted-services/dns` | The DNS responder for `direct.getportico.tv` and the ACME zone |
| `hosted-services/apikit` | Copy of portico-server's `apikit`; check with `scripts/shared-apikit.sh` |
| `cast-receiver` | How the shared Google Cast receiver is published |
| `deployment` | Host configuration and runbooks: `hosted/TOPOLOGY.md` first |
| `scripts` | Release, deploy, backup and test scripts |
| `Project Architecture` | How Portico is built; start with its README. `Recommendation Enhancement/` holds the title dataset's plans |
| `Product Contracts` | What the product must do |
| `Brand Assets` | Logos and marks |
| `Deployment Information` | Operator records; secrets in it are ignored by git |
| `experiments` | The September frontend experiments on fixture data: `classic` and `cinema` (iPhone, iPad, Apple TV), `web-classic`, `web-cinema` and `web-glass`; briefs in `BRIEF.md` and `WEB-BRIEF.md` |

Go tests run on the runner: `scripts/runner-test.sh`. Several scripts use the portico-server checkout beside this one (`../apps/portico-server`, or `PORTICO_SERVER_REPO`).

## Checkout layout

The scripts assume this layout under one projects folder (on the project server, `/home/justin/projects/Portico`):

```
Portico/
  apps/portico-server/          public: server, web app, shared packages
  apps/portico-react-native/    public: iPhone, iPad, Apple TV, Android, Android TV, Fire TV
  apps/portico-roku/            public: Roku (reads the two above as siblings)
  portico-internal/             this repository
  portico-recommendation-enhancement/   private: the title dataset (data files: its DATA-FILES.md)
  getportico-tv/                public: the website
  .private/                     local only: keys and old operator captures; never in Git
```
