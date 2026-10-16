package mediaanalysis

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"io"

	"portico.local/server/internal/decoder"
)

// Trickplay tiles and scene detection each decoded every frame of the video;
// for a film that is nearly all of a Complete scan's cost. When both are due,
// one decode serves both: the video at the scene sampling rate, at the
// trickplay tile size. Every frame is a scene sample (reduced here to the
// 32x18 grey picture the detector reads); every so many is a trickplay tile.

var sharedVideoStages = []string{"trickplay", "segment_detection"}

func isSharedVideoStage(stage string) bool {
	return stage == "trickplay" || stage == "segment_detection"
}

// sceneIntervalUS is how often the scene detector samples a video.
func sceneIntervalUS(duration int64) int64 { return max(int64(1000000), (duration+65534)/65535) }

// sharedVideoPlan says whether one decode can serve trickplay and scenes for
// this video: the trickplay interval must be a whole number of scene samples.
func (s *Service) sharedVideoPlan(b binding) (geometry trickplayLayout, trickplayUS, sceneUS int64, every int, ok bool) {
	geometry = trickplayGeometry(b.Trickplay, s.options.MaxPreviewFrames)
	trickplayUS = max(geometry.IntervalUS, (b.DurationUS+int64(geometry.MaxFrames)-1)/int64(geometry.MaxFrames))
	sceneUS = sceneIntervalUS(b.DurationUS)
	if b.VideoCodec == "" || b.DurationUS <= 0 || trickplayUS%sceneUS != 0 || trickplayUS/sceneUS > 3600 {
		return geometry, trickplayUS, sceneUS, 0, false
	}
	return geometry, trickplayUS, sceneUS, int(trickplayUS / sceneUS), true
}

// sceneFromTile reduces an RGB tile to the detector's 32x18 grey picture: the
// mean luma of each cell.
func sceneFromTile(frame []byte, width, height int, out []byte) {
	for cy := 0; cy < 18; cy++ {
		y0, y1 := cy*height/18, max(cy*height/18+1, (cy+1)*height/18)
		for cx := 0; cx < 32; cx++ {
			x0, x1 := cx*width/32, max(cx*width/32+1, (cx+1)*width/32)
			sum, n := 0, 0
			for y := y0; y < y1; y++ {
				row := y * width * 3
				for x := x0; x < x1; x++ {
					p := row + x*3
					sum += 299*int(frame[p]) + 587*int(frame[p+1]) + 114*int(frame[p+2])
					n++
				}
			}
			out[cy*32+cx] = byte(sum / (n * 1000))
		}
	}
}

// produceSharedVideo runs one decode for trickplay and segment detection
// (both due, and sharedVideoPlan said yes) and returns each stage's result.
func (s *Service) produceSharedVideo(ctx context.Context, b binding, decode Decode, publications map[string]string) (map[string]produced, error) {
	geometry, trickplayUS, sceneUS, every, _ := s.sharedVideoPlan(b)
	trick := produced{Artifacts: []artifactOutput{}, Markers: []Marker{}}
	segments := produced{Artifacts: []artifactOutput{}, Markers: []Marker{}}
	results := func() map[string]produced {
		return map[string]produced{"trickplay": trick, "segment_detection": segments}
	}
	addTile := s.adder(ctx, publications["trickplay"], &trick)
	chapters, err := s.chapters(ctx, b.AssetID)
	if err != nil {
		return results(), err
	}
	segments.Markers = chapterCandidates(chapters, b.DurationUS)
	spec := decoder.AnalysisSpec{Kind: "trickplay", MaxDurationUS: b.DurationUS, IntervalUS: sceneUS, MaxFrames: 65536, Width: geometry.TileWidth, Height: geometry.TileHeight,
		SourceVideo: decoder.PreviewVideoFacts{DynamicRange: b.Video.DynamicRange, DolbyVisionProfile: b.Video.DolbyVisionProfile, DolbyVisionCompatibility: b.Video.DolbyVisionCompatibility}}
	count, sampled := 0, 0
	signals := []sceneSignal{}
	sheet := newTrickplaySheet(geometry)
	flush := func() error {
		if sheet.frames == 0 {
			return nil
		}
		data, e := sheet.encode()
		if e != nil {
			return e
		}
		index := (count - 1) / geometry.PerSheet
		start := int64(index*geometry.PerSheet) * trickplayUS
		end := min(b.DurationUS, int64(count)*trickplayUS)
		if e = addTile(data, trickplaySheetKind, "image/jpeg", index, start, end, sheet.width(), sheet.height()); e != nil {
			return e
		}
		sheet.reset()
		return nil
	}
	err = decode(ctx, publications["trickplay"], spec, func(r io.Reader) error {
		frame := make([]byte, geometry.TileWidth*geometry.TileHeight*3)
		scene := make([]byte, 32*18)
		for {
			_, e := io.ReadFull(r, frame)
			if e == io.EOF {
				return nil
			}
			if e != nil {
				return e
			}
			if sampled >= 65536 {
				return ErrBudget
			}
			sceneFromTile(frame, geometry.TileWidth, geometry.TileHeight, scene)
			signals = append(signals, measureScene(scene))
			index := sampled
			sampled++
			if index%every != 0 {
				continue
			}
			if count >= geometry.MaxFrames {
				return ErrBudget
			}
			if int64(count)*trickplayUS >= b.DurationUS {
				return ErrConflict
			}
			img := image.NewRGBA(image.Rect(0, 0, geometry.TileWidth, geometry.TileHeight))
			for p := 0; p < geometry.TileWidth*geometry.TileHeight; p++ {
				img.SetRGBA(p%geometry.TileWidth, p/geometry.TileWidth, color.RGBA{frame[p*3], frame[p*3+1], frame[p*3+2], 255})
			}
			sheet.place(img)
			count++
			if count%geometry.PerSheet == 0 {
				if e = flush(); e != nil {
					return e
				}
			}
		}
	})
	if err == nil {
		err = flush()
	}
	if err != nil {
		return results(), err
	}
	if count == 0 {
		return results(), ErrUnsupported
	}
	descriptor := geometry.descriptor(count, trickplayUS, b.DurationUS)
	if !descriptor.valid() {
		return results(), ErrUnsupported
	}
	trick.Summary = trickplaySummary{TrickplayDescriptor: descriptor, Sampled: true}
	if err = addTile([]byte(jsonString(descriptor)), trickplayDescriptorKind, "application/json", 0, 0, min(b.DurationUS, max(1, int64(count)*trickplayUS)), descriptor.Width, descriptor.Height); err != nil {
		return results(), err
	}
	segments.Markers = append(segments.Markers, detectSegments(signals, sceneUS, b.DurationUS)...)
	if len(segments.Markers) > 512 {
		return results(), ErrBudget
	}
	segments.Summary = map[string]any{"detector": "chapter-label-and-luminance-v1", "candidateCount": len(segments.Markers), "sampledFrames": len(signals), "sampleIntervalUS": fmt.Sprint(sceneUS), "automaticSkip": false, "accuracy": "heuristic; owner review required", "sharedDecode": true}
	return results(), nil
}
