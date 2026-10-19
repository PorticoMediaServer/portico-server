# Glossaire fr-CA — français canadien (Québec)

Partiel : ne contient que ce qui se dit différemment du français de France (`fr`).
166 messages surchargés sur 5 357 ; le reste est hérité de `fr`. État : tout est
`machine`, en attente de révision par un locuteur natif.

## Adresse

`Vous` partout, comme `fr` (convention des applis de diffusion dans les deux
marchés : Netflix, Plex, Apple TV). Impératif deuxième personne du pluriel,
jamais `tu`.

## Termes surchargés (fr → fr-CA)

| Anglais (source) | fr (France) | fr-CA retenu | Unités | Note |
| --- | --- | --- | --- | --- |
| email / email address | e-mail, adresse e-mail | courriel, adresse courriel | ~34 | Terme OQLF, universel au Québec et au Canada français. `vos e-mails` → `vos courriels`, `l’e-mail` → `le courriel`, `d’e-mails` → `de courriels`. |
| tuner(s) | tuner, tuners | syntonisateur(s) | ~47 | Terme OQLF (`syntonisateur HDHomeRun`, `conflit de syntonisateur`, `Rechercher des syntonisateurs sur le réseau`). |
| playlist(s) | playlist(s) | liste(s) de lecture | ~47 | Terme OQLF. Les deux sont féminins, donc aucun article à retoucher (`la playlist` → `la liste de lecture`, `une playlist` → `une liste de lecture`). Étiquettes d’onglets : `Playlists` → `Listes de lecture` (plus long, assumé). |
| streaming | Streaming | diffusion en continu | 3 | Terme OQLF (`Diffusion en continu`, `Limites de diffusion en continu`). Plus long que l’original comme titre de réglage ; assumé. |
| PIN / PIN code | code PIN, codes PIN | NIP | 31 | OQLF (numéro d’identification personnel), masculin comme `code` : `le code PIN` → `le NIP`, `un code PIN` → `un NIP`, `Les codes PIN` → `Les NIP`. |
| weekend | Le week-end | La fin de semaine | 1 | `server.people.days.weekends` ; le changement d’article est inclus. |
| mobile data | données mobiles | données cellulaires | 1 | `downloads.wifiOnlyHelp`. Au Québec, le téléphone mobile se dit `cellulaire`. |

## Termes vérifiés et laissés hérités de `fr`

- Télécharger (download), mot de passe, nom d’utilisateur, sous-titres,
  bande-annonce, télécommande, sauvegarde, paramètres, accueil, se connecter /
  connexion, supprimer, recherche, bibliothèque, épisode, saison : identiques au
  Québec.
- `TV en direct`, `enregistrer / enregistrement` (DVR) : usage commun aux deux
  marchés.
- `pris en charge` (jamais l’anglicisme `supporté`) : déjà correct dans `fr`.
- Aucune chaîne de clavardage, de balado, de magasinage, de stationnement ou de
  téléversement dans le catalogue : rien à québéciser de ce côté.
- Noms de produits intacts : Portico, Portico Account, Portico Services,
  HDHomeRun, M3U, XMLTV, SRT/WebVTT. Clés `locale.*` intouchées.

## Typographie et ponctuation (communes avec `fr`)

- Apostrophe typographique `’` (U+2019) partout, jamais `'` (cit. ICU) — vérifié
  par l’import.
- Insécable normale (U+00A0) devant `; ! ? :` et dans `« … »`, comme le `fr`
  actuel (qui n’emploie pas d’insécable fine U+202F) : aucune divergence fr-CA.
- Points de suspension `…` (un seul caractère), guillemets `« … »` avec
  insécables, `→`/`›`/`·` conservés tels quels depuis `fr`.
- Capitalisation : minuscule aux étiquettes courantes, majuscule en tête de
  phrase et aux titres, comme `fr`. `Diffusion en continu` et `Liste de lecture`
  prennent la majuscule seulement en position de titre.
- Espaces de début/fin conservées à l’identique de la source (vérifié par
  l’import).

## Points à revoir par un locuteur natif

1. `Liste de lecture` contre `playlist` : Spotify/YouTube affichent `playlist`
   même au Québec ; l’OQLF recommande `liste de lecture`. Choix OQLF assumé,
   mais un réviseur pourrait trancher pour `playlist` dans les étiquettes courtes
   (onglets TV).
2. `Diffusion en continu` contre `streaming` : même tension. Tou.tv et
   Radio-Canada emploient `diffusion en continu`.
3. `NIP` contre `code PIN` : `NIP` domine (banques, OQLF), mais `code PIN`
   reste compris ; les applis grand public mélangent les deux.
4. `Syntonisateur` est le terme correct mais peu familier hors contexte
   télé ; les utilisateurs de HDHomeRun le connaissent.
5. La base `fr` elle-même est une traduction automatique non révisée : si elle
   change, les surcharges suivent tant que le texte de base correspond.
