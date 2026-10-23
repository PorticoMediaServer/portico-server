/* SPDX-License-Identifier: GPL-3.0-or-later
 * Bounded disc structure reader. It does not decrypt or assemble loose clips.
 * Source-only WIP: public header/build and fixture qualification is outstanding.
 */
#include <errno.h>
#include <inttypes.h>
#include <stdarg.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <libbluray/bluray.h>
#include <dvdread/dvd_reader.h>
#include <dvdread/ifo_read.h>
#include <dvdread/ifo_types.h>
#include <dvdnav/dvdnav.h>

#define OUTPUT_MAX (1024 * 1024)
#define TITLE_MAX 4096
static char output[OUTPUT_MAX + 1];
static size_t used;
static int output_error;
static void emit(const char *fmt, ...) {
    va_list ap;
    int n;
    if (output_error) return;
    va_start(ap, fmt);
    n = vsnprintf(output + used, sizeof(output) - used, fmt, ap);
    va_end(ap);
    if (n < 0 || (size_t)n >= sizeof(output) - used) { output_error = 1; return; }
    used += (size_t)n;
}
static int publish(void) {
    if (output_error) return 4;
    if (fwrite(output, 1, used, stdout) != used || fflush(stdout)) return 5;
    return 0;
}
static void language(const uint8_t *lang, size_t len) {
    size_t n = 0;
    while (n < len && lang[n]) {
        if (!((lang[n] >= 'a' && lang[n] <= 'z') || (lang[n] >= 'A' && lang[n] <= 'Z'))) { emit("\"\""); return; }
        n++;
    }
    if (n != 2 && n != 3) { emit("\"\""); return; }
    emit("\"");
    for (size_t i = 0; i < n; i++) emit("%c", lang[i] >= 'A' && lang[i] <= 'Z' ? lang[i] + ('a'-'A') : lang[i]);
    emit("\"");
}
static void probe_start(const char *kind, const char *packaging, const char *encryption) {
    emit("{\"schema\":1,\"kind\":\"%s\",\"packaging\":\"%s\",\"encryption\":\"%s\",\"titles\":[", kind, packaging, encryption);
}
static void bd_streams(const BLURAY_STREAM_INFO *streams, unsigned n, const char *kind, unsigned clip, int *first) {
    for (unsigned i = 0; i < n; i++) {
        const BLURAY_STREAM_INFO *s = &streams[i];
        emit("%s{\"clipSequence\":%u,\"pid\":%u,\"kind\":\"%s\",\"coding\":%u,\"language\":", *first ? "" : ",", clip, s->pid, kind, s->coding_type);
        language(s->lang, 3);
        /* STN coding alone does not prove Dolby Vision EL. A PMT DOVI
         * descriptor pass must supply this fact before the join is enabled. */
        emit(",\"dolbyVisionEnhancementLayer\":false,\"dolbyVisionEnhancementKnown\":false}");
        *first = 0;
    }
}
static int probe_bd(BLURAY *bd, const char *packaging) {
    const BLURAY_DISC_INFO *info = bd_get_disc_info(bd);
    uint32_t n, order = 0;
    const char *encryption;
    if (!info || !info->bluray_detected) return 2;
    encryption = info->bdplus_detected ? "bdplus" : info->aacs_detected ? "aacs" : "none";
    probe_start("bluray", packaging, encryption);
    if (strcmp(encryption, "none")) { emit("]}\n"); return publish(); }
    n = bd_get_titles(bd, TITLES_RELEVANT, 60);
    if (n > TITLE_MAX) return 4;
    for (uint32_t i = 0; i < n; i++) {
        BLURAY_TITLE_INFO *t = bd_get_title_info(bd, i, 0);
        int first = 1;
        if (!t) return 3;
        if (t->duration < 60 * UINT64_C(90000)) { bd_free_title_info(t); continue; }
        emit("%s{\"discOrder\":%u,\"playlist\":%u,\"playlistFile\":\"%05u.mpls\",\"durationMs\":%" PRIu64 ",\"angleCount\":%u,\"clips\":[", order ? "," : "", order, t->playlist, t->playlist, t->duration/90, t->angle_count);
        for (uint32_t j = 0; j < t->clip_count; j++) {
            BLURAY_CLIP_INFO *c = &t->clips[j];
            /* clip_id is the library-validated five-digit identifier. */
            for (unsigned k = 0; k < 5; k++) if (c->clip_id[k] < '0' || c->clip_id[k] > '9') { bd_free_title_info(t); return 3; }
            emit("%s{\"sequence\":%u,\"file\":\"%.5s.m2ts\",\"inTime90k\":%" PRIu64 ",\"outTime90k\":%" PRIu64 "}", j ? "," : "", j, c->clip_id, c->in_time, c->out_time);
        }
        emit("],\"streams\":[");
        for (uint32_t j = 0; j < t->clip_count; j++) {
            BLURAY_CLIP_INFO *c = &t->clips[j];
            bd_streams(c->video_streams, c->video_stream_count, "video", j, &first);
            bd_streams(c->audio_streams, c->audio_stream_count, "audio", j, &first);
            bd_streams(c->pg_streams, c->pg_stream_count, "subtitle", j, &first);
            bd_streams(c->ig_streams, c->ig_stream_count, "interactive", j, &first);
            bd_streams(c->sec_video_streams, c->sec_video_stream_count, "video", j, &first);
            bd_streams(c->sec_audio_streams, c->sec_audio_stream_count, "audio", j, &first);
        }
        emit("],\"chapters\":[");
        for (uint32_t j = 0; j < t->chapter_count; j++) emit("%s{\"startMs\":%" PRIu64 "}", j ? "," : "", t->chapters[j].start/90);
        emit("]}"); order++;
        bd_free_title_info(t);
        if (output_error) return 4;
    }
    emit("]}\n"); return publish();
}
static int seek_bd(BLURAY *bd, unsigned playlist, uint64_t ms) {
    const BLURAY_DISC_INFO *info = bd_get_disc_info(bd);
    int64_t offset;
    uint64_t time;
    if (!info || !info->bluray_detected || info->aacs_detected || info->bdplus_detected) return 6;
    if (!bd_select_playlist(bd, playlist) || !bd_select_angle(bd, 0)) return 3;
    if (ms > UINT64_MAX/90) return 2;
    offset = bd_seek_time(bd, ms*90);
    if (offset < 0) return 3;
    time = bd_tell_time(bd)/90;
    if (time > ms) return 3;
    emit("{\"schema\":1,\"playlist\":%u,\"requestedMs\":%" PRIu64 ",\"byteOffset\":%" PRId64 ",\"titleTimeMs\":%" PRIu64 "}\n", playlist, ms, offset, time);
    return publish();
}
/* No libdvdcss is linked. Detect scrambled PES headers in the bounded title
 * prefix; unreadable sectors fail the probe. Later media reads are still
 * required to reject scrambled packets. This is not a decryption operation. */
static int dvd_scrambled(dvd_reader_t *dvd, int vts) {
    uint8_t block[2048];
    dvd_file_t *vob = DVDOpenFile(dvd, vts, DVD_READ_TITLE_VOBS);
    ssize_t count;
    if (!vob) return -1;
    for (int i = 0; i < 1024; i++) {
        count = DVDReadBlocks(vob, i, 1, block);
        if (count < 0) { DVDCloseFile(vob); return -1; }
        if (count == 0) break;
        for (size_t j = 0; j + 9 < sizeof(block); j++) {
            if (block[j] || block[j+1] || block[j+2] != 1) continue;
            unsigned id = block[j+3];
            if ((id == 0xbd || (id >= 0xc0 && id <= 0xef)) && (block[j+6] & 0xc0) == 0x80 && (block[j+6] & 0x30)) { DVDCloseFile(vob); return 1; }
        }
    }
    DVDCloseFile(vob); return 0;
}
static void dvd_language(uint16_t code) {
    uint8_t l[3] = {(uint8_t)(code >> 8), (uint8_t)code, 0}; language(l, 2);
}
static void dvd_stream(unsigned pid, const char *kind, unsigned coding, uint16_t lang, int *first) {
    emit("%s{\"clipSequence\":0,\"pid\":%u,\"kind\":\"%s\",\"coding\":%u,\"language\":", *first ? "" : ",", pid, kind, coding);
    dvd_language(lang); emit(",\"dolbyVisionEnhancementLayer\":false,\"dolbyVisionEnhancementKnown\":true}"); *first = 0;
}
static int probe_dvd(const char *path, const char *packaging) {
    dvd_reader_t *dvd = DVDOpen(path);
    ifo_handle_t *vmg = NULL, *vts = NULL;
    dvdnav_t *nav = NULL;
    int ret = 3, first_title = 1;
    unsigned order = 0;
    if (!dvd) return 2;
    vmg = ifoOpen(dvd, 0);
    if (!vmg || !vmg->tt_srpt || vmg->tt_srpt->nr_of_srpts > 99) goto done;
    if (dvdnav_open(&nav, path) != DVDNAV_STATUS_OK || dvdnav_set_readahead_flag(nav, 0) != DVDNAV_STATUS_OK) goto done;
    /* Check every referenced VTS before publishing any playable title. */
    for (unsigned i = 0; i < vmg->tt_srpt->nr_of_srpts; i++) {
        int status = dvd_scrambled(dvd, vmg->tt_srpt->title[i].title_set_nr);
        if (status < 0) goto done;
        if (status) { probe_start("dvd", packaging, "css"); emit("]}\n"); ret = publish(); goto done; }
    }
    probe_start("dvd", packaging, "none");
    for (unsigned i = 0; i < vmg->tt_srpt->nr_of_srpts; i++) {
        title_info_t ti = vmg->tt_srpt->title[i];
        uint64_t *ends = NULL, duration = 0;
        uint32_t count;
        unsigned pgcn;
        pgc_t *pgc;
        int first = 1;
        count = dvdnav_describe_title_chapters(nav, i+1, &ends, &duration);
        if (!count || !ends) { free(ends); goto done; }
        if (duration < 60*UINT64_C(90000)) { free(ends); continue; }
        vts = ifoOpen(dvd, ti.title_set_nr);
        if (!vts || !vts->vts_ptt_srpt || !vts->vts_pgcit || !vts->vtsi_mat || !ti.vts_ttn || ti.vts_ttn > vts->vts_ptt_srpt->nr_of_srpts) { free(ends); goto done; }
        pgcn = vts->vts_ptt_srpt->title[ti.vts_ttn-1].ptt[0].pgcn;
        if (!pgcn || pgcn > vts->vts_pgcit->nr_of_pgci_srp) { free(ends); goto done; }
        pgc = vts->vts_pgcit->pgci_srp[pgcn-1].pgc;
        if (!pgc || vts->vtsi_mat->nr_of_vts_audio_streams > 8 || vts->vtsi_mat->nr_of_vts_subp_streams > 32) { free(ends); goto done; }
        emit("%s{\"discOrder\":%u,\"title\":%u,\"vts\":%u,\"pgc\":%u,\"durationMs\":%" PRIu64 ",\"angleCount\":%u,\"clips\":[],\"streams\":[", first_title ? "" : ",", order++, i+1, ti.title_set_nr, pgcn, duration/90, ti.nr_of_angles);
        first_title = 0;
        dvd_stream(0x1e0, "video", vts->vtsi_mat->vts_video_attr.mpeg_version ? 2 : 1, 0, &first);
        for (unsigned j = 0; j < vts->vtsi_mat->nr_of_vts_audio_streams; j++) {
            audio_attr_t a = vts->vtsi_mat->vts_audio_attr[j];
            if (!(pgc->audio_control[j] & 0x8000)) continue;
            unsigned pos = (pgc->audio_control[j] >> 8) & 0x7f;
            unsigned pid = a.audio_format == 0 ? 0x80+pos : a.audio_format == 4 ? 0xa0+pos : a.audio_format == 6 ? 0x88+pos : 0x1c0+pos;
            dvd_stream(pid, "audio", a.audio_format, a.lang_code, &first);
        }
        for (unsigned j = 0; j < vts->vtsi_mat->nr_of_vts_subp_streams; j++) {
            uint32_t control = pgc->subp_control[j];
            if (!(control & UINT32_C(0x80000000))) continue;
            /* Preserve every enabled aspect-ratio presentation; source IDs
             * are substream IDs, not a guessed track ordinal. */
            for (unsigned k = 0; k < 4; k++) dvd_stream(0x20 + ((control >> (k*8)) & 0x1f), "subtitle", 0, vts->vtsi_mat->vts_subp_attr[j].lang_code, &first);
        }
        emit("],\"chapters\":[");
        for (uint32_t j = 0; j < count; j++) emit("%s{\"startMs\":%" PRIu64 "}", j ? "," : "", j ? ends[j-1]/90 : UINT64_C(0));
        emit("]}"); free(ends); ifoClose(vts); vts = NULL;
        if (output_error) { ret = 4; goto done; }
    }
    emit("]}\n"); ret = publish();
done:
    if (vts) ifoClose(vts);
    if (nav) dvdnav_close(nav);
    if (vmg) ifoClose(vmg);
    DVDClose(dvd); return ret;
}
static int number(const char *s, uint64_t *out) {
    char *end;
    if (!s || !*s) return 0;
    for (const char *p = s; *p; p++) if (*p < '0' || *p > '9') return 0;
    errno = 0; *out = strtoull(s, &end, 10); return !errno && !*end;
}
int main(int argc, char **argv) {
    struct stat st;
    BLURAY *bd;
    int result;
    if (argc < 3 || stat(argv[2], &st) || (!S_ISDIR(st.st_mode) && !S_ISREG(st.st_mode))) return 2;
    if (!strcmp(argv[1], "probe") && argc == 3) {
        bd = bd_open(argv[2], NULL);
        if (bd && bd_get_disc_info(bd) && bd_get_disc_info(bd)->bluray_detected) {
            result = probe_bd(bd, S_ISDIR(st.st_mode) ? "folder" : "iso"); bd_close(bd); return result;
        }
        if (bd) bd_close(bd);
        return probe_dvd(argv[2], S_ISDIR(st.st_mode) ? "folder" : "iso");
    }
    if (!strcmp(argv[1], "seek") && argc == 5) {
        uint64_t playlist, ms;
        if (!number(argv[3], &playlist) || playlist > 99999 || !number(argv[4], &ms)) return 2;
        bd = bd_open(argv[2], NULL); if (!bd) return 3;
        result = seek_bd(bd, (unsigned)playlist, ms); bd_close(bd); return result;
    }
    return 2;
}
