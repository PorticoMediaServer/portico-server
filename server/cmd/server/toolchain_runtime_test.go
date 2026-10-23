package main

import (
	"context"
	"errors"
	"testing"

	"portico.local/server/internal/decoder"
)

func TestRuntimeToolchainPublishesOnlySelectedIdentity(t *testing.T) {
	initial := decoder.ToolchainFacts{Identity: "selected"}
	installed := initial
	flow := toolchainRuntimeFlow{
		probe: func(_ context.Context, ff string) (decoder.ToolchainFacts, error) {
			if ff != "/final/bin/ffmpeg" || installed.VulkanAvailable {
				t.Fatal("probe did not use the selected tool or published early")
			}
			return decoder.ToolchainFacts{Identity: "selected", VulkanAvailable: true}, nil
		},
		bundle: func(_ context.Context, ff, fp string, facts decoder.ToolchainFacts) (decoder.ToolchainFacts, error) {
			if ff != "/final/bin/ffmpeg" || fp != "/final/bin/ffprobe" || !facts.VulkanAvailable {
				t.Fatal("bundle did not consume the same pair's measured facts")
			}
			facts.DolbyVisionProfile7To81 = true
			return facts, nil
		},
		current: func() *decoder.ToolchainFacts { return &installed },
		publish: func(facts decoder.ToolchainFacts) { installed = facts },
	}
	if err := completeToolchainRuntime(context.Background(), "/final/bin/ffmpeg", "/final/bin/ffprobe", initial.Identity, flow); err != nil || !installed.VulkanAvailable || !installed.DolbyVisionProfile7To81 {
		t.Fatal("qualified selected facts were not installed", installed, err)
	}
}

func TestRuntimeToolchainRetainsInventoryOnFailureChangeAndCancellation(t *testing.T) {
	for _, kind := range []string{"probe-error", "bundle-error", "probe-identity", "bundle-identity", "current-identity", "no-current", "cancel-probe", "cancel-bundle"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			published := false
			initial := decoder.ToolchainFacts{Identity: "selected"}
			flow := toolchainRuntimeFlow{
				probe: func(context.Context, string) (decoder.ToolchainFacts, error) {
					if kind == "probe-error" {
						return decoder.ToolchainFacts{}, errors.New("probe refused")
					}
					if kind == "cancel-probe" {
						cancel()
					}
					facts := initial
					if kind == "probe-identity" {
						facts.Identity = "replacement"
					}
					return facts, nil
				},
				bundle: func(_ context.Context, _, _ string, facts decoder.ToolchainFacts) (decoder.ToolchainFacts, error) {
					if kind == "bundle-error" {
						return facts, errors.New("bundle refused")
					}
					if kind == "cancel-bundle" {
						cancel()
					}
					if kind == "bundle-identity" {
						facts.Identity = "replacement"
					}
					return facts, nil
				},
				current: func() *decoder.ToolchainFacts {
					if kind == "no-current" {
						return nil
					}
					if kind == "current-identity" {
						return &decoder.ToolchainFacts{Identity: "replacement"}
					}
					return &initial
				},
				publish: func(decoder.ToolchainFacts) { published = true },
			}
			if err := completeToolchainRuntime(ctx, "/final/bin/ffmpeg", "/final/bin/ffprobe", initial.Identity, flow); err == nil || published {
				t.Fatal("unqualified or stale facts replaced inventory", published, err)
			}
		})
	}
}
