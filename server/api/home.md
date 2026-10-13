# Home and personal continuation

`GET /v1/home` composes the viewer’s rows, exact totals, paging, layout and hero.
Every row declares `seeAll`, an explicit destination:

- `{type:"library",libraryId,pivot,query?,sort:[{field,direction}]}` opens that library’s browse tab with exactly those sort and filter choices.
- `{type:"row",rowId}` opens the full row, paged through `/v1/home/rows/{rowId}`.
- `{type:"saved_view",resourceId}` opens that saved view.

Recently added rows name the library’s works pivot and newest-added sort. Other recommendation and continuation rows name their full row; saved views name the resource. Clients never infer a destination from the row id.

Continue Watching merges unfinished films with one continuation per started show. Films remain until finished or dismissed. Show continuation respects the owner’s age window, with recently added season premieres exempted. Specials do not interrupt an ordinary season. Missing titles never enter the row. A show’s detail Next Up ignores Home age limits and dismissals. Rewatching retains the existing contract: meaningful progress makes the title unwatched until completion.

The item personal-state mutation accepts `continueDismissed: true` with its normal operation id and expected revision. It affects only this profile’s row membership. `false` restores it. A new intentional playback identity makes it eligible again; reports from the dismissed playback do not. Episode artwork inherits from its season and show.

Container watched and unwatched marks supersede older member intent, clear progress, and fence previous playback. The response includes a thirty-second Undo token. `POST /v1/containers/{kind}/{id}/personal-state/undo` with `{expectedRevision,undoToken}` restores the prior watched/progress mix, unless members or their state changed. Marks are bounded to 5,000 members; use individual seasons for larger shows. Undo preserves viewing history, favourites, ratings and saved lists.

Known durations honor the profile’s started percentage. Unknown durations become resumable at thirty seconds and can finish on valid terminal evidence. Audiobook parts use the listening rule: resume after one second and finish near the end. Video completion uses the library’s percentage unless the profile explicitly overrides its played percentage; Credits mode uses an approved marker when one exists and falls back to the percentage otherwise. Early end-of-stream below completion policy keeps the resume point when the title has a known longer duration.

Unchanged paused reports preserve continuation recency and personal revisions. The owner’s history records meaningful plays even while the viewer pauses history; the viewer’s History, personal count and watched/progress writes stop while paused. Shared popularity counts only opted-in, unpaused profiles, with at least three distinct eligible viewers. Recommendation days follow the server’s local date, consistently across devices.
