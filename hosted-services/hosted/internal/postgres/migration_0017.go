package postgres

// 0017 (BE-member, Spec — Hosted at Scale): each user server is the authority
// for its members; Hosted keeps only a discovery index ("which servers is this
// Portico Account a member of"), which servers push to, and a per-account
// server-list version that clients check from memory.
//
//   - server_members is the index, one row per (server, account) with the role
//     the server reported. Removing a row queues a members-only label rotation
//     exactly as removing a membership did (A20/A87).
//   - server_member_sync holds the push sequence each server last applied.
//   - accounts.server_list_version takes a value from one global sequence on
//     every change to the account's server set, so a device holding an older
//     version sees a larger one. The in-memory map follows the values with a
//     trailing re-read (bumpServerListVersionsTx explains why).
//
// server_member_departures records an account that removed a server from its
// list here; the server learns it at its next check-in and ends the membership,
// and re-listing it takes a consent newer than the departure (INT M6).
// server_member_full_prior holds the index while a paged full list arrives.
// Pending Hosted invitations are cancelled: invitations are the servers' now.
//
// servers_owner_account: the list and deletion look servers up by owner, which
// was a scan of every server.
//
// servers.custody_released: the server's own member list no longer names its
// custodian (the claim's account) as owner, and custody could not follow a new
// owner (a direct account, or a Portico owner that has not accepted custody).
// The server is then unowned here: the former custodian no longer sees it as
// theirs and may leave, and retiring it waits for an owner who accepts custody
// (INT M10).
//
// A Hosted sign-out everywhere or password change no longer wakes the account's
// servers: server sessions are the server's own (Justin, 17 Sep: users are
// virtually never signed out), so p10_account_epoch_policy keeps only its
// Hosted-side cleanup.
//
// A clean cut (pre-release, no users; servers' 0120 is one too): the index
// starts empty. A server's list shows its custodian from servers, and its
// members as they join again and the server pushes them. Every account that
// had a membership gets a new list version so its devices re-read the list.
// The memberships, invitations, profiles, tickets and policy tables stay
// until a later migration drops them.
const migration0017 = `
CREATE SEQUENCE server_list_versions;
ALTER TABLE accounts ADD COLUMN server_list_version bigint NOT NULL DEFAULT 0;
CREATE INDEX accounts_server_list_version ON accounts(server_list_version) WHERE server_list_version>0;
CREATE TABLE server_members(
 server_id text NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
 account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 role text NOT NULL CHECK(role IN ('owner','admin','member')),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(server_id,account_id));
CREATE INDEX server_members_account ON server_members(account_id,server_id);
CREATE INDEX servers_owner_account ON servers(owner_account_id) WHERE owner_account_id IS NOT NULL;
ALTER TABLE servers ADD COLUMN custody_released boolean NOT NULL DEFAULT false;
CREATE TABLE server_member_sync(
 server_id text PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
 sequence bigint NOT NULL DEFAULT 0 CHECK(sequence>=0),
 push_digest bytea,
 full_through bigint,
 full_page integer,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp());
CREATE TABLE server_member_full_prior(
 server_id text NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
 account_id text NOT NULL,
 role text NOT NULL,
 PRIMARY KEY(server_id,account_id));
CREATE TABLE server_member_departures(
 server_id text NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
 account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 left_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 pending boolean NOT NULL DEFAULT true,
 PRIMARY KEY(server_id,account_id));
CREATE INDEX server_member_departures_pending ON server_member_departures(server_id) WHERE pending;
CREATE TRIGGER server_members_label_rotation AFTER DELETE ON server_members
 FOR EACH ROW EXECUTE FUNCTION rotate_member_label_on_membership();
CREATE TRIGGER server_list_version_notify AFTER UPDATE OF server_list_version ON accounts
 FOR EACH STATEMENT EXECUTE FUNCTION portico_notify_queue('` + ServerListChannel + `');
UPDATE accounts SET server_list_version=nextval('server_list_versions') WHERE id IN(SELECT account_id FROM memberships WHERE NOT revoked);
UPDATE invitations SET status='cancelled' WHERE status='pending';
CREATE OR REPLACE FUNCTION p10_account_epoch_policy() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.auth_epoch=OLD.auth_epoch THEN RETURN NEW; END IF;
 DELETE FROM profile_trusted_selections WHERE account_id=NEW.id;
 DELETE FROM attachment_tickets WHERE account_id=NEW.id AND consumed_at IS NULL;
 RETURN NEW;
END $$;
`
