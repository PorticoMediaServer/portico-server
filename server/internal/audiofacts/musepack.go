package audiofacts

import (
	"encoding/binary"
	"hash/crc32"
)

// CompleteCodecHeaders supplies the Musepack sample counts FFmpeg's demuxers
// omit. Raw FFmpeg output includes the codec's 481-frame synthesis delay.
// Counts and beginning silence come from this exact retained source header,
// and must fit the fully decoded raw-frame measurement before publication.
func CompleteCodecHeaders(f Facts, prefix []byte) (Facts, error) {
	if f.Codec != "mpc7" && f.Codec != "mpc8" {
		return f, nil
	}
	if n := id3End(prefix); n > 0 {
		if n >= int64(len(prefix)) {
			return f, ErrInconsistent
		}
		prefix = prefix[n:]
	}
	var total, silence int64
	var rate, channels int
	if f.Codec == "mpc7" {
		if len(prefix) < 28 || string(prefix[:3]) != "MP+" || prefix[3] != 7 && prefix[3] != 0x17 {
			return f, ErrInconsistent
		}
		frames := int64(binary.LittleEndian.Uint32(prefix[4:8]))
		flags := binary.LittleEndian.Uint32(prefix[20:24])
		last := int64((flags >> 20) & 0x7ff)
		if last == 0 {
			last = 1152
		}
		if frames <= 0 || last > 1152 {
			return f, ErrInconsistent
		}
		total = frames * 1152
		if flags>>31 != 0 {
			total -= 1152 - last
		} else {
			total -= 481
		}
		rate = []int{44100, 48000, 37800, 32000}[(binary.LittleEndian.Uint32(prefix[8:12])>>16)&3]
		channels = 2
	} else {
		if len(prefix) < 8 || string(prefix[:4]) != "MPCK" {
			return f, ErrInconsistent
		}
		p := 4
		found := false
		for packets := 0; packets < 64 && p+3 <= len(prefix); packets++ {
			start := p
			key := string(prefix[p : p+2])
			p += 2
			size, n, ok := mpcVar(prefix[p:])
			if !ok || size < 3 || size > int64(len(prefix)-start) {
				return f, ErrInconsistent
			}
			p += n
			end := start + int(size)
			if end < p {
				return f, ErrInconsistent
			}
			if key == "SH" {
				b := prefix[p:end]
				if len(b) < 8 || b[4] != 8 || binary.BigEndian.Uint32(b[:4]) != crc32.ChecksumIEEE(b[4:]) {
					return f, ErrInconsistent
				}
				total, n, ok = mpcVar(b[5:])
				if !ok {
					return f, ErrInconsistent
				}
				b = b[5+n:]
				silence, n, ok = mpcVar(b)
				if !ok {
					return f, ErrInconsistent
				}
				b = b[n:]
				if len(b) < 2 || int(b[0]>>5) > 3 {
					return f, ErrInconsistent
				}
				rate = []int{44100, 48000, 37800, 32000}[b[0]>>5]
				channels = int(b[1]>>4) + 1
				found = true
				break
			}
			p = end
		}
		if !found {
			return f, ErrInconsistent
		}
	}
	start := int64(481) + silence
	audible := total - silence
	if rate != f.SampleRate || channels != f.Channels || silence < 0 || audible <= 0 || start > f.RawFrames || audible > f.RawFrames-start {
		return f, ErrInconsistent
	}
	if f.DurationFrames > 0 {
		f.BitRate = int64(float64(f.BitRate) * float64(f.DurationFrames) / float64(audible))
	}
	f.StartFrames, f.DurationFrames, f.EndFrames, f.TrimSource = start, audible, f.RawFrames-start-audible, "musepack-header"
	return f, nil
}

func mpcVar(b []byte) (int64, int, bool) {
	var value int64
	for i, c := range b {
		if i >= 8 || value > ((1<<53)-1-int64(c&127))>>7 {
			return 0, 0, false
		}
		value = value<<7 | int64(c&127)
		if c&128 == 0 {
			return value, i + 1, true
		}
	}
	return 0, 0, false
}
