# Portico web experiment: brief shared by both builds

This is the web version of the app experiment in `BRIEF.md`, which you should read too. Justin
wants to see whether a from-scratch Portico web frontend, designed by you, beats the current
one. Two builds, by two builders, in the same two themes as the apps:

- **web-classic** (`experiments/web-classic`): the standard, Plex-like interpretation (Plex Web,
  Jellyfin, Emby), done excellently. Pair it with `experiments/classic`.
- **web-cinema** (`experiments/web-cinema`): the creative interpretation, drawing on the web apps
  of the paid streaming services (tv.apple.com, Netflix, Disney+, Max, Prime Video). Pair it with
  `experiments/cinema`.

Each must be a **fully featured frontend that could replace the current web UI**. It runs on
fixture data, but every feature of the current web UI has a real, working page or interaction.
The code must be good enough that swapping the fixture layer for real Portico services would
make it shippable.

**Leave the current web UI untouched.** Don't edit anything under `apps/`. Read it only.

## Hard rules

1. **Portico's signature blue glow.** A navy canvas with a blue projection glow falling from the
   top left:
   - `ink` #071018;
   - glow stops `ambient` #153749 and `raisedBright` #1d3848;
   - Portico blue `accent` #409fd4, `accentStrong` #5cb3e2, `accentDeep` #2f86bb.

   The reference is `apps/portico-react-native/apps/apple/src/ui/Canvas.tsx`, and the web's own
   version is in `apps/portico-server/web/src`. You may evolve the glow; keep it recognisable.
2. **The real Portico logos only.**
   - Location: `/Users/justin/Documents/Codex Projects/Portico (Original Retired Project - GPT 5.5 and 5.6)/portico-internal/Brand Assets/`.
     Read its README.
   - What to use: the Play-P symbol (`svg/portico-symbol-dark.svg` and the mono-white variant)
     and the full wordmark (`svg/portico-wordmark-dark.svg` and the mono-white variant), plus
     the favicon.
   - Copy them into your project. Don't recolour the play triangle, stretch the artwork or add
     effects.
3. **Typography.** Portico's brand face is **Manrope**, self-hosted, at weights 400–700. Classic
   uses Manrope. Cinema may pair Manrope with one stronger OFL display sans for headlines. No
   serif or novelty display faces; Justin disliked the serif used in the Cinema app. Self-host
   the font files; no Google Fonts CDN at runtime.
4. **Navigation:**
   - Desktop and laptop: a left rail or sidebar (the web counterpart of the TV rail).
   - Tablet and phone widths: collapse it into a sensible pattern (a bottom bar, a drawer, or
     similar).
   - The whole app is responsive down to 375 px wide.
5. **Stack:** the same as the current web app, so the result could replace it:
   - Vite 8, React 19.2 and TypeScript 7;
   - TanStack Router for routes and URL state;
   - Radix UI primitives where useful (dialogs, menus, popovers, sliders, tooltips);
   - hls.js is optional; fixture playback can be simulated.

   Keep dependencies lean and justified. Use CSS modules or a single token-driven CSS approach;
   no Tailwind unless you have a strong reason.
6. **Accessibility and input:** full keyboard navigation with visible focus, sensible ARIA
   (Radix helps), reduced-motion support, and working hover, focus and pressed states.

## Feature completeness: the current web UI is the reference

Inventory `apps/portico-server/web/src` completely:
- every route (grep `path:` under `app/` and `shell/`);
- every folder under `screens/` (auth, home, library, detail, people, search, saved, channels,
  live, downloads, together, account, settings, server, and the shared dialogs and actions);
- `player/` (the player, timeline, options, queue list, lyrics, mini guide, live captions, Play
  on, post-play, skip prompts, group pill);
- `shell/`;
- `admin/`.

Also read its string catalogue (`apps/portico-server/packages/i18n/src/catalog/en-US.ts`); it
surfaces features the route list hides.

Write `FEATURES.md` as a checklist: every current-web feature, and where your build covers it.
The **server administration** area (every `/server/$section` page) is included and must be
complete and believable. Tick every row before you finish.

Improve on the current web freely: layout, information architecture, flows. Keep features; don't
drop any.

## Fixture data and artwork

- The same populated household as the apps:
  - several profiles, one of them a kids profile;
  - two or three servers, one of them unavailable;
  - every media type (movies, TV, anime, music, audiobooks, photos, home videos);
  - live TV with a full guide, DVR, collections, playlists and people.
- **Reuse the artwork** already gathered and licensed for the paired app. Copy what you need:
  - web-classic from `experiments/classic/assets`;
  - web-cinema from `experiments/cinema/assets`;
  - their `scripts/art` show the sources.

  Generate web-appropriate sizes (for example WebP) with a script. Serve everything locally, with
  no network needed at runtime.
- Put all data behind a service or repository interface, typed after the real shapes in
  `apps/portico-server/packages/client-core/src`, so real services can replace the fixtures.
- Simulate latency and loading states so skeletons and transitions are real. Make them toggleable,
  for example `?latency=0`.

## Quality bar

Production-ready and extremely refined on every page, which is the lesson from the app round.
Before calling anything done, review every page yourself with headless screenshots at:
- 1440×900;
- 1920×1080;
- 1024×768;
- 390×844.

Look at each as a design lead shipping it would, and fix what you find. Check:
- alignment and spacing rhythm;
- type hierarchy, truncation and wrapping;
- image cropping and empty boxes;
- contrast;
- focus rings;
- consistency between similar pages;
- loading, empty and error states;
- placeholder-sounding copy and fixture grammar;
- hover and motion.

Use **Playwright** for screenshots, headless Chromium. Its browsers are already in
`~/Library/Caches/ms-playwright`, so add `@playwright/test` or `playwright` as a dev dependency.
Keep a script (`npm run shots`) that screenshots every route at those sizes into a folder, and
delete old shots as you go.

## Working rules

- **Your own folder** (`experiments/web-classic` or `experiments/web-cinema`), as its own git
  repository:
  - `git init`, then commit **often**: at least every meaningful step, and never more than about
    45 minutes of uncommitted work. A pause can come at any time.
  - Plain commit messages. No AI or assistant attribution. No AI model or agent names in code,
    file names or commits.
  - Never push.
- **Dev server ports:** web-classic 5401 (preview 5411); web-cinema 5402 (preview 5412). Stop
  servers you started, by PID, when you finish.
- **Scratch files:** under `experiments/<your folder>/.scratch/`, git-ignored. Nothing in any
  shared scratch folder.
- **No approval prompts.** Justin is away. Don't use:
  - `osascript`, `open -a`, `sudo`, `defaults write` or keychain commands;
  - simulator tools;
  - the in-app browser tools.

  Guard every `rm` variable as `"${VAR:?}"`, or use plain relative paths inside your folder. If
  something asks for approval, find another way.
- **Disk:** about 75 GB free. Keep `node_modules` to one install, and screenshots and generated
  images lean.

## Done means

- `npm run build` succeeds with no type errors. `npm run preview` serves the production build.
- Every row in `FEATURES.md` is ticked. Every route works at all four sizes with no console
  errors: have the shots script fail on console errors.
- `README.md` covers:
  - the design idea;
  - architecture (routing, design tokens, components, data layer);
  - how to swap in real services;
  - how to run it;
  - decisions;
  - a "Status and next steps" section, kept current as you go.
- A screen index page (for example `/dev/screens`) links to every route and state, for review.
- All work committed, and servers stopped.
