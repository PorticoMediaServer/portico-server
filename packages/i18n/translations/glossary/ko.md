# Korean (ko) glossary — machine translation, needs native review

Address form: polite 해요체 for full sentences (‑어요/‑예요 endings, e.g.
“저장됐어요”, “다시 시도하세요”). Imperatives use the soft 해요체 request
form (‑세요). Buttons, tabs, menu items and labels use concise noun forms
without endings (설정, 추가, 삭제, 재생목록). No honorific customer language
(‑시‑) except fixed strings like “환영해요”. “You” is usually dropped, as is
natural in Korean UI copy.

Punctuation and typography: standard Korean spacing (띄어쓰기); sentence-final
period kept as in English (“저장됐어요.”). Ellipsis … kept as in source.
Middle dot · and arrow › kept. Curly quotes “ ” kept; ASCII ' never used.
Percent/units attach directly (100%, 4K, 1080p, 10MB, Mbps). Ranges use ~ or
– as in source. Numbers stay Arabic numerals.

Plurals: Korean uses `other` only; every English plural was rewritten as
`{n, plural, other {…}}` with counters (개 for things, 명 for people, 회 for
plays, 번 for attempts, 편 for episodes, 분/시간/일 for time). Exact `=0`
branches kept (규칙 없음, 일치하는 타이틀 없음). Select keys kept in English.

## Key terms (English → Korean)

- Settings → 설정
- Sign in / Sign-in → 로그인 (verb: 로그인하다, 로그인하세요)
- Sign out → 로그아웃
- Account → 계정
- Password → 비밀번호
- Profile → 프로필
- Server → 서버
- Library → 라이브러리
- Library Channel → 라이브러리 채널
- Collection → 컬렉션
- Playlist → 재생목록
- Queue (playback) → 대기열
- Home → 홈
- Search → 검색
- Play → 재생
- Pause → 일시 정지
- Resume / Continue Watching → 이어보기
- Continue Listening → 이어듣기
- Watch Together → 같이 보기
- My List → 찜 목록
- Favorites → 즐겨찾기
- Subtitles → 자막
- Closed captions → 자막 (강제 자막: 강제 자막)
- Audio → 오디오
- Video → 영상
- Quality → 화질
- Episode → 에피소드
- Season → 시즌
- Movie → 영화
- TV show / series → TV 시리즈 (short lists), 시리즈 (in prose)
- Anime → 애니메이션
- Song → 노래, Album → 앨범, Artist → 아티스트, Track → 트랙
- Audiobook → 오디오북
- Book → 책
- Downloads → 다운로드
- Recordings → 녹화
- Live TV → 실시간 TV
- Guide (TV listings) → 편성표 (instructional guide → 가이드, 안내)
- Tuner → 튜너
- Channel → 채널
- Remote access → 원격 접속
- Secure connection → 보안 연결
- Certificate → 인증서
- PIN → PIN (네 자리 PIN)
- Two-step verification → 2단계 인증
- Recovery codes → 복구 코드
- Authenticator app → 인증 앱
- Invitation → 초대
- Owner → 소유자, Member → 멤버
- Access → 접근 (권한 contexts: 접근 권한), Allow → 허용
- Notifications → 알림
- Report (bug) → 보고서 (신고 아님; feedback → 의견)
- Backup → 백업
- Restore → 복원
- Scan → 스캔, Refresh → 새로고침
- Metadata → 메타데이터, Artwork → 아트워크
- Folder → 폴더, File → 파일
- Storage → 저장 공간, Free space → 여유 공간
- Webhook → 웹훅
- API key → API 키, Token → 토큰
- Conversion → 변환 (never 트랜스코딩)
- Direct play/stream → 직접 재생/스트리밍
- Original quality → 원본 화질, Automatic → 자동
- HDR → HDR, Dolby Vision → Dolby Vision
- Surround sound → 서라운드, Stereo → 스테레오
- Player → 플레이어, Playback → 재생
- Skip intro/recap/credits → 인트로/리캡/크레딧 건너뛰기
- Watched → 봤음, Unwatched → 안 봄 (noun badges: 시청함/안 봄)
- Mark as watched → 봤음으로 표시
- Rating (stars) → 평점, (content grade) → 등급
- Filter → 필터, Sort → 정렬
- View (saved) → 저장 보기
- Match (metadata) → 매칭
- Missing (files) → 없음 / 없어진 타이틀
- Offline → 오프라인
- Delete → 삭제, Remove → 삭제 (from lists: ~에서 삭제)
- Cancel → 취소, Confirm → 확인, Save → 저장, Close → 닫기
- Edit → 편집, Create → 만들기, Add → 추가
- Retry → 다시 시도, Undo → 실행 취소
- Enable/Disable → 켜기/끄기 (switches: 켜짐/꺼짐)
- Update → 업데이트, Upgrade → 업그레이드
- Version → 버전
- Dashboard → 대시보드
- Devices → 기기 (phone → 휴대폰, tablet → 태블릿, TV as-is)
- Browser → 브라우저
- Network → 네트워크, Home network → 홈 네트워크
- Address → 주소
- Quick Connect → 퀵 커넥트
- Watch history → 시청 기록
- Play history → 재생 기록
- Recommendations → 추천
- Support bundle/diagnostics → 지원 묶음/진단 (bundles kept literal where UI-facing)
- Maintenance → 유지 관리
- Logs → 로그
- rclone remote → rclone 리모트
- XMLTV/M3U → as-is
- Portico / Portico Account / Portico Services → never translated; sentences
  keep an explicit “Portico가 …” subject where English has one (validator rule)

## Judgement calls a reviewer should check

- 이어보기 covers both Resume and Continue Watching; “봤음/안 봄” for watched
  marks is colloquial on purpose (matches streaming-app badges).
- 편성표 for the TV guide vs 가이드 for instructions; a few “guide”
  compounds had to pick one — search the record for 편성표/가이드 if unsure.
- Particle helpers (을/를, 이/가, 은/는, 으로/로) attached to placeholders,
  e.g. “{title}을(를)”. Fine for TTS/accessibility but a reviewer may prefer
  spacing variants for specific titles.
- Long server-console explanations (network ranges, FFmpeg, rclone) are
  translated plainly in 해요체; terminology there (샌드박스, 리버스 프록시,
  지문 for fingerprint) deserves a second look.
- Config example `[drive]\ntype = drive` left in English on purpose (syntax).
- Server › Access style breadcrumbs use › with spaces, copied from source.
