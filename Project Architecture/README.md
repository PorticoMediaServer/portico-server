# Project Architecture

How Portico is built, reconciled with the October programme reports. Implementation and external acceptance are distinguished in each chapter. [Product Contracts](../Product%20Contracts/) say what the product must do.

1. [Overview](01%20Overview.md): repositories, components, principles
2. [Server](02%20Server.md): process, state folder, database, API, identity, networking
3. [Catalogue and Metadata](03%20Catalogue%20and%20Metadata.md): entities, derived data, read paths, providers
4. [Playback and Media](04%20Playback%20and%20Media.md): Playback Protocol v1, delivery, music, decoder confinement, Live TV, casting
5. [Recommendations](05%20Recommendations.md): the engine, surfaces, the title enrichment dataset
6. [Clients](06%20Clients.md): shared packages, web, Apple, Android/Fire and Vega
7. [Hosted Services](07%20Hosted%20Services.md): accounts, directory, claim, DNS, certificates
8. [Operations](08%20Operations.md): four-platform packages, restart/restore, reports, releases and testing

[Specs](Specs/): product behaviour and specialised designs. Current generated contracts own exact wire schemas; historical examples are not alternate APIs.

[Backlog](Backlog.md): known open work.

Keep these current: when a change alters what a document says, update the document in the same change.
