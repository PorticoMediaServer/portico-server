# Turkish (tr) glossary

Status: machine translation, marked for native-speaker review. Not yet reviewed.

Address: neutral polite "siz" throughout, the usual form in Turkish streaming apps. Imperatives take the plural ending ("Yeniden deneyin", "Oturum açın", "Kaydedin"), and possessives are "-iniz/-ınız/-unuz/-ünüz" ("Hesabınız", "Sunucunuz"). Turkish has no separate "tu" form in the catalogue, so the informal "sen" is never used.

Typography and conventions:
- Sentence case for buttons, labels, headings and tabs ("Yeniden dene", "Sırada"). Title case is not used.
- Dotted and dotless i: "İ" is the capital of "i" and "ı" is the lowercase of "I". Check words starting with capital İ (for example "İndirilenler", "İzlemeye devam et").
- Quotation marks are curly double quotes “ ” around names and titles, as in the English.
- Apostrophe: U+2019 (’) before Turkish suffixes on names, as in "Portico’nun", "Portico’yu", "Portico Account’unuz", "Webhook’lar". Never the ASCII '.
- Ellipsis is "…" (U+2026), kept from the English.
- Percentages put the sign before the number, with no space: "%50" (Turkish convention, not "50 %").
- Numbers and units: after a number a Turkish noun stays singular ("3 bölüm", "10 saniye"). Both plural branches therefore read the same; the other branch is kept so the sentence still parses.
- Product names are never translated: Portico, Portico Account, Portico Services. Where Turkish needs a suffix, it follows the apostrophe rule above.
- Language names (`locale.*`) are not translated.

## Core UI terms (English → Turkish)

- Settings → Ayarlar
- Sign in → Oturum aç (noun: Oturum açma); Sign out → Oturumu kapat
- Sign up / Create account → Hesap oluştur
- Account → Hesap; Your account → Hesabınız
- Profile → Profil; Profiles → Profiller
- Library → Kitaplık; Libraries → Kitaplıklar
- Home → Ana sayfa
- Search (verb) → Ara; Search (screen or noun) → Arama
- My List / Watchlist → Listem; add to My List → Listeme ekle; remove → Listemden çıkar
- Continue Watching → İzlemeye devam et; Continue Listening → Dinlemeye devam et
- Up Next → Sırada
- Recommended for you → Sana önerilenler; Trending now → Şimdi popüler
- Recently added → Son eklenenler
- Favorites → Favoriler; Saved → Kaydedilenler
- Movies → Filmler; Movie → Film
- Shows (TV) → Diziler; Show → Dizi
- Season → Sezon; Episode → Bölüm
- Track / Song → Parça; Album → Albüm; Artist → Sanatçı
- Books → Kitaplar; Book → Kitap; Audiobooks → Sesli kitaplar
- Subtitles → Altyazılar; Subtitle → Altyazı; Audio → Ses
- Downloads (screen) → İndirilenler; Download (single) → İndirme
- Play → Oynat; Pause → Duraklat; Resume → Devam et; Stop → Durdur
- Continue (button) → Devam et
- Skip intro → Girişi atla; Skip credits → Jeneriği atla
- Watch Together → Birlikte izle
- Live TV → Canlı TV; Guide (TV guide) → Program rehberi; Channel → Kanal
- Recordings → Kayıtlar; Record (verb, a live show) → Kayıt al; Save → Kaydet
- Cast (actors) → Oyuncular; Crew → Ekip; Director → Yönetmen; Starring → Başrolde
- Collection → Koleksiyon; Playlist → Çalma listesi
- Notifications → Bildirimler; History → Geçmiş; Watch history → İzleme geçmişi
- Appearance → Görünüm; Language → Dil; Preferences → Tercihler
- Quality → Kalite; Automatic (quality) → Otomatik; Original → Orijinal
- Direct play → Doğrudan oynatma; Converting / Transcoding → Dönüştürme; Buffer → Arabellek
- Storage → Depolama; Backup → Yedek; Restore → Geri yükle
- Scan → Tara; Refresh → Yenile; Match (metadata) → Eşleştir
- Delete → Sil; Remove → Kaldır; Undo → Geri al; Cancel → İptal; Close → Kapat; Done → Tamam
- Discard → Vazgeç; Dismiss → Kapat; Dismiss alert → Kapat
- Try again → Yeniden dene; Show more → Daha fazla göster; See all → Tümünü gör
- View → Görüntüle; Details → Ayrıntılar; Clear all → Tümünü temizle; Back to the start → Başa dön
- Reset to default → Varsayılana sıfırla
- Devices → Cihazlar; This device → Bu cihaz; TV (device) → TV
- PIN → PIN; Password → Şifre; Two-step verification → İki adımlı doğrulama
- Owner → Sahip; Member → Üye; Administrator → Yönetici; Role → Rol
- Limits → Sınırlar; Parental rating → Yaş derecelendirmesi; Rating (score) → Puan
- API keys → API anahtarları; Webhooks → Webhook’lar; Logs → Günlükler
- Support export → Destek dışa aktarımı; Quick Connect → Hızlı Bağlantı
- Unwatched → İzlenmemiş; Watched → İzlendi
- Genre → Tür; Year → Yıl; Decade → On yıl; Duration / Runtime → Süre
- Sort by → Sırala; Filters → Filtreler
- Server → Sunucu; Server console → Sunucu konsolu; Remote access → Uzaktan erişim
- Secure connection → Güvenli bağlantı; Port (network) → Bağlantı noktası; Address → Adres; Certificate → Sertifika
- Metadata → Meta veri (written as two words, the common Turkish usage)
- Artwork → Kapak görseli
- Watch state words for the progress badge → "İzlendi", "İzlenmemiş"

## Plurals

Turkish uses `one` and `other` only. The `one` and `other` branches are written the same way for count nouns, because the noun stays singular after a number. Example: `{seconds, plural, one {# saniye} other {# saniye}}`.

## More terms

- Chapter (a marker inside a file) → Bölüm işareti; Episode → Bölüm. "Bölüm" is never used for chapters.
- Intro → Giriş; Recap → Özet; Credits → Jenerik; Skip credits → Jeneriği atla
- Part (of a film) → Kısım; Disc → Disk; Track → Parça
- Trickplay (seek-bar pictures) → Önizleme küçük resimleri (settings); Önizleme karoları (storage category)
- Artwork → Kapak görseli (singular) / Kapak görselleri (plural)
- Rating (score) → Puan; Parental / content rating → Yaş derecelendirmesi
- Reconnecting → Yeniden bağlanılıyor
- DVR (the recording feature) → Kayıt; the word "DVR" appears only inside "Canlı TV ve DVR"

## Judgement calls for review

- "Your account" is Hesabınız (the owner's own account). "Account" in general is Hesap.
- "Continue" and "Resume" both use "Devam et". The context (button or playback) makes the meaning clear.
- "Watch history" and "History" use "Geçmiş" in the same way.
- "Downloads" for the feature uses "İndirilenler" for the screen and "İndirme" for one file. Review whether "İndirmeler" reads better in the navigation tab.
- "Server console" is "Sunucu konsolu", a literal translation. A native reviewer may prefer "Sunucu yönetimi".
- "Watch Together" is "Birlikte izle", the usual term in Turkish streaming apps.
- "Portico Account" stays in English. Turkish suffixes attach with a curly apostrophe ("Portico Account’unuz", "Portico’nun"). Some sentences could be rephrased to avoid the suffix if a reviewer prefers.
- "Guide" for live TV is "Program rehberi" (electronic programme guide).
- Sign-in with the Terms link. `auth.agreeToTerms` reads "Devam ederek şunları kabul edersiniz:" and is followed by the two link texts ("Kullanım Koşulları" and "Gizlilik Politikası"). The app must join those two links with "ve" for the sentence to read correctly. Check this composition in the app.
- Typed confirmation word `server.confirm.word.delete` stays "delete", because the server expects that exact typed token. The same applies to other typed tokens.
- The "Ratings" and "Limits" screens use "Derecelendirme" and "Yaş derecelendirmesi" consistently; check it against the parental-controls copy.
- Percentages put the sign first with no space ("%50", "%80"), as is usual in Turkish.
- Turkish uses "siz" throughout, so imperatives end in "-in/-ın/-un/-ün/-iniz" ("Yeniden deneyin", "Oturum açın").
- "Izgara" (grid) keeps the dotless capital I in the Turkish spelling; check any title-case uses.
- Plural branches: Turkish needs only `one` and `other`. Both branches use the same singular noun after a number.
