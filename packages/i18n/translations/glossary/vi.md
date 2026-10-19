# Vietnamese (vi) glossary — machine translation, needs native-speaker review

Every message is recorded as `machine` output. Nothing here has been checked by a native speaker
or tested in the apps. Terms follow the wording of Vietnamese streaming services (Netflix Vietnam,
Disney+ Vietnam, Apple TV in Vietnamese) where they have a settled term.

## Address form and voice

- Second person: **bạn** where the English addresses the viewer directly ("Bạn đã đăng xuất"). It is
  used sparingly. Most UI labels drop the pronoun, as Vietnamese app copy does ("Đăng nhập lại để tiếp
  tục."), which keeps the tone neutral and avoids a fixed level of formality.
- No "anh/chị/em", no honorifics. Owners, members and other people are named by role
  ("chủ máy chủ", "thành viên") or by their name.
- Short imperatives for buttons and actions: "Thử lại", "Xóa", "Lưu".

## Punctuation and typography

- Apostrophes are always `’` (U+2019). The ASCII `'` is never used (ICU quoting).
- Quoted titles keep the English curly quotes `“…”`, as in the source.
- Ellipsis is the single character `…` with no space before it.
- Arrows `→`, hierarchy `›` and middle dots `·` are kept where the source uses them.
- Numbers and units keep a normal space ("8 Mbps", "5 phút"). No non-breaking spaces are inserted.
- Sentence case for labels, buttons and headings, following the English. No Title Case.
- Keyboard keys and product or technical names stay in English: Portico, Portico Account,
  Portico Services, Quick Connect, HDR, Dolby Vision, DTS, Dolby, mpv, FFmpeg, ffprobe, M3U, XMLTV,
  Xtream, HDHomeRun, WebDAV, rclone, NFO, TMDB, TheTVDB, AniList, IMDb, MusicBrainz, OpenSubtitles,
  Cover Art Archive, DLNA, MCP, PIN, QR, API, Wi-Fi, AirPlay, Google Cast, Apple TV, Fire TV,
  Android TV, iPhone, iPad.

## Plurals

- Vietnamese has only the `other` category for whole numbers. Every plural keeps its `one` branch
  (the English requires it) with the same Vietnamese text as `other`, so the import check passes and
  the count is shown with `#`. There is no grammatical plural to agree with, so a noun does not change.
- `=0` exact branches are kept, with their own sentence where the English has one
  (for example "Không có lượt phát sóng sắp tới nào khớp").
- Select keys are unchanged (`{kind, select, movie {…} show {…} other {…}}`).

## Key terms (English → Vietnamese)

### Navigation and screens

| English | Vietnamese | Note |
| --- | --- | --- |
| Home | Trang chủ | Also "Home row" → Hàng trên Trang chủ |
| Search | Tìm kiếm | |
| Settings | Cài đặt | Section paths use the arrow: "Cài đặt › Truy cập từ xa" |
| Preferences | Tùy chọn | Account-level choices; "Cài đặt" is the whole screen |
| Library / Libraries | Thư viện | |
| Library Channels | Kênh thư viện | Feature name; one name everywhere |
| Live TV | TV trực tiếp | |
| Guide (program guide) | Lịch phát sóng | |
| Saved | Đã lưu | Tab and section |
| My List / Watchlist | Danh sách của tôi | Same term for the saved list and its filter |
| Favorites | Yêu thích | |
| Playlists | Danh sách phát | |
| Collections | Bộ sưu tập | |
| Saved views | Chế độ xem đã lưu | |
| History | Lịch sử | |
| Downloads | Tải xuống | Noun for the page and the action; "Xem nội dung tải xuống" for "View downloads" |
| Watch Together | Xem cùng | Group viewing feature |
| Continue Watching | Tiếp tục xem | |
| Up Next | Tiếp theo | |
| Recommended for you | Đề xuất cho bạn | |
| Trending now | Đang thịnh hành | |
| Notifications | Thông báo | |
| Profile | Hồ sơ | |
| Account | Tài khoản | |
| Devices | Thiết bị | |
| Feedback / Report a problem | Báo cáo sự cố | |

### Titles and media

| English | Vietnamese | Note |
| --- | --- | --- |
| Title (a movie, show or album) | Tựa | Used for any title; "Tựa này" = this title |
| Movie | Phim | |
| Show (series in the library) | Chương trình | Navigation and lists |
| TV show | Phim truyền hình | Title kind labels; library kind "Chương trình truyền hình" |
| Season | Mùa | |
| Episode | Tập | |
| Song | Bài hát | |
| Track | Bản nhạc | Track number, disc track |
| Album | Album | |
| Artist | Nghệ sĩ | |
| Audiobook | Sách nói | |
| Book | Sách | Used for book libraries |
| Author | Tác giả | |
| Anime | Anime | |
| Chapter | Chương | |
| Disc | Đĩa | |
| Playlist | Danh sách phát | |
| Collection | Bộ sưu tập | |
| Genre | Thể loại | |
| Cast | Diễn viên | |
| Crew | Đoàn phim | |
| Director / Writer | Đạo diễn / Biên kịch | |
| Credits (cast and crew list) | Danh sách thực hiện | Not the end credits |
| End credits | Phần kết | |
| Synopsis | Tóm tắt | |
| Metadata | Siêu dữ liệu | |
| Artwork / Poster / Backdrop | Ảnh bìa / Poster / Ảnh nền | |
| Rating (score) | Đánh giá | "Your rating" → Đánh giá của bạn |
| Critic rating | Điểm phê bình | |
| Release date | Ngày phát hành | |
| Air date | Ngày phát sóng | |
| Date added | Ngày thêm | |
| Runtime / Duration | Thời lượng | |
| Year | Năm | |
| Resolution | Độ phân giải | |
| Bitrate | Tốc độ bit | |
| File size | Dung lượng tệp | |
| Unmatched | Chưa khớp | Metadata matching |
| Missing | Thiếu | A title whose files are gone |
| Watched | Đã xem | |
| Unwatched / Unplayed | Chưa xem / Chưa phát | "Chưa xem" for titles, "Chưa phát" for play state filters |
| In progress | Đang xem dở | |
| Not started | Chưa bắt đầu | |
| Finished | Đã xong | |
| Play count | Số lượt phát | |

### Playback

| English | Vietnamese | Note |
| --- | --- | --- |
| Play | Phát | Also "Play on" → Phát trên |
| Pause | Tạm dừng | |
| Resume | Tiếp tục | |
| Play next | Phát tiếp theo | |
| Queue | Hàng đợi | |
| Shuffle | Ngẫu nhiên | Button and sort option |
| Repeat | Lặp lại | |
| Speed | Tốc độ | |
| Sleep timer | Hẹn giờ tắt | |
| Subtitles | Phụ đề | |
| Forced subtitles | Phụ đề bắt buộc | Foreign dialogue and signs |
| Audio | Âm thanh | |
| Audio track | Rãnh âm thanh | |
| Quality | Chất lượng | |
| Original (quality) | Gốc | "Chất lượng gốc" in long form |
| Automatic | Tự động | |
| Converting (video) | Chuyển đổi | Noun and verb |
| Direct play | Phát trực tiếp | |
| Repackage | Đóng gói lại | Change the container only |
| Stream | Luồng | |
| Skip intro | Bỏ qua phần giới thiệu | |
| Skip credits | Bỏ qua phần kết | |
| Skip ad | Bỏ qua quảng cáo | |
| Picture in Picture | Hình trong hình | |
| Fullscreen | Toàn màn hình | |
| Mute | Tắt tiếng | |
| Volume | Âm lượng | |
| Still watching? | Bạn còn xem không? | |
| Watch with group | Xem cùng nhóm | |
| Surround sound | Âm thanh vòm | |
| Stereo | Âm thanh nổi | |

### Actions

| English | Vietnamese | Note |
| --- | --- | --- |
| Sign in / Log in | Đăng nhập | |
| Sign out | Đăng xuất | |
| Create account | Tạo tài khoản | |
| Save | Lưu | |
| Done | Xong | |
| Close | Đóng | Closes a window or panel |
| Cancel | Hủy | |
| Discard | Bỏ | Drops unsaved changes |
| Dismiss | Ẩn | Hides a notice; "Đóng" is for windows |
| Back | Quay lại | |
| Next / Previous | Tiếp / Trước | Pager and channel controls |
| Undo | Hoàn tác | |
| Refresh | Làm mới | |
| Try again / Retry | Thử lại | |
| Remove | Xóa | Also used for "Remove from …" lists; see judgement calls |
| Delete | Xóa | |
| Mark as watched | Đánh dấu là đã xem | |
| Mark as unwatched | Đánh dấu là chưa xem | |
| Reset | Đặt lại | |
| Stop | Dừng | Stop playback |
| Start over | Bắt đầu lại | |
| Loading | Đang tải | |
| Scan (library) | Quét | "Rescan" → Quét lại |
| Match (metadata) | Khớp | |
| Lock / Unlock | Khóa / Mở khóa | Metadata fields |
| Record | Ghi | Verb for live TV; "Bản ghi" for recordings |
| Recordings | Bản ghi | |
| Recording rule (series) | Quy tắc ghi hình / quy tắc loạt phim | |
| Tuner | Bộ thu | |
| Channel | Kênh | |

### Server, sharing and security

| English | Vietnamese | Note |
| --- | --- | --- |
| Server | Máy chủ | |
| Server console | Bảng điều khiển máy chủ | |
| Owner | Chủ máy chủ / Chủ sở hữu | "Chủ máy chủ" in sentences, "Chủ sở hữu" in labels |
| Member | Thành viên | |
| Administrator | Quản trị viên | |
| Invitation | Lời mời | |
| Remote access | Truy cập từ xa | |
| Home network | Mạng gia đình | |
| Away from home | Ngoài nhà | |
| Secure connection | Kết nối bảo mật | HTTPS |
| Certificate | Chứng chỉ | |
| Two-step verification | Xác minh hai bước | |
| Recovery code | Mã khôi phục | |
| Authenticator app | Ứng dụng xác thực | |
| API key | Khóa API | |
| Access token | Mã thông báo truy cập | |
| Webhook | Webhook | Kept as is |
| Logs | Nhật ký | |
| Backups | Bản sao lưu | |
| Storage | Bộ nhớ | Disk space; "Dung lượng" for amounts |
| Remote storage | Bộ nhớ từ xa | |
| Background task | Tác vụ nền | |
| Maintenance | Bảo trì | |
| Portico Account | Portico Account | Product name, kept |
| Portico Services | Portico Services | Product name, kept |

## Judgement calls a reviewer should check

- **"Show" vs "TV show".** A show in the library is "chương trình" in navigation, tabs and lists.
  "TV show" is "phim truyền hình" in the title-kind label and "chương trình truyền hình" in the library
  kind names. Check whether one term should be used throughout.
- **"Title" as "tựa".** "Tựa" is used for any title, whether a film, show, album or book. Vietnamese
  apps often say "phim" for all of these, which would be wrong for music and audiobooks.
- **Episode codes.** The compact labels use `M{season} · T{episode}` (Mùa, Tập) instead of `S`/`E`.
  A reviewer may prefer `S1 E2`, which needs no translation. The change is one line in the catalogue.
- **Remove and Delete.** Both are "Xóa". The English separates taking an item off a list (Remove)
  from deleting a file. Vietnamese apps usually do not separate them, so some destructive confirmations
  may need "Xóa vĩnh viễn" to be clearer.
- **Dismiss and Close.** Dismiss is "Ẩn" (hides a notice or suggestion); Close is "Đóng" (a window or
  panel). Check that these read as intended on TV focus labels.
- **Credits.** "Credits" as the cast and crew list is "Danh sách thực hiện"; "credits" at the end of a
  film is "phần kết". Check each use.
- **Owner.** "Chủ máy chủ" in sentences and "Chủ sở hữu" in labels. Vietnamese has no single word
  that covers both an owner and the holder of a server.
- **Watch Together and Watch with group.** Translated as "Xem cùng" and "Xem cùng nhóm".
- **Language names** under `language.*` are translated ("Tiếng Anh", "Tiếng Việt", …). The catalogue
  currently has no `locale.*` keys, so no language name is left in English.
- **Long server-console sentences** (FFmpeg, storage, secure connection, DLNA, OpenSubtitles, assistants,
  scan issues) were translated literally to stay accurate. They are the most likely to read stiffly,
  and need a technical check against the server behaviour.
- **Short labels.** Labels such as "Cài đặt", "Tải xuống" and "Thử lại" were kept to the shortest natural
  term to fit TV focus labels. Long messages were not shortened on purpose.
