
### Continue Watching dismissal

The existing item personal-state mutation accepts `continueDismissed: true`
(with its normal `operationId` and `expectedRevision`). It dismisses only that
profile's item; it does not mark the item watched or alter saved progress.
`false` restores it explicitly. A new playback identity automatically makes the
item eligible again; later progress reports from the dismissed playback do not.
Continue Watching and Up Next declare `artworkShape: "poster"`; episode artwork
falls back from its own selection to the season and then the show.
