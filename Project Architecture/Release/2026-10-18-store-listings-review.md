# Portico app store copy and configuration review

Draft for Justin's review • 18 October 2026

This package covers the public text and proposed configuration for one Apple app spanning iOS/iPadOS and tvOS, one Google Play app spanning mobile and Android TV, and an Amazon Appstore app for Fire TV. The app is free with no in-app purchases, as confirmed by Justin. It includes public copy, discovery fields, support and deletion pages, privacy/rating proposals, beta text and private reviewer instructions. Screenshots and builds are outside this pass. No console fields, identifiers, website pages or app code have been changed.

## 1 Review decisions

| Item | Proposed value or disposition |
| --- | --- |
| Public name | Portico on every store; no separate “Portico for TV” product name |
| Price and purchases | Free; no in-app purchases or subscriptions for this release — confirmed |
| Advertising | No advertising SDKs or Portico-served ads; independent Live TV programmes may contain their own ads |
| Identity | Reserve `tv.getportico.portico` as the common main bundle/package identifier; Apple Top Shelf uses a separate child identifier. Availability and dependent sign-in/app-link configuration must be checked before registration. |
| First distribution | TestFlight on Apple; closed testing on Play; Amazon Live App Testing before public listing. The public descriptions below are for the eventual public release. |
| Primary language | English (US) as the broadly available fallback; use the same neutral English copy in English (Canada) if a separate localization is added |
| Territories | Proposed first public set: Canada, United States, United Kingdom, Australia and New Zealand. Expand after initial feedback; Justin can choose a wider launch. Do not silently enable every storefront. |
| Initial intended audience | Proposed: adults 18+ for the initial testing cohort. Public target audience and actual store-generated content rating are separate decisions; do not force a low rating to match household use. |
| Public support contact | `admin@getportico.tv`, already used by the website; confirm it is monitored for support and tester feedback |
| Store seller | Existing individual accounts identify Justin Ehler. Do not promise the storefront seller name can be changed to “Portico” just by editing listing text. |
| Publication | Manual release/managed publishing after approval; no automatic public launch, pre-order, pre-registration, promotions or custom product pages initially |

**Decisions still for Justin:** tone/name/subtitle; initial countries; public audience positioning; monitored support address; approval of privacy/rating interpretations; EU trader classification if EU distribution is added. A home address and personal phone are not included here; public legal-contact details require a separate decision where the consoles require them.

### Public positioning

Lead with a personal media library across devices. Explain the separate server requirement plainly. Keep “My List” as the visible save-for-later name. Feature claims concern the current source, not promises of every codec/device working. Live TV/DVR copy requires a configured source. The antenna-tuner example is concrete and avoids implying that Portico supplies channels. Keep protocol/provider jargon out of discovery copy; do not conceal actual supported functions from reviewers.

Do not advertise universal codec support, guaranteed 4K/HDR/Dolby/Atmos, DRM-service playback, included content, a Portico media relay, downloads on TVs, Chromecast/Alexa integration, or every accessibility feature. Those claims need their own product acceptance. Offline downloads and Watch Together exist in source but are not included in the base public descriptions until accepted for the candidate. Optional lines after acceptance: “Download titles to watch offline on your phone or tablet.” and “Watch together in sync with people on your server.” Never add the download line to TV copy. A listing may remain concise without enumerating every feature.

Apple's public App Store is not the distribution channel for a build that is still a beta; use TestFlight for that stage. The public copy must describe an accepted public-ready version, rather than concealing a testing build behind polished wording. [Apple review guidelines](https://developer.apple.com/app-store/review/guidelines/).

## 2 Apple public listing

App name, subtitle and privacy information are shared at app level. Descriptions, promotional text and keywords can be maintained for each platform localization. The proposed subtitle is a store metadata field, not an instruction to add a decorative subtitle in the app UI.

### App name

7 / 30 characters. 

```text
Portico
```

### Subtitle

26 / 30 characters. 

```text
Your own streaming service
```

### Promotional text for both platforms

150 / 170 characters. 

```text
Every feature is free, with no paid tier or subscription. Stream movies, shows, music and audiobooks from your own Portico Server across your devices.
```

### Keywords for both platforms

92 / 100 bytes. 

```text
movies,shows,music,audiobooks,player,library,server,live tv,dvr,nas,home,video,antenna,tuner
```

### iPhone and iPad description

1733 / 4000 characters. 

```text
Your media, at home on every screen.

Portico turns the movies, shows, music and audiobooks on your server into your own streaming service. Every feature in this release is free, with no paid tier or subscription.

A library that feels like yours
Explore artwork, seasons, albums and collections. Search across the libraries you can access and keep the titles you want to return to in My List.

Your place, wherever you play
Your server profile keeps your watch and listening progress. Start on one device and continue on another connected to the same server and profile.

More than movies
Listen to music and audiobooks, choose available audio and subtitle tracks, and watch Live TV or recordings from an antenna tuner configured on your server.

Room for the household
Each person can have their own profile, progress, preferences and library access. The server owner decides who can connect and what they can use.

A direct connection to your server
Your app connects directly to the Portico Server you choose. Portico's account service helps coordinate sign-in and connections; it does not host or relay your media streams.

Before you begin
You need access to a running Portico Server, either your own or one you have been invited to use. Portico Server runs on Windows, macOS, Linux and Docker. Your server must be reachable to stream. Watching away from home needs a server that can be reached from the internet; Portico can set up the connection on supported networks.

Portico does not include movies, shows, music, audiobooks or television channels. Use it with media and services you are authorized to access. Playback depends on your media, device, network and server.

Get Portico Server: https://getportico.tv/download/
```

### Apple TV description

1422 / 4000 characters. 

```text
Your library belongs on the big screen.

Portico turns the movies, shows, music and audiobooks on your server into your own streaming service. Every feature in this release is free, with no paid tier or subscription.

Find your next watch
Browse artwork, seasons and collections, search your libraries and keep titles for later in My List.

Pick up where you left off
Continue watching or listening with the progress saved to your server profile. Each person can keep their own place and library access.

Watch and listen your way
Choose available audio and subtitle tracks. Enjoy Live TV and recordings when compatible sources are configured on your server.

Connect directly
Your Apple TV streams from the Portico Server you choose. Portico's account service coordinates access without hosting or relaying your media streams.

Before you begin
You need access to a running Portico Server, either your own or one you have been invited to use. Portico Server runs on Windows, macOS, Linux and Docker. The server must be reachable. Watching away from home needs a server that can be reached from the internet; Portico can set up the connection on supported networks.

Portico does not include movies, shows, music, audiobooks or television channels. Use it with media and services you are authorized to access. Playback depends on your media, device, network and server.

Get Portico Server: https://getportico.tv/download/
```

### Other Apple fields

| Field | Proposed entry |
| --- | --- |
| Primary category | Entertainment |
| Secondary category | Music |
| Marketing URL | `https://getportico.tv/` |
| Support URL | `https://getportico.tv/support/` — proposed new page; ready-to-publish text in section 7. Use the existing docs URL only as an interim review reference. |
| Privacy policy URL | `https://getportico.tv/privacy/` — existing public page; browser-verified during this pass |
| Privacy choices URL | `https://getportico.tv/account-deletion/` — proposed new page, not yet deployed |
| Copyright | `2026 Justin Ehler` |
| SKU | `PORTICO-UNIVERSAL` — internal, not public |
| Bundle ID | `tv.getportico.portico`, shared across iOS and tvOS, subject to registration |
| Platforms | iOS/iPadOS and tvOS. Explicitly deselect Mac (Designed for iPad) and Apple Vision Pro in availability; eligible iPad/iPhone apps may otherwise be available there by default |
| License | Proposed Apple standard EULA, subject to checking compatibility with the release's own and bundled component licences. Website terms govern hosted services; they do not replace the standard app EULA automatically. |
| Kids category | No |
| Game Center / health / medical status | Not applicable to Portico's proposed categories |
| Price / pre-order | Free; pre-orders off |
| Sign in with Apple | Configure the new primary App ID and its relationship to the existing web Services ID, permitted native client IDs and return URLs. Update Hosted audience configuration and associated-domain/app-link identifiers together before testing sign-in. No credential values belong in this file. |
| IAP, subscriptions and server notifications | No products or payment links; no IAP metadata to submit for this free release |
| Version | Propose 1.0 for the first public release; beta build identifiers remain separate; confirm during build preparation |
| What’s New | Not entered for the first Apple version. Later updates need accurate change notes rather than reusing launch copy. |
| Apple TV privacy policy text | Paste the complete plain-text policy in appendix A; a URL alone is not the tvOS field |
| Accessibility labels | No affirmative support claims until the relevant criteria are verified for each platform; source props alone do not qualify |

Apple field limits were checked against [app information](https://developer.apple.com/help/app-store-connect/reference/app-information/app-information/) and [platform version information](https://developer.apple.com/help/app-store-connect/reference/app-information/platform-version-information/). Keywords are checked as UTF-8 bytes; this draft uses ASCII.

## 3 Google Play public listing

One app record serves phones, tablets, Android TV and Google TV. TV can use its dedicated track without creating a second app identity. This keeps the closed-test/production-access process attached to one app; it does not waive TV quality review. [Android TV distribution](https://developer.android.com/training/tv/publishing/distribute).

### App name

7 / 30 characters. 

```text
Portico
```

### Short description

67 / 80 characters. 

```text
Your own streaming service for movies, shows, music and audiobooks.
```

### Full description

1739 / 4000 characters. 

```text
Your media, at home on every screen.

Portico turns the movies, shows, music and audiobooks on your server into your own streaming service. Every feature in this release is free, with no paid tier or subscription.

FIND SOMETHING YOU LOVE
Browse artwork, seasons, albums and collections. Search the libraries you can access and keep titles for later in My List.

KEEP YOUR PLACE
Your server profile keeps your watch and listening progress, preferences and library access. Continue on another device connected to the same server and profile.

WATCH, LISTEN AND TUNE IN
Play movies and shows, listen to music and audiobooks, and choose available audio and subtitle tracks. Watch Live TV and recordings from an antenna tuner configured on your server.

AT HOME ON ANDROID TV
Use your remote to browse and play on Android TV and Google TV. The same Portico app also supports phones and tablets.

YOUR SERVER STAYS IN CHARGE
The server owner controls the library and who can access it. Your app connects directly to that server; Portico's account service coordinates sign-in and connections without hosting or relaying your media streams.

BEFORE YOU BEGIN
You need access to a running Portico Server, either your own or one you have been invited to use. Portico Server runs on Windows, macOS, Linux and Docker. Your server must be reachable to stream. Watching away from home needs a server that can be reached from the internet; Portico can set up the connection on supported networks.

Portico does not include movies, shows, music, audiobooks or television channels. Use it with media and services you are authorized to access. Playback depends on your media, device, network and server.

Get Portico Server: https://getportico.tv/download/
```

### Other Google fields

| Field | Proposed entry |
| --- | --- |
| Application or game | App |
| Category | Video Players & Editors |
| Tags | Video player; Media player; Music & Audio; Entertainment; TV — candidates only; select up to five relevant labels actually offered by Play, not invented free-text tags |
| Default language | English (United States) |
| Developer website | `https://getportico.tv/` |
| Support email | `admin@getportico.tv` |
| Support phone | Leave the optional public support number blank; verified account contact details remain separate |
| Privacy policy | `https://getportico.tv/privacy/` |
| Account deletion URL | `https://getportico.tv/account-deletion/` after the proposed page is published |
| Contains ads | No: Portico does not serve ads or include an advertising SDK. Broadcaster ads inside user-provided Live TV are not a Portico ad placement; do not advertise “ad-free television.” |
| Pricing | Free; no subscriptions or in-app purchases |
| Listing experiments / custom listings / promotional content | None initially |
| App access | All or some functionality restricted: supply a reachable reviewer server and dedicated credentials; add separate Hosted-account instructions |
| Target audience | Proposed 18+ for initial testing and first public listing, pending Justin’s review. Not designed for children / no Families enrollment. This does not dictate the calculated content rating. |
| Account creation | Yes: Portico Account flows exist. Direct local server accounts are separate. |
| Account deletion | Mobile has an in-app flow; TV currently has a generic account-management handoff. Add a clearly labelled deletion entry on TV before treating that path as submission-ready; see the Apple TV prerequisite below. |
| Other data deletion without deleting account | Proposed Yes through the support route described in section 7, once that support process is operational |
| Data safety | Use section 6; never select “No data collected” just because media bypasses Hosted |
| Advertising ID | No known advertising-ID use; check the final merged manifest and SDK set before submission |
| Sensitive/high-risk permissions | No broad storage, SMS, contacts, location, microphone or camera access requested by the current main manifest. Playback/download foreground services require their actual declarations. |
| News / government / finance / health / gambling / loans | No such app function; do not infer a news-app category just because a user supplies a TV news channel |
| Foreground service declaration | Media playback for user-started music/audiobooks; data sync for user-started downloads. Provide required demonstration material later if the console requires it; this copy pass does not fabricate a video. |
| Android TV opt-in | Advanced settings → Form factors → add Android TV, then its dedicated review/track. No second Google app record. |
| Play App Signing | Enroll the new app and configure its upload key before first upload; do not reuse the debug signing key. Signing setup remains build preparation. |
| Closed-test countries | Proposed same five initial markets as the public draft. Configure the testing track’s country availability explicitly; tester invitations alone do not set this. |
| IARC contact | `admin@getportico.tv` as the private monitored rating contact, subject to mailbox confirmation |
| Old Play drafts | Leave both old August drafts unpublished. Identify them as retired only when the new app record is established; no deletion or renaming in this review pass. |
| Managed publishing | Enabled before any production submission; closed-test publication remains a separate deliberate action |

Google name/description limits: [store listing setup](https://support.google.com/googleplay/android-developer/answer/9859152?hl=en). Current Android source targets SDK 36 with minimum API 24; compatible devices are ultimately derived from the accepted binary, not manually promised in the description.

## 4 Amazon Fire TV public listing

This is the Fire TV listing, not a promise of Fire tablet or general Android support through Amazon. Fire OS and Vega OS have separate binaries and compatibility requirements. Do not select Vega devices solely because the Android Fire TV flavor builds; Vega needs its own accepted package. Amazon console state has not been inspected in this pass. Its metadata policy forbids price information; the listing uses “every feature included” and leaves the actual Free price to the store setting rather than putting price slogans in metadata. [Amazon listing policy](https://developer.amazon.com/docs/policy-center/listing-promo.html).

### Display title

7 / 30 characters. 

```text
Portico
```

### Short description

151 / 200 characters. The 200-character short-copy budget is our conservative Fire TV presentation target, not Amazon’s full field limit.

```text
Your movies, shows, music and audiobooks on Fire TV, with every feature included. Requires a running Portico Server. No media or channels are included.
```

### Long description

1454 / 4000 characters. 

```text
Your library belongs on the big screen.

Portico turns the movies, shows, music and audiobooks on your server into your own streaming service. Every feature in this release is included, with no paid tier or subscription.

Browse your collection
Explore artwork, seasons and collections. Search your libraries and keep the titles you want to return to in My List.

Keep your place
Continue watching or listening with progress saved to your server profile. Each person can have their own progress, preferences and library access.

More ways to enjoy your library
Choose available audio and subtitle tracks. Watch Live TV and recordings from an antenna tuner configured on your server.

A direct connection
Your Fire TV streams directly from your chosen Portico Server. Portico's account service coordinates access without hosting or relaying your media streams.

Before you begin
You need access to a running Portico Server, either your own or one you have been invited to use. Portico Server runs on Windows, macOS, Linux and Docker. The server must be reachable. Watching away from home needs a server that can be reached from the internet; Portico can set up the connection on supported networks.

Portico does not include movies, shows, music, audiobooks or television channels. Use it with media and services you are authorized to access. Playback depends on your media, device, network and server.

Get Portico Server: https://getportico.tv/download/
```

### Product feature bullets

286 characters. 

```text
Browse movies, shows, music and audiobooks from your Portico Server
Use an interface designed for your TV remote
Keep progress and My List with your server profile
Watch Live TV and recordings from a configured antenna tuner
Connect directly to your server without a Portico media relay
```

### Search keywords

101 characters. 

```text
personal media,media player,media server,movies,television,music,audiobooks,live tv,dvr,antenna,tuner
```

### Other Amazon fields

| Field | Proposed entry |
| --- | --- |
| App title / display title | Portico |
| Public developer name | Justin Ehler, consistent with the existing individual Apple/Google seller and website identity. Confirm the Amazon account before entry. |
| Category | Entertainment; choose an available media/video subcategory where offered |
| Price | Free; no IAP or subscriptions |
| Device targeting | Compatible Fire TV devices only. Exclude tablets/other platforms and unqualified OS families. |
| Support website / email | Proposed `https://getportico.tv/support/`; `admin@getportico.tv` |
| Privacy URL / account deletion URL | Same proposed URLs as Apple and Google |
| Collects/transfers user data | Yes; complete Amazon's own privacy questions using section 6, not a blind copy of Play's flags |
| Account creation / deletion | Yes, including creation outside the TV app; browser-based account deletion and support route |
| Data-only deletion | Proposed Yes via the operational support request process |
| Content rating | Complete the media-player/provider and interactive-feature questions truthfully; section 5 supplies the proposed factual answers |
| DRM option | No added Amazon DRM wrapper; preserve Portico’s own access controls and all bundled licence obligations. |
| Release | Live App Testing first; no immediate public submission or automatic expansion to every device/country |

Amazon's short description is limited to 2,000 bytes (approximately 1,200 English characters); Fire TV shows its first 200 characters prominently. Long descriptions allow 4,000 characters and are plain text; use three to five feature bullets. The short description above states the server requirement within that initial view. [Appstore details](https://developer.amazon.com/docs/app-submission/appstore-details.html).

## 5 Content ratings and declarations

These are proposed factual questionnaire responses, not a preselected Apple, IARC or Amazon age-rating badge. Complete each store's exact questionnaire and accept its calculated regional results. Do not answer every content descriptor “None” merely because the app ships without a content catalogue.

| Topic | Proposed response and boundary |
| --- | --- |
| App purpose | General-purpose personal media client, not a game, public video catalogue, IPTV subscription seller or piracy service |
| Children | Not made specifically for children. Household profiles are not a child-directed product declaration. Proposed initial testers are adults. |
| Parental/content controls | Yes where the questionnaire describes account/library/content-rating restrictions: server administrators can limit profile access. This is not verified age assurance. |
| Age verification / age assurance | No verified age-assurance system is established by a profile PIN or content-rating cap |
| Unrestricted web browsing | No: a Portico server address and an authentication session are not a general web browser. |
| Public UGC / social network | No public uploads/feed/catalogue operated by Portico. Users do supply and share private server media; disclose that feature where the question includes private media sharing. |
| Messaging/chat | Messaging/Chat: No; no general chat is present in the reviewed source. Users interact: Yes where IARC asks about shared server features such as Watch Together or playlists. Neither is a public social feed. |
| User media and mature descriptors | Private server media can contain mature content. Use the questionnaire's media-player/provider treatment; describe owner-controlled content and profile restrictions. An exact rating requires completing the current branching questions. |
| Gambling, contests, purchases, medical advice | No such built-in function |
| Ads | No Portico-served ads or advertising SDK. A broadcaster’s ads inside a user-configured programme are not an app advertising placement. |
| Third-party content rights | The app accesses third-party/user-selected content. Do not certify “no third-party content.” Only attest to rights held or otherwise permitted; require a rights-cleared review fixture and public artwork before submission. |
| Permissions | Network discovery is for servers/TVs; selected-photo access is for avatars; background playback/download services perform user-requested media work. No claim of broad photo-library or microphone access. |

Apple definitions distinguish public UGC/social media, messaging, web access, parental controls and age assurance. These distinctions matter for this personal-server app. [Age-rating definitions](https://developer.apple.com/help/app-store-connect/reference/app-information/age-ratings-values-and-definitions/).

## 6 Privacy declarations for review

### Data destinations

The current client has two distinct destinations. Portico's Hosted service handles optional Portico Accounts, identity, server memberships and connection coordination. A chosen media server handles library queries, searches, playback/progress, preferences and authorized administration. An independently run server is not the same recipient as Portico Hosted. Device-only data is a third case.

This proposal does not claim a final binary/SDK data-flow audit. It supplies the factual mapping and conservative answers to review before entering each console. A privacy manifest with an empty collected-data array is not evidence that account service requests collect nothing.

| Data or activity | Recipient and use | Draft store treatment |
| --- | --- | --- |
| Account name, username, email and account ID | Hosted for account creation, sign-in, recovery and membership; selected server for local accounts | Declare name/email/user identifiers for account functionality and security. Linked to identity; not tracking. Email is required for flows that require it, not all local-server use. |
| Profile photo | Optional selected upload to the relevant account/server service | Photos or videos → Photos; optional, app functionality/personalization; identity-linked when stored with profile. No broad photo-library scan. |
| Device/installation/session IDs, device name and version, network address | Hosted and selected server for trusted sessions, routing and security | Device/other identifiers plus relevant diagnostics/other data. Linked when tied to account/session. Do not equate an IP address with location collection unless location is actually derived. |
| Connection and security errors / operational records | Hosted for abuse prevention and service operation; server diagnostics belong to its operator | Apple: App Functionality covers security, uptime and crash/performance minimization. Play/Amazon may classify reliability evaluation as Analytics under their own definitions; no advertising analysis is proposed. |
| Search queries, viewed items, play progress, saved titles, ratings and playlists | Selected media server for searching, resume, recommendations and user preferences | Play/Amazon: app interactions, in-app search history, other actions/other user content as applicable. Apple: not collected by Portico where sent only to an independent server operator, with no developer/partner access. Play/Amazon disclosures use their broader rules. If a developer-operated public demo receives these records, include that actual collection separately. |
| Media, subtitles and offline files | Primarily server-to-device playback/download; optional user-selected subtitle/avatar uploads can be device-to-server | Receiving a stream/download is not itself uploading the user's media collection. Declare actual uploads under their relevant photo/file/content categories. No broad device-file inventory. |
| Optional client crash/diagnostic report | User-initiated report sent to the selected server owner | Play/Amazon: optional crash logs and diagnostics where their collection definitions apply. Apple: not developer-collected if only an independent owner can access it. |
| Optional feedback/support attachments | User-initiated support channel | Customer support/other user content where the store's collection rules apply; optional, support/functionality, not advertising |
| Exact location, contacts, calendar, SMS, health, payment, advertising ID | No matching app collection/permissions found in reviewed source | Proposed No for those data types; keep financial purchasing fields No for this no-IAP release |

### Apple proposed answers

- Collect data: **Yes**. Portico Services retain account and connection information beyond a realtime request.
- Proposed data types: Contact Info → Name and Email Address; User Content → Photos for optional avatars; Identifiers → User ID and Device ID; Diagnostics → Other Diagnostic Data for retained service security/connection records where applicable. Stored network addresses should use the suitable identifier/other-data category, not location unless location is actually derived.
- Linked to identity: **Yes** for account/profile/session records. Used for tracking: **No**.
- Purposes: **App Functionality** for authentication, account features, fraud/security, service uptime, crash minimization and performance. **Analytics: No** and **Product Personalization: No** for the current Portico Services scope. Recommendations and progress on a private media server are not personalization performed by Portico Services.
- Data sent solely to an independently operated server, without Portico or its integrated partners having access, does not meet Apple's developer/partner collection definition. This includes optional reports sent only to that server's owner. Reclassify actual developer-operated public demo or support collection if offered; do not hide it behind the independent-server rule.
- Photos and voluntary support are optional; account identifiers are required for the relevant account flow. No advertising purpose or ATT tracking prompt is proposed.
- The complete tvOS privacy policy text is in appendix A. It uses the current redesign's “Portico Services” terminology and must match the deployed URL before submission.

[Apple privacy definitions](https://developer.apple.com/app-store/app-privacy-details/) distinguish developer/partner access from independent server use and include reliability/security within App Functionality. These answers still need final binary/SDK reconciliation.

### Google proposed answers

- Collects or shares required data types: **Yes**. Include off-device flows through the app and its libraries, not only calls to Portico-owned infrastructure.
- Collection optionality: optional Portico Account/profile photo/feedback flows are identified separately; identifiers and required server-session work are necessary for the selected function.
- Sharing: proposed **No** for the reviewed flows: service-provider processing and the server the person explicitly chose use the relevant provider/user-directed-transfer exceptions. Collection remains **Yes**. Recheck any new recipient or automatic onward disclosure rather than generalizing this answer to future integrations.
- **All collected user data encrypted in transit: proposed No for the current configuration**, unless final verification establishes that every in-scope personal-data route is encrypted. Android explicitly permits owner-selected plain HTTP server connections. Verified server identity alone does not encrypt HTTP. Hosted routes use HTTPS, but that does not justify a universal encryption claim.
- Account deletion available: **Yes**, after the public page in section 7 is deployed and its route/support workflow is verified. Data-only requests: proposed **Yes** through the stated support process.
- Independent security review / payment verification badges: no affirmative claim.

[Google Data safety definitions](https://support.google.com/googleplay/android-developer/answer/10787469?hl=en) cover collection, transfer exceptions and encryption; [account deletion requirements](https://support.google.com/googleplay/android-developer/answer/13327111?hl=en) require an external resource as well as the applicable in-app path.

### Amazon proposed answers

- Collects user data or transfers user data: **Yes**; name/email/user IDs, optional profile photos, device/network identifiers, diagnostics and actual selected-server interaction data as mapped above.
- Use App Functionality and Security purposes; add Personalization or Analytics only for actual corresponding uses. No advertising purposes.
- Apply Amazon's own provider/user-initiated-transfer exceptions; don't copy Google's “shared” values mechanically.
- Account creation: **Yes**, including creation outside the TV app. Provide the account-deletion page; data-only request support is proposed Yes once operational.

[Amazon privacy labels](https://developer.amazon.com/docs/app-submission/appstore-privacy-labels.html) and [account/data deletion](https://developer.amazon.com/docs/app-submission/user-account-and-data-deletion.html) use their own questionnaire definitions.

## 7 Public support and account deletion pages

These are proposed new public website pages, not existing deployed endpoints. Their full text is ready for review. Site implementation/publication is a separate step after approval. Use `admin@getportico.tv` only if it will be monitored; no response-time promise is invented.

### Proposed support page at /support/

667 characters. 

```text
Portico support

Need help connecting, browsing or playing your media?

Start with the setup and troubleshooting guides at https://getportico.tv/docs/.

For help with the app or a Portico Account, email admin@getportico.tv. Include your device, operating system, Portico version and a short description of what happened. Never send passwords, access tokens or private media files.

For access to a particular server, missing libraries, local accounts or server permissions, contact that server's administrator.

Privacy: https://getportico.tv/privacy/
Account and data deletion: https://getportico.tv/account-deletion/
Copyright and abuse reports: abuse@getportico.tv
```

### Proposed account deletion page at /account-deletion/

1560 characters. 

```text
Delete your Portico Account or request data deletion

You can request deletion even if you have uninstalled Portico.

To delete a Portico Account, open https://web.getportico.tv/account?tab=security in a browser, sign in and find Delete your Portico Account. Choose Delete account… Review the effects on your server memberships and profiles, enter the confirmation requested and confirm deletion. If the page explains that server ownership must be resolved first, follow its instructions or contact us for help.

You can also email admin@getportico.tv with the subject "Portico account deletion" or "Portico data deletion". We may need to verify that you own the account. Do not send your password or recovery codes. Use this route if you cannot sign in, want help with deletion, or want to request removal of particular information without deleting your account.

Account deletion removes the Portico Account and revokes its active access. Related account and coordination data are deleted or anonymized as described in our privacy policy. Some security, legal and recovery records may need to remain for the limited purposes and retention periods described there; deletion is not a promise to erase every backup immediately.

A Portico Account is separate from an account on an independently operated Portico Server. Contact that server's administrator about local accounts, playback history, logs or files held there. Deleting a Portico Account does not delete media files from independently operated servers.

Privacy policy: https://getportico.tv/privacy/
```

### Destination details to verify before the pages go live

The hosted web source has deletion at `/account?tab=security`: “Delete your Portico Account” → “Delete account…”. Native mobile has a Delete Portico Account action under the account settings. The native TV view instead hands account management to the website. Confirm the exact displayed labels and the browser URL in the final version, and retain an uninstall-independent support route. Deletion must be distinguished from sign-out, temporary deactivation or deleting media. Server ownership and pending disposition can change the confirmation flow, so the page follows the preview instead of promising unconditional immediate completion.

Current website privacy text permits deletion by contacting `admin@getportico.tv` and describes bounded retention with security/legal exceptions. Do not promise all data is erased immediately, all backups disappear in 30 days, or a Portico Account deletion deletes independently owned server media. Confirm the data-only support process is staffed before selecting that optional store response.

### Short privacy explanation for help or listing-adjacent text

402 characters. This supplements the full policy; it is not the tvOS policy field.

```text
Portico connects directly to the server you choose. Your media streams and ordinary library activity do not pass through Portico's hosted account service. If you use a Portico Account, we process account and connection information needed to provide and protect the service. Your server administrator controls information stored on that server. Read the privacy policy at https://getportico.tv/privacy/.
```

## 8 Beta pages and release text

Use beta wording only on TestFlight, Play testing materials, Amazon testing invitations and an explicitly labelled testing download page. Production-store descriptions in sections 2–4 do not claim that an alpha is already production-ready.

### Beta app description and testing invitation

672 characters. 

```text
Try Portico with your own media server on phones, tablets and televisions.

This is an early testing release. You need access to a running Portico Server; no movies, music or television channels are included.

Please try sign-in and server selection, browsing and search, My List, playback, audio and subtitles, music and audiobook progress, and switching between your devices. If your server has Live TV and DVR configured, try those too.

Send feedback through TestFlight where available, or email admin@getportico.tv. Include your device, operating system, app version and the steps that led to the problem. Please do not send passwords, access tokens or private media.
```

### What to test

485 characters. 

```text
Please test:
- Sign-in, server selection and profile switching.
- Browsing, search and My List.
- Starting, pausing, seeking and resuming playback.
- Audio and subtitle selection.
- Music and audiobook progress, including background listening on mobile.
- Navigation and playback with a TV remote.
- Live TV and recordings, if configured on your server.

Please report the device, OS version, app version and steps to reproduce any problem. Do not include credentials or private media.
```

### Google closed-test release notes

401 / 500 characters. 

```text
First Portico testing release for Android phones, tablets and TV.

Connect to a Portico Server to browse movies, shows, music and audiobooks, save titles to My List and continue playback with your server profile. Live TV and recordings require compatible sources on the server.

This is an early testing build. Please send issues and feedback to admin@getportico.tv. No media or channels are included.
```

### First public release notes for Google and Amazon

232 / 500 characters. 

```text
Introducing Portico: browse and play media from your Portico Server, keep titles in My List and continue with your server profile. Includes movies, shows, music, audiobooks, and Live TV and recordings when configured on your server.
```

**Beta App Review:** provide the same private sign-in details and reviewer notes as public App Review, plus the beta feedback email and privacy URL. External TestFlight testing can require review.

Feedback email: `admin@getportico.tv`. TestFlight privacy and marketing URLs use the same approved public destinations. No tester addresses or private feedback are included here. Do not claim the twelve-person Play requirement is already satisfied; current drafts show zero opted-in closed testers. TestFlight and Amazon testing are separate programmes, not substitutes for Play's process.

### Public download-page cards

| Card | Text | Initial action |
| --- | --- | --- |
| iPhone and iPad | Your Portico library on the go. Browse, watch and listen with your own server. | Join the TestFlight beta — only once its approved invitation URL exists |
| Apple TV | Your library on the big screen, with navigation made for your remote. | Join the TestFlight beta — only once tvOS enrollment is available |
| Android and Google TV | One Portico app for your phone, tablet and television. Connect to your own server and keep your place. | Join the Android test — once the actual closed-test opt-in link is issued |
| Fire TV | Stream from your Portico Server with an experience made for your TV remote. | Join the Fire TV test — once an Amazon testing route exists |

Page notice: “Portico is currently available as an early testing release. You need access to a Portico Server. No media or channels are included.” Public-release buttons later become “Download on the App Store,” “Get it on Google Play” and the approved Amazon badge. Do not invent product URLs before store records generate them. Keep server installers separate from these client download cards.

## 9 Private reviewer and compliance configuration

The following text is for console review fields, not a public product page. Credentials must be inserted only through secure private fields at submission; this file contains none. The existing demo is an orientation resource, not a guarantee of reviewer readiness. Create and verify dedicated non-expiring review access when builds are ready.

### Proposed App Review and app-access notes

1596 / 4000 bytes. Below Apple’s 4,000-byte review-note ceiling; adapt the final labels and credentials per platform.

```text
Portico is a client for independently operated Portico media servers. It does not sell, supply or unlock a catalogue of films, television, music or live channels. There are no in-app purchases or subscriptions in this release.

For review, use the dedicated reviewer server and account supplied in the private sign-in fields. No account creation, payment or personal server is required for this review path.

1. Choose Sign in directly to a server. On TV, enter the supplied Server address, choose Continue, then Use a password instead.
2. On phone or tablet, use the direct server sign-in form and supplied server address.
3. Enter the supplied viewer username and password, then select the reviewer profile.
4. Open the Movies and TV libraries. Play a fixture, pause, seek, resume and try the available audio/subtitle tracks.
5. Open Music and Audiobooks to test listening and saved progress. On TV, use the directional remote and select button.

The review server's playable sample files must be original, licensed or public-domain fixtures supplied for testing. Poster artwork alone is not evidence of licensed playable content. Ordinary media requests go directly to the chosen server.

For account creation/deletion review, use the separate Portico Account review credentials and instructions supplied privately. The direct server account above is a separate identity and cannot demonstrate Portico Account deletion.

Review contact: Justin Ehler, admin@getportico.tv. The contact telephone number is supplied privately in the console. Please contact us if any test resource is unavailable.
```

| Submission field | Proposed handling |
| --- | --- |
| Reviewer contact | Justin Ehler; `admin@getportico.tv`; private current telephone number entered by Justin/secure field |
| Sign-in required | Yes; provide viewer server credentials. Add a separate Hosted account for account creation/deletion testing. |
| Reviewer URL | Dedicated HTTPS server with rights-cleared playable fixtures; no IP allowlist, expiring invite, required purchase or mandatory personal hardware |
| TV sign-in | Sign in directly to a server → Server address → Continue → Use a password instead → supplied username/password. Verify these exact labels in the candidate; a reachable direct review route avoids requiring another device for ordinary playback review. |
| Live TV / DVR | Describe configured review sources only if rights-cleared and actually available. Otherwise identify the dependency and an alternate way to review core playback. |
| Third-party rights | Verify playable media, metadata, artwork, logos and public brand assets; never equate fictional/synthetic title matching with rights to use commercial posters publicly |
| Encryption/export | Encryption is used. Do not answer “no encryption” or set a non-exempt flag by guesswork. Apple source includes CryptoKit/AES-GCM and secure TV approval beyond ordinary HTTPS; classify actual algorithms, usage and bundled libraries before selecting exemptions/documentation. |
| EU trader/legal status | Justin determines trader classification and approves any legally required public address/contact details. Initial proposed countries exclude the EU; do not infer non-trader from the app being free. |
| Apple agreements | Console inspection showed an update notice while developer history showed acceptance on 18 October; reconcile current status before submission, without accepting terms as part of this draft |
| App privacy/SDK manifests | Reconcile the final archived app and third-party required-reason/collected-data manifests with the approved public answers; an empty current source array does not settle them |
| Permissions demonstrations | Prepare any required foreground-service or restricted-permission demonstration after builds; no fake approval claim in the listing |
| Store-specific questions | Answer the exact live wording. Content rating, crypto exemption, privacy taxonomy and signed legal attestations remain submission decisions, not automatic consequences of approving the marketing copy. |

### Apple TV deletion prerequisite

The current tvOS source can create an account through native Sign in with Apple, but its settings hide the mobile Delete Account row and provide only generic account management. Before Apple TV submission, add an explicit “Delete your Portico Account” action that starts the deletion path or gives a clearly directed QR/URL handoff to `/account?tab=security`. Validate the actual complete flow. A wording change in this document cannot supply that missing product affordance. This source finding is outside this copy-only change; no native code has been altered. [Apple account deletion guidance](https://developer.apple.com/support/offering-account-deletion-in-your-app/).

## 10 Non-screenshot graphics and optional fields

No screenshots are requested or produced. Existing brand assets can supply app icons, banners and store feature graphics. This section defines the creative/configuration choices; exports still need the current store dimensions and format checks.

- **App icons:** use the existing white Portico Play-P on the navy background, from the approved brand masters. No beta ribbon, price text, third-party show artwork or platform logo inside the icon. Use Apple's platform-specific icon asset process for iOS and Apple TV, and store-ready icon exports for Google/Amazon.
- **Google feature graphic:** proposed wording “Your own streaming service” with the Portico wordmark on navy. Keep it product-focused; no “best,” “#1,” pricing badge or borrowed film artwork. Export as 1024 × 500 JPEG or opaque PNG. Use the existing 512 × 512 square brand master for the separate Play icon, with store-format validation.
- **TV banners and Amazon app icon:** use the approved Portico mark on navy. Amazon’s required Fire TV app icon is 1280 × 720 PNG without transparency; keep the mark inside its safe area. Android’s launcher banner remains a separate build asset.
- **Amazon background image:** required 1920 × 1080 JPG or opaque PNG. It must not embed a logo or title. Choose original, rights-cleared imagery relevant to the app, with the visual focus toward the right; do not reuse the wordmark icon as the background. Creative approval and an export remain outstanding. Optional Amazon promotional art can be omitted initially.
- **Videos / Apple app previews / YouTube trailer:** leave optional video fields empty for this first listing. Required policy demonstration videos are private reviewer material, not marketing previews.
- **Featured nominations, in-app events, promotional offers, custom pages and A/B experiments:** none initially. No unused or misleading placeholder copy.

## 11 Final review questions

1. Does “Your own streaming service” fit the store positioning, or should we keep the longer website line “Your media, at home on every screen” only in descriptions?
2. Are Canada, US, UK, Australia and New Zealand the right first public markets?
3. Is `admin@getportico.tv` the public support/feedback mailbox, or should `support@getportico.tv` be established as a forwarding alias? The current copy keeps the existing address until a working alias is confirmed.
4. Should the initial public audience be adults while the beta settles, or a broader general audience with the appropriate content ratings? This choice does not change the truth of rating questionnaire answers.
5. Are the main selling points the right ones: every feature included without a paid tier or subscription, your own library, separate household progress, one account/profile across devices, direct server connections and optional Live TV/DVR?

Approving this draft authorizes neither console publication nor accepting legal agreements. Build-derived fields, exact privacy/rating attestations and private review access are deliberately identified above so they can be completed accurately when the candidate is ready.

## Appendix A Apple TV privacy policy text

Plain-text rendering of the current website redesign policy follows, using “Portico Services” throughout. The redesign policy must be approved and deployed before this text is entered in a console; the currently deployed older page has different terminology. It is included in full because Apple TV requires a text field. Do not publish a shortened privacy promise in that field. If approved privacy practices or policy wording change, update the web policy, this text and the store disclosures together.

```text
Privacy

Your media activity stays between your devices and the Portico Server you choose. Portico Services coordinate access, but they aren't part of the media path.

Last updated: August 25, 2026

Who this policy covers

This policy covers information processed by the Portico public website, Portico applications, and Portico-hosted account and connection services (“Portico Services”).

Each independently operated Portico Server has its own administrator and its own data practices. The server owner, not Portico, controls the media, local accounts, permissions, server logs and other information held on that server.

Public website

The public website doesn't use advertising, tracking pixels, account sessions, or analytics that follow you across the web.

When you visit, our delivery and hosting systems must process ordinary request information to return the page and protect the service. This includes the requested address, the time, browser information, your network address and security signals. We don't use that information to build advertising profiles.

Portico Account information

If you use Portico Services, we process the information needed to provide and protect your account. This can include:

- your email address

- your profile name and image

- authentication and recovery information

- account settings

- active sessions

- registered devices

- security events

- records of your requests to us

We don't store passwords and comparable secrets as readable account records. We store them using appropriate one-way or encrypted protections.

Processed profile images are kept in private object storage run by our infrastructure provider, separate from Portico's account database. After an authenticated request, Portico authorizes access to an image for a limited period. The storage bucket itself isn't made public.

Server coordination information

Portico Services process the limited account and connection information needed to connect accounts and servers. This can include:

- a server identifier and public identity key

- ownership and membership records

- invitations and revocations

- signed policy and route information

- connection reachability and health state

- certificate coordination state

- software version and device descriptors

- timestamps

- the public network address observed when a server contacts Portico Services

This information tells Portico Services how to authenticate and coordinate a server. It doesn't give access to that server's media library.

Media and streaming stay private

Portico Services don't host, proxy, tunnel, relay, cache, transform, inspect, meter or store any of the following: video, audio, Live TV, DVR recordings, artwork, subtitles or downloads. They don't receive ordinary library browsing, playback, progress, remote-control or server API traffic.

Portico Services don't collect:

- library metadata

- titles

- filenames

- filesystem paths

- private source addresses

- playback history

- watchlists

- ratings

- queues

- local searches

- stream contents

- streaming details

Your client connects directly to the Portico Server, so Portico Services aren't technically in a position to monitor that activity.

Account and connection-coordination requests still carry the limited technical information described above. Your media traffic and media activity don't pass through Portico Services.

A future relay

Portico doesn't currently provide a media relay. If we introduce an optional relay, it's intended to carry end-to-end encrypted connections that Portico can't decrypt or inspect.

A relay may need to process limited connection, security and traffic-volume information to operate. It won't be designed to reveal stream contents, titles, library information or playback details. We'll publish updated privacy information specific to that feature before it becomes available.

Diagnostics and communications

Portico may process operational logs, error details, request identifiers, network addresses, security signals and aggregate service measurements. We need these to keep hosted services secure and reliable. We keep sensitive information in logs to a minimum, and we don't use operator tools to browse private servers.

If you contact support or report abuse, we process the messages, attachments and contact details you provide. We use them to respond and to keep an appropriate record of the matter.

How we use information

We use information to:

- provide and secure Portico Accounts

- coordinate connections

- maintain memberships and invitations

- deliver service messages

- diagnose failures

- prevent fraud and abuse

- improve reliability

- respond to requests

- enforce our terms

- comply with lawful obligations

We don't sell personal information or use it for targeted advertising.

When information is shared

We share information only as reasonably needed:

- with service providers that help operate Portico, such as infrastructure, content delivery, email, security and support providers

- with a server owner when needed to administer a membership or address a service issue

- when you direct us to share it

- when required by valid legal process, or when necessary to protect people, Portico or its services

Providers receive only the information appropriate to their role and are expected to protect it.

If Portico's ownership or operations change, relevant information may be transferred as part of that transaction, subject to continued protection and applicable law.

Abuse and legal requests

We review reports sent to abuse@getportico.tv in good faith. When necessary to investigate or to allow a response, we may share a report, including the reporter's contact information, with the relevant account or server operator. We won't do this if law, safety or the circumstances call for a different approach. Our Abuse page (https://getportico.tv/abuse/) explains the process and how to request a correction.

Portico responds to valid legal process. We may preserve or disclose information where the law requires it. We can't provide media or media activity that Portico Services don't have.

Retention

Our goal is to keep personal and operational information for as short a time as practical while keeping the service secure and usable.

Active account, membership, session and current connection information stays available while it's needed for those functions. It isn't deleted just because it was created more than 30 days ago.

Once information is no longer needed for its stated purpose, we delete or anonymize it as soon as practical, and ordinarily no later than 30 days afterward. A longer period may apply when:

- the information is still needed for security, service recovery, resolving a dispute, or another purpose described in this policy, or

- Portico is required or compelled by law to preserve it.

Encrypted backups rotate on a separate, limited recovery schedule. A deleted value may remain, inaccessible, in a backup until that backup expires.

When you replace or remove a profile image, Portico stops referencing the previous image immediately and schedules it for deletion. Temporary delivery links expire automatically. If the storage provider is temporarily unavailable, we retry the deletion. The image stays subject to the maximum post-purpose retention period above, unless the law requires us to preserve it.

Where feasible, we keep transient network information for shorter periods. Raw or hashed network addresses, stale endpoint observations, rate-limit state and cached connection-retry state are ordinarily kept for about one or two days or less after they stop being operationally useful. Portico doesn't keep them permanently just for convenience.

Routine operational audit events are generally kept for seven days. Security and support audit records may be kept longer while they're still needed for those purposes. Request-network fingerprints are removed from them earlier.

Your choices

You can review and change the available profile, account, membership, device and server settings in Portico. You can also ask for access to, correction of, or deletion of your personal information by emailing admin@getportico.tv. We may need to verify the request. Some information may be kept where it's required for security, legal compliance or another permitted reason.

Server owners manage the information stored on their own Portico Servers. Send requests about a server's media, local accounts, permissions, playback records or logs to that server's owner.

Security

Portico uses technical and organizational safeguards appropriate to the information it processes. These include encrypted transport, limited access, credential protection and data minimization. No system can guarantee absolute security. Please use strong account protection and report suspected compromise promptly.

International processing

Portico and its service providers may process information in more than one country. Where required, we use appropriate safeguards for information transferred across borders. We apply this policy to the information in our care.

Changes and contact

We may update this policy as Portico develops. When we do, the date above will change. Where practicable, we'll give reasonable notice of material changes.

Send privacy and security questions to admin@getportico.tv. Send copyright and abuse reports to abuse@getportico.tv.
```

## Appendix B Evidence and field validation

Copy incorporates the current website redesign’s “Your own streaming service” positioning and no-paid-tier message, the native main source and the completed launch-remediation review. Opus 5.5 reviewed the draft; its substantive corrections are incorporated. This is editorial/configuration review, not a legal attestation or a signed build acceptance. It avoids promising platform acceptance that the source does not establish. Current client source has mobile-only account deletion controls, optional photo picking, media playback/download services, and explicit Android HTTP-route support. The private server and Hosted privacy distinction is retained throughout.

Official field/rule references are linked beside the relevant proposed settings. Local sources for reconciliation:

- Website main legal/public sources plus `.worktrees/getportico-tv-redesign/src/pages/index.astro` and `privacy.astro` for the current proposed positioning and privacy terminology. The redesign remains separate from the deployed older website.
- `apps/portico-react-native/apps/native/src/screens/settings/SettingsPage.tsx`, `DeleteAccount.tsx`, `Together.tsx`, `android/app/src/main/AndroidManifest.xml`, `android/app/src/main/res/xml/network_security_config.xml`, `Native/PorticoNetwork.swift` and platform `PrivacyInfo.xcprivacy` files.
- `apps/portico-server/web/src/screens/account/Security.tsx`.
- The 18 October authenticated console inspection: two old Play drafts with uploaded 0.1.0 bundles, zero closed-test participants, no default listings; Apple identifiers present but no App Store Connect app records.

Apple/Google bounded fields and short-copy budgets are machine-checked in the accompanying copy JSON. Amazon's exact display-title/keyword field ceilings should be confirmed in the current console; the proposed name is seven characters and keywords are deliberately compact. The deployed older public privacy page was opened successfully in Chrome’s Portico profile. It is not yet the redesign policy reproduced in appendix A. `/support/` and `/account-deletion/` remain proposed new pages. No console, source app, identifier or website changes were made for this review package.
