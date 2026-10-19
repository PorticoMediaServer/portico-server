# 日本語グロッサリー（ja）— Portico UI 翻訳

 politeness: 文（説明・エラー・確認・通知）は丁寧なです／ます調。ボタン・タブ・メニュー項目などの短いラベルは名詞形で簡潔に（日本の主要配信アプリの慣行に準拠）。

## 住所・呼称
- ユーザーへの呼びかけは二人称を避け、無主語または「あなたへの～」の形に統一（例：home.row.genreForYou → あなたへの{genre}）。
- 所有者：サーバーの所有者。管理者（Administrator）は管理者、メンバー（Member）はメンバー。
- プロフィール：プロフィール（Profile）。メインプロフィール、プロフィールを切り替え。
- デバイス：デバイス。このデバイス、そのデバイス。

## 主要用語（English → 日本語）
- Search → 検索
- Settings → 設定
- Home → ホーム
- Library → ライブラリ
- Saved → 保存済み
- Sign in → ログイン ／ Sign out → ログアウト
- Watchlist / My List → マイリスト
- Continue Watching → 視聴中の作品
- Up Next → 次の作品
- Favorites → お気に入り
- Downloads → ダウンロード
- Watch Together → 一緒に観る
- Channels → チャンネル ／ Library Channels → ライブラリチャンネル ／ Guide → 番組表
- Live TV → ライブTV
- Subtitles → 字幕 ／ Audio → 音声 ／ Quality → 画質
- Episode → エピソード ／ Season → シーズン ／ Show → 番組（kind名では TV番組）／ Movie → 映画
- Song → 曲 ／ Album → アルバム ／ Artist → アーティスト ／ Audiobook → オーディオブック
- Collection → コレクション ／ Playlist → プレイリスト ／ Queue → キュー
- Server → サーバー ／ Account → アカウント
- Notifications → 通知 ／ History → 履歴
- Play → 再生 ／ Pause → 一時停止 ／ Resume → 続きから再生／再開
- Cancel → キャンセル ／ Save → 保存 ／ Delete → 削除 ／ Remove → 削除
- Close → 閉じる ／ Dismiss → 閉じる ／ Done → 完了 ／ Undo → 元に戻す
- Try again → もう一度お試しください（文中）／再試行（短ラベル action.tryAgain）
- Refresh → 更新 ／ Retry → もう一度お試しください
- Details → 詳細 ／ More → その他 ／ Show more → さらに表示
- Recording → 録画 ／ Tuner → チューナー
- Age rating / Content rating → 年齢制限 ／ Rating system → レイティング制度
- PIN → PIN ／ Password → パスワード ／ Two-step verification → 2段階認証
- Recovery code → 再設定コード ／ Authenticator app → 認証アプリ
- Remote access → リモートアクセス ／ Secure connection → 安全な接続
- Metadata → メタデータ ／ Match(ing) → マッチング／照合（動詞は照合、状態名はマッチング寄りで文脈により使い分け）
- Scan (library) → 読み取り（名詞）／読み取る（動詞）。「スキャン」は使わない
- Backup → バックアップ ／ Restore → 復元
- Webhook → Webhook ／ API key → APIキー ／ Personal access token → 個人アクセストークン
- Transcode / Convert → 変換 ／ Direct play → ダイレクト再生 ／ Direct stream → ダイレクトストリーム
- Original quality → オリジナル（画質）／ Automatic → 自動
- HDR → HDR ／ Dolby Vision → Dolby Vision（製品名扱いで原文維持）
- Picture in Picture → ピクチャインピクチャ
- AirPlay / Google Cast / Chromecast 的表記 → AirPlay／Google Cast（原文維持）
- mpv / FFmpeg / TMDB / M3U / XMLTV / HDHomeRun → 原文維持
- Portico / Portico Account / Portico Server / Portico Title Dataset → 翻訳せず原文維持（検査で必須）
- 言語名（language.*）→ 各言語の日本語名（英語→英語、日本語→日本語、フランス語→フランス語…）

## 句読点・表記規約
- 全角句読点 。、！？ を使用。半角 . , ! ? は使わない（URL・コード・数式・“192.168.1.20:32500”のような例示は除く）。
- 三点リーダは …（U+2026、全角1文字）。末尾の継続を示す「…中」と組み合わせる（読み込み中…、接続中…）。
- 中点は ・（例：S{season}・E{episode}、{field}・{direction}）。
- 範囲・区切りは ～（例：{min}～{max}、1～100）。
- 英語の引用符 “ ” は日本語では 「 」 に置換（例：「{title}」を削除しますか？）。
- 英語のアポストロフィ ’（U+2019）は維持（’s を含む所有格は「～の」に訳すため基本出現しない）。
- コロンに相当する区切りは ： または ・ を使用（例：監督：{name}、確認：{when}）。
- パーセントは ％（全角）。Mbps、fps、GB、MB などの単位は半角英字のまま。
- スペース区切りの英語複合語は詰める（例：ライブTV、2段階認証）。
- ブラウザーは長音付き「ブラウザー」、コンピューターは使わず「パソコン」、スマートフォンはカタカナ表記。

## 複数形・ICU
- 日本語は plural カテゴリー other のみ。one/other の原文は other に統一（=0 など exact ブランチは保持）。
- select のキー（home/library/other、yes/no、asc/other など）は原文のまま、値のみ翻訳。
- {argument} はすべて原文どおり保持。# は数値表示に使用。
- 前後空白は原文どおり保持。

## 判断メモ（レビュワー向け）
- 「Watch Together」は機能名として「一緒に観る」に統一（行名・設定・招待文すべて）。
- 「My List」は Netflix JP にならい「マイリスト」。
- 「Continue Watching」は行名「視聴中の作品」、本文では「視聴中の作品から～」で統一。
- 「Queue」は「キュー」に統一（再生待ち行列の意味）。
- 「Shows」は一覧系では「番組」、kind名（title.kind.show）では「TV番組」。
- 「Title」（作品）の訳は文脈で「タイトル」／「作品」を使い分け（cannotPlay.thisTitle → このタイトル、home.row.hiddenGems → 隠れた名作）。
- 「Unavailable」は文脈で「利用できません」／「利用不可」／「利用不可のアイテム」を使い分け。
- 「Recordings」は録画物として「録画」、「storage」は「ストレージ」。
- 「Library scan」は「ライブラリの読み取り」に統一（スキャン不使用）。
- サーバー管理系の“the server’s owner”は「サーバーの所有者」に統一。
- 技術系固有名（UPnP、NAT-PMP、PCP、VA-API、Quick Sync、NVENC、AMF、VideoToolbox、PEM、rclone、WebDAV、MCP、OpenSubtitles、AniList、IMDb、TVDB）は原文維持。
- web.direct.recoveryHelp の `portico-server recover-owner` は CLI コマンドのため原文維持。
- 長文の server.setup.*、secure.*、claim.* は丁寧文で統一し、主語 Portico は検査要件のため残した（日本語として自然な範囲で文頭・文中に配置）。
