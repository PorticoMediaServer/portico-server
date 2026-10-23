/* SPDX-License-Identifier: GPL-3.0-or-later */
#include "../mpegts_dovi.h"
#include <stdio.h>
#include <stdlib.h>
#define CHECK(x) do { if (!(x)) { fprintf(stderr, "check failed line %d: %s\n", __LINE__, #x); exit(1); } } while (0)
int main(int argc, char **argv) {
    CHECK(argc == 4);
    FILE *file = fopen(argv[1], "rb");
    CHECK(file);
    uint8_t *data = malloc(1048577);
    CHECK(data);
    size_t size = fread(data, 1, 1048577, file);
    CHECK(size > 0 && size <= 1048576 && feof(file));
    CHECK(!fclose(file));
    PD_DoviFacts facts;
    pd_dovi_probe(data, size, (unsigned)atoi(argv[2]), &facts);
    unsigned expected = (unsigned)atoi(argv[3]);
    CHECK(facts.base[0x1015] == expected);
    CHECK(!facts.base[0x1011]);
    printf("{\"passed\":true,\"basePid\":%u,\"malformed\":%s}\n", expected, facts.malformed ? "true" : "false");
    free(data);
    return 0;
}
