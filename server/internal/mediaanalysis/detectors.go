package mediaanalysis

import "strings"

type sceneSignal struct{ Black, CreditStyle bool }

func measureScene(frame []byte) sceneSignal {
	dark, bright, total := 0, 0, 0
	for _, v := range frame {
		total += int(v)
		if v < 24 {
			dark++
		}
		if v > 180 {
			bright++
		}
	}
	n := float64(len(frame))
	if n == 0 {
		return sceneSignal{}
	}
	return sceneSignal{float64(dark)/n > .98, float64(total)/n < 65 && float64(bright)/n > .025 && float64(bright)/n < .35}
}
func chapterCandidates(chapters []chapter, duration int64) []Marker {
	out := []Marker{}
	for _, c := range chapters {
		if c.Start < 0 || c.End > duration || c.End <= c.Start {
			continue
		}
		title := strings.ToLower(c.Title)
		kind := ""
		switch {
		case strings.Contains(title, "recap") || strings.Contains(title, "previously"):
			kind = "recap"
		case strings.Contains(title, "intro") || strings.Contains(title, "opening"):
			kind = "intro"
		case strings.Contains(title, "credit") || strings.Contains(title, "outro"):
			kind = "credits"
		case strings.Contains(title, "commercial") || strings.Contains(title, "advert"):
			kind = "commercial"
		}
		if kind != "" {
			m := candidate(kind, c.Start, c.End, .85, "embedded_chapter_label:v1")
			m.Title = c.Title
			out = append(out, m)
		}
	}
	return out
}

// These are measured, deliberately low-confidence hypotheses, not semantic
// ground truth. Every result is unapproved; no detector authorizes skipping.
func detectSegments(frames []sceneSignal, step, duration int64) []Marker {
	if step <= 0 || len(frames) == 0 {
		return nil
	}
	type interval struct{ start, end int64 }
	gaps := []interval{}
	for i := 0; i < len(frames); {
		if !frames[i].Black {
			i++
			continue
		}
		start := i
		for i < len(frames) && frames[i].Black {
			i++
		}
		if int64(i-start)*step >= 1000000 {
			gaps = append(gaps, interval{int64(start) * step, min(int64(i)*step, duration)})
		}
	}
	out := []Marker{}
	for _, gap := range gaps {
		if gap.start >= 10000000 && gap.start <= 180000000 && gap.end < duration/3 {
			out = append(out, candidate("intro", 0, gap.start, .3, "luminance_opening_boundary:v1"))
			if gap.start <= 90000000 {
				out = append(out, candidate("recap", 0, gap.start, .2, "luminance_opening_boundary_semantic_ambiguous:v1"))
			}
			break
		}
	}
	for i := 1; i < len(gaps) && len(out) < 64; i++ {
		start, end := gaps[i-1].end, gaps[i].start
		if start > duration/6 && end < duration*9/10 && end-start >= 15000000 && end-start <= 240000000 {
			out = append(out, candidate("commercial", start, end, .25, "paired_black_boundaries_semantic_ambiguous:v1"))
		}
	}
	for i := 0; i < len(frames); {
		if !frames[i].CreditStyle || int64(i)*step < duration*4/5 {
			i++
			continue
		}
		start := i
		for i < len(frames) && frames[i].CreditStyle {
			i++
		}
		if int64(i-start)*step >= 10000000 {
			out = append(out, candidate("credits", int64(start)*step, duration, .35, "sustained_end_title_luminance:v1"))
			break
		}
	}
	return out
}
