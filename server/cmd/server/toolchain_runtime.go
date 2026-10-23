package main

import (
	"context"
	"errors"
	"log"
	"time"

	"portico.local/server/internal/decoder"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/supervise"
)

type toolchainRuntimeFlow struct {
	probe   func(context.Context, string) (decoder.ToolchainFacts, error)
	bundle  func(context.Context, string, string, decoder.ToolchainFacts) (decoder.ToolchainFacts, error)
	current func() *decoder.ToolchainFacts
	publish func(decoder.ToolchainFacts)
}

func startToolchainRuntime(ctx context.Context, drain *drainBarrier, ffmpeg, ffprobe, selectedIdentity string) {
	bounded, stop := context.WithTimeout(ctx, 40*time.Second)
	done := make(chan struct{})
	drain.add("toolchain-runtime", stop, done)
	supervise.Go("cmd.toolchain-runtime", func() {
		defer close(done)
		defer stop()
		err := completeToolchainRuntime(bounded, ffmpeg, ffprobe, selectedIdentity, toolchainRuntimeFlow{
			probe: decoder.ProbeToolchainRuntime, bundle: decoder.QualifyToolchainBundle, current: decoder.CurrentToolchain,
			publish: func(facts decoder.ToolchainFacts) {
				decoder.ConfigureToolchain(facts)
				playback.SetConversionOutputs(decoder.AvailableConversionOutputs(&facts))
			},
		})
		if err != nil && bounded.Err() == nil {
			log.Printf("Optional media toolchain qualification unavailable: %v", err)
		}
	})
}

// Optional probes enrich only the final selected inventory. A refused optional
// qualification leaves that inventory installed, with no prospective grants.
func completeToolchainRuntime(ctx context.Context, ffmpeg, ffprobe, selectedIdentity string, flow toolchainRuntimeFlow) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if selectedIdentity == "" {
		return errors.New("selected media inventory is unavailable")
	}
	facts, err := flow.probe(ctx, ffmpeg)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if facts.Identity != selectedIdentity {
		return errors.New("selected media inventory changed before optional qualification")
	}
	facts, err = flow.bundle(ctx, ffmpeg, ffprobe, facts)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	current := flow.current()
	if facts.Identity != selectedIdentity || current == nil || current.Identity != selectedIdentity {
		return errors.New("selected media inventory changed during optional qualification")
	}
	flow.publish(facts)
	return nil
}
