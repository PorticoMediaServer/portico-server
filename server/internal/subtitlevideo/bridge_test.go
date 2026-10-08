package subtitlevideo

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"portico.local/server/internal/decoder"
)

func TestMediaBridgeComposesWithConfinedDecoder(t *testing.T) {
	tool, lookErr := exec.LookPath("true")
	if lookErr != nil {
		t.Skip("no true(1) here")
	}
	source := &fixtureCachedSource{}
	bridge := NewMediaBridge(source)
	var previous string
	for range 2 {
		err := bridge.With(context.Background(), func(url string, reservation *decoder.EndpointReservation) error {
			if url == previous {
				t.Fatal("fresh physical attempt reused its private endpoint")
			}
			previous = url
			cmd, err := decoder.ProbeCommand(tool, url, reservation)
			// Every platform builds the decoder: sandboxed where it can, the
			// baseline elsewhere (mediaexec).
			{
				if err != nil {
					t.Fatalf("bridge rejected by decoder boundary: %v", err)
				}
				if cmd == nil {
					t.Fatal("missing confined command")
				}
			}
			// The admitted URL reaches only its retained source; altered tokens and
			// provider-style query strings cannot redirect or fetch other data.
			request, _ := http.NewRequest(http.MethodGet, url, nil)
			request.Header.Set("Range", "bytes=0-15")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				return err
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				return err
			}
			if response.StatusCode != 206 || len(body) != 16 {
				t.Fatalf("range response %d / %d bytes", response.StatusCode, len(body))
			}
			for _, invalid := range []string{url + "?next=private", url[:strings.LastIndex(url, "/")+1] + strings.Repeat("z", 64)} {
				if _, err := decoder.ProbeCommand("/private/test/ffprobe", invalid, reservation); !errors.Is(err, decoder.ErrInvalidConfiguration) {
					t.Fatalf("accepted altered URL: %v", err)
				}
				response, err := http.Get(invalid)
				if err != nil {
					return err
				}
				response.Body.Close()
				if response.StatusCode != 404 {
					t.Fatalf("altered URL reached source: %d", response.StatusCode)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
