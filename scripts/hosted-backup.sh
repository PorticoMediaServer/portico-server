#!/bin/sh
# Database base backup. Runs as the postgres user (portico-hosted-backup.service).
# The online key bundle is a separate unit running as the Hosted service user,
# which owns those 0600 files (hosted-key-backup.sh).
set -eu
umask 077
pgbackrest --stanza=hosted check
pgbackrest --stanza=hosted --type=full backup
