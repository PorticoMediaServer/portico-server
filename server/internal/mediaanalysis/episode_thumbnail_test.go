package mediaanalysis

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"portico.local/server/internal/decoder"
	"testing"
)

type thumbnailRangeInput struct {
	reads, validated int
	changed          bool
}

func (i *thumbnailRangeInput) Evidence() string { return "test-evidence" }
func (i *thumbnailRangeInput) Size() int64      { return 512 << 30 }
func (i *thumbnailRangeInput) ReadExtent(_ context.Context, off, n int64) ([]byte, error) {
	if n > 1024 || off < 0 {
		return nil, ErrBudget
	}
	i.reads++
	return make([]byte, n), nil
}
func (i *thumbnailRangeInput) Validate(context.Context) error {
	i.validated++
	if i.changed {
		return ErrConflict
	}
	return nil
}
func (i *thumbnailRangeInput) Close() error { return nil }
func thumbnailJPEG(w, h int) []byte {
	var b bytes.Buffer
	jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h)), nil)
	return b.Bytes()
}
func TestEpisodeThumbnailPolicyProviderAndRangedGeneration(t *testing.T) {
	input := &thumbnailRangeInput{}
	allowed := 0
	decoded := 0
	request := EpisodeThumbnailRequest{Mode: EpisodeThumbnailDefaultMode, DurationUS: 60000000, Input: input, Authorize: func(context.Context) error { allowed++; return nil }, Decoder: func(in Input) Decode {
		return func(ctx context.Context, key string, spec decoder.AnalysisSpec, consume func(io.Reader) error) error {
			decoded++
			if key != EpisodeThumbnailOperation || spec.Kind != "image" || spec.StartUS != 6000000 {
				t.Fatalf("recipe %+v %s", spec, key)
			}
			if _, err := in.ReadExtent(ctx, 1<<30, 128); err != nil {
				return err
			}
			return consume(bytes.NewReader(thumbnailJPEG(320, 180)))
		}
	}}
	result, err := MakeEpisodeThumbnail(context.Background(), request)
	if err != nil || result.Origin != "generated" || len(result.JPEG) == 0 || decoded != 1 || allowed != 2 || input.reads != 1 || input.validated != 2 {
		t.Fatalf("result %+v err=%v reads=%d auth=%d", result, err, input.reads, allowed)
	}
	request.ProviderStill = true
	result, err = MakeEpisodeThumbnail(context.Background(), request)
	if err != nil || result.Origin != "provider" || decoded != 1 {
		t.Fatalf("provider path %+v %v", result, err)
	}
	request.Manual = true
	result, err = MakeEpisodeThumbnail(context.Background(), request)
	if err != nil || result.Origin != "generated" || decoded != 2 {
		t.Fatalf("manual path %+v %v", result, err)
	}
	for _, kind := range []string{"tv", "anime"} {
		if EpisodeThumbnailMode(kind) != EpisodeThumbnailDefaultMode {
			t.Fatal("episode defaults")
		}
	}
	for _, kind := range []string{"movie", "music", "audiobook", "book", "comic"} {
		if EpisodeThumbnailMode(kind) != "off" {
			t.Fatal("non-episode default")
		}
	}
}
func TestEpisodeThumbnailRejectsPolicyChangesCorruptOrExcessiveOutput(t *testing.T) {
	denied := errors.New("source policy denied")
	calls := 0
	r := EpisodeThumbnailRequest{Mode: EpisodeThumbnailDefaultMode, DurationUS: 10, Input: &thumbnailRangeInput{}, Authorize: func(context.Context) error { return denied }, Decoder: func(Input) Decode { t.Fatal("decoder acquired before policy"); return nil }}
	if _, err := MakeEpisodeThumbnail(context.Background(), r); !errors.Is(err, denied) {
		t.Fatal(err)
	}
	r.Authorize = func(context.Context) error {
		calls++
		if calls > 1 {
			return denied
		}
		return nil
	}
	r.Decoder = func(Input) Decode {
		return func(_ context.Context, _ string, _ decoder.AnalysisSpec, consume func(io.Reader) error) error {
			return consume(bytes.NewReader(thumbnailJPEG(320, 180)))
		}
	}
	if result, err := MakeEpisodeThumbnail(context.Background(), r); !errors.Is(err, denied) || len(result.JPEG) > 0 {
		t.Fatalf("published after revocation %+v %v", result, err)
	}
	r.Authorize = func(context.Context) error { return nil }
	for _, mode := range []string{"off", "provider_only"} {
		r.Mode = mode
		r.Manual = true
		if _, err := MakeEpisodeThumbnail(context.Background(), r); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("manual bypass %s %v", mode, err)
		}
	}
	r.Mode = EpisodeThumbnailDefaultMode
	r.Manual = false
	for _, data := range [][]byte{[]byte("bad image"), thumbnailJPEG(1, 1), make([]byte, (1<<20)+1)} {
		r.Decoder = func(Input) Decode {
			return func(_ context.Context, _ string, _ decoder.AnalysisSpec, consume func(io.Reader) error) error {
				return consume(bytes.NewReader(data))
			}
		}
		if _, err := MakeEpisodeThumbnail(context.Background(), r); err == nil {
			t.Fatal("invalid image accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := MakeEpisodeThumbnail(ctx, r); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	r.Input = &thumbnailRangeInput{changed: true}
	if _, err := MakeEpisodeThumbnail(context.Background(), r); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}
