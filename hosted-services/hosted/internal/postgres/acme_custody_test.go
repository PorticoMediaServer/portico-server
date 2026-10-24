package postgres_test

import (
	"bytes"
	"context"
	"testing"

	"portico.local/hosted/internal/postgres"
)

func TestACMECustodyRecoveryCASPreservesOrdersAndRefusesDrift(t *testing.T) {
	for _, mode := range []string{"current", "queued", "expired lease", "registered", "cipher changed", "directory changed", "created changed", "live lease", "provider boundary", "provider receipt", "certificate material"} {
		t.Run(mode, func(t *testing.T) {
			s := database(t)
			ctx := context.Background()
			issuer, directory := "google-production", "https://dv.acme-v02.api.pki.goog/directory"
			if _, err := s.DB.Exec(`INSERT INTO certificate_acme_accounts(issuer,directory_url,key_cipher) VALUES($1,$2,$3)`, issuer, directory, []byte("encrypted-candidate")); err != nil {
				t.Fatal(err)
			}
			candidate, err := s.ReadACMECustody(ctx, issuer, directory)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.CheckACMECandidate(ctx, candidate); err != nil {
				t.Fatal(err)
			}
			original := postgres.ACMECustody{Issuer: issuer, Directory: directory, Cipher: []byte("encrypted-original"), URL: "https://dv.acme-v02.api.pki.goog/real-account"}
			statement := ""
			switch mode {
			case "registered":
				statement = `UPDATE certificate_acme_accounts SET account_url='already-used'`
			case "cipher changed":
				statement = `UPDATE certificate_acme_accounts SET key_cipher=decode('01','hex')`
			case "directory changed":
				statement = `UPDATE certificate_acme_accounts SET directory_url='https://different.example/directory'`
			case "created changed":
				statement = `UPDATE certificate_acme_accounts SET created_at=created_at+interval '1 second'`
			}
			if statement != "" {
				if _, err = s.DB.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			withOrder := mode == "queued" || mode == "expired lease" || mode == "live lease" || mode == "provider boundary" || mode == "provider receipt" || mode == "certificate material"
			if withOrder {
				if _, err = s.DB.Exec(`INSERT INTO certificate_namespaces(operation_id,namespace) VALUES('operation','ptc-aaaaaaaaaaaaaaaaaaaa')`); err != nil {
					t.Fatal(err)
				}
				if _, err = s.DB.Exec(`INSERT INTO certificate_orders(id,request_id,operation_id,server_id,account_id,claim_generation,credential_generation,token_hash,public_key,namespace,issuer,csr,csr_hash,state,lease_until,creation_attempted,provider_url) VALUES('order','request','operation','server','account',1,1,decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),'ptc-aaaaaaaaaaaaaaaaaaaa',$1,decode(repeat('00',100),'hex'),decode(repeat('00',32),'hex'),'queued',CASE WHEN $2 THEN now()+interval '1 minute' ELSE NULL END,$3,CASE WHEN $4 THEN 'provider-receipt' ELSE '' END)`, issuer, mode == "live lease", mode == "provider boundary", mode == "provider receipt"); err != nil {
					t.Fatal(err)
				}
				if mode == "expired lease" {
					if _, err = s.DB.Exec(`UPDATE certificate_orders SET lease_until=now()-interval '1 minute'`); err != nil {
						t.Fatal(err)
					}
				}
				if mode == "certificate material" {
					if _, err = s.DB.Exec(`UPDATE certificate_orders SET chain=decode('01','hex')`); err != nil {
						t.Fatal(err)
					}
				}
			}
			var ordersBefore, ordersAfter int
			var orderBefore, orderAfter string
			if err = s.DB.QueryRow(`SELECT coalesce(json_agg(o ORDER BY id)::text,'[]') FROM certificate_orders o`).Scan(&orderBefore); err != nil {
				t.Fatal(err)
			}
			if err = s.DB.QueryRow(`SELECT count(*) FROM certificate_orders`).Scan(&ordersBefore); err != nil {
				t.Fatal(err)
			}
			err = s.RecoverACMECustody(ctx, candidate, original)
			success := mode == "current" || mode == "queued" || mode == "expired lease"
			if success {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("changed or used candidate recovered")
			}
			if e := s.DB.QueryRow(`SELECT count(*) FROM certificate_orders`).Scan(&ordersAfter); e != nil || ordersAfter != ordersBefore {
				t.Fatal("orders changed", e)
			}
			if e := s.DB.QueryRow(`SELECT coalesce(json_agg(o ORDER BY id)::text,'[]') FROM certificate_orders o`).Scan(&orderAfter); e != nil || orderBefore != orderAfter {
				t.Fatal("order facts/generations/deadlines changed", e)
			}
			if success {
				got, e := s.ReadACMECustody(ctx, issuer, directory)
				if e != nil || !bytes.Equal(got.Cipher, original.Cipher) || got.URL != original.URL || !got.CreatedAt.Equal(candidate.CreatedAt) {
					t.Fatal("custody CAS mismatch", e)
				}
				if s.RecoverACMECustody(ctx, candidate, original) == nil {
					t.Fatal("used candidate replaced on replay")
				}
			} else if mode == "live lease" || mode == "provider boundary" || mode == "provider receipt" {
				got, e := s.ReadACMECustody(ctx, issuer, directory)
				if e != nil || !bytes.Equal(got.Cipher, candidate.Cipher) || got.URL != "" {
					t.Fatal("refusal mutated candidate", e)
				}
			}
		})
	}
}
