package localmetadata

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestLocalArtworkRejectsUnboundedOrNonImageContent(t *testing.T) {
	service, err := New(t.TempDir(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.store([]byte("<html>not artwork</html>")); err == nil {
		t.Fatal("HTML accepted as cover")
	}
	var wide bytes.Buffer
	if err = png.Encode(&wide, image.NewRGBA(image.Rect(0, 0, 10000, 1))); err != nil {
		t.Fatal(err)
	}
	if _, err = service.store(wide.Bytes()); err == nil {
		t.Fatal("unbounded dimensions accepted")
	}
	if _, _, err = service.Open("../../secret"); err == nil {
		t.Fatal("art path traversal accepted")
	}
	var valid bytes.Buffer
	if err = png.Encode(&valid, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	key, err := service.store(valid.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	file, mime, err := service.Open(key)
	if err != nil || mime != "image/png" {
		t.Fatal(mime, err)
	}
	file.Close()
}
