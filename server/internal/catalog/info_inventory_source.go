package catalog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/subtitles"
)

type inventoryInfoFrame struct {
	source         LibrarySource
	policy         ScanPolicy
	asset          int64
	token          string
	size, modified int64
	kind           string
}

// inventoryInfoFrameTx is the same live admission used by the accepting scan
// writer, repeated for each staged quantum and the final accepted-pending seal.
// A directory ETag or old read transaction never substitutes for this proof.
func inventoryInfoFrameTx(ctx context.Context, tx *sql.Tx, job, object, revision string, policyRevision int64, refresh bool) (inventoryInfoFrame, error) {
	var out inventoryInfoFrame
	source, err := InventoryFenceTx(ctx, tx, job)
	if err != nil {
		return out, err
	}
	out.source = source
	policy, err := ScanPolicyTx(ctx, tx, source.LibraryID)
	if err != nil {
		return out, err
	}
	out.policy = policy
	if policy.Revision != policyRevision || !policy.Allows("probe") {
		return out, context.Canceled
	}
	q := `SELECT a.id,o.asset_id,o.size,o.modified_ns,l.kind FROM inventory_objects o JOIN catalog_assets a ON a.token=o.asset_id AND a.size=o.size AND a.modified_ns=o.modified_ns JOIN libraries l ON l.id=? WHERE o.id=? AND o.source_id=? AND o.root_incarnation=? AND o.revision=? AND o.state='available' AND o.retired=0`
	if refresh {
		q += ` AND o.analysis_revision=o.revision`
	}
	err = tx.QueryRowContext(ctx, q, source.LibraryID, object, source.ID, source.Incarnation, revision).Scan(&out.asset, &out.token, &out.size, &out.modified, &out.kind)
	return out, err
}

// acceptInventoryInfo is the technical source half of scan publication. Naming,
// artwork and queue ACK remain in the original accepting transaction. This
// returns a receipt rather than claiming canonical facts are already applied.
func (s *Service) acceptInventoryInfo(ctx context.Context, job, object, revision string, policyRevision int64, f assets.Facts, refresh bool) (compactcatalog.InfoAcceptReceipt, error) {
	if f.InventoryOnly {
		return compactcatalog.InfoAcceptReceipt{}, errors.New("technical metadata was not observed")
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return compactcatalog.InfoAcceptReceipt{}, err
	}
	for _, stream := range f.Streams {
		raw = append(raw, stream.EncodeDetail()...)
		breadth, _ := json.Marshal(stream.Detail.StreamBreadth)
		raw = append(raw, breadth...)
	}
	fingerprint := sha256.Sum256(raw)
	check := func(ctx context.Context, tx *sql.Tx) error {
		_, err := inventoryInfoFrameTx(ctx, tx, job, object, revision, policyRevision, refresh)
		return err
	}
	return compactcatalog.PublishInfoSource(ctx, s.db, check, func(ctx context.Context, tx *sql.Tx) (compactcatalog.InfoInputSpec, []compactcatalog.InfoSourceFamily, error) {
		frame, err := inventoryInfoFrameTx(ctx, tx, job, object, revision, policyRevision, refresh)
		if err != nil {
			return compactcatalog.InfoInputSpec{}, nil, err
		}
		// Disc inputs also bind the structure/title witness; their adapter supplies
		// families10..18 before this common publisher is called by those producers.
		if f.DiscSelection != nil {
			return compactcatalog.InfoInputSpec{}, nil, compactcatalog.ErrInfoInputIncomplete
		}
		if !refresh {
			if (frame.kind == "music" || frame.kind == "audiobook") && f.AudioCodec == "" || frame.kind != "music" && frame.kind != "audiobook" && f.VideoCodec == "" {
				return compactcatalog.InfoInputSpec{}, nil, errors.New("media stream unsupported for library kind")
			}
			if err = compactcatalog.SetAssetTx(ctx, tx, frame.asset, map[string]any{"container": f.Container, "video_codec": f.VideoCodec, "audio_codec": f.AudioCodec, "width": f.Width, "height": f.Height, "duration": f.Duration}); err != nil {
				return compactcatalog.InfoInputSpec{}, nil, err
			}
		}
		var families []compactcatalog.InfoSourceFamily
		if !refresh {
			streams, err := assets.BuildInfoStreams(ctx, tx, frame.asset, frame.token, frame.size, frame.modified, f.Streams)
			if err != nil {
				return compactcatalog.InfoInputSpec{}, nil, err
			}
			families = append(families, streams...)
		}
		if !refresh || (frame.kind == "music" || frame.kind == "audiobook") && frame.policy.Allows("local_metadata") {
			chapters, err := assets.BuildInfoChapters(ctx, tx, frame.asset, frame.token, frame.size, frame.modified, f)
			if err != nil {
				return compactcatalog.InfoInputSpec{}, nil, err
			}
			families = append(families, chapters...)
		}
		if frame.policy.Allows("subtitles") {
			subs, err := subtitles.BuildInfoInput(ctx, tx, frame.asset, frame.token, frame.size, frame.modified, f.SubtitleInventory)
			if err != nil {
				return compactcatalog.InfoInputSpec{}, nil, err
			}
			families = append(families, subs...)
		}
		var mask, epoch int64
		for _, family := range families {
			mask |= 1 << uint(family.Number-1)
		}
		if err = tx.QueryRowContext(ctx, `SELECT info_epoch FROM catalog_assets WHERE id=? AND token=?`, frame.asset, frame.token).Scan(&epoch); err != nil {
			return compactcatalog.InfoInputSpec{}, nil, err
		}
		kind := compactcatalog.InfoWitnessInventory
		if refresh {
			kind = compactcatalog.InfoWitnessRefresh
		}
		witness := compactcatalog.InfoWitness{Kind: kind, JobID: job, SourceID: frame.source.ID, SourceGeneration: frame.source.Generation, RootIncarnation: frame.source.Incarnation, ObjectID: object, ObjectRevision: revision, PolicyRevision: policyRevision, Fingerprint: fingerprint}
		spec := compactcatalog.InfoInputSpec{AssetID: frame.asset, Token: frame.token, ExpectedEpoch: epoch, FamilyMask: mask, Fingerprint: fingerprint, Witnesses: []compactcatalog.InfoWitness{witness}}
		return spec, families, nil
	})
}
