#!/usr/bin/env python3
"""Exercise source-built CPU codecs and music filters with generated media."""
import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import re
import struct
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('bundle', type=Path)
    parser.add_argument('--scratch', required=True, type=Path)
    parser.add_argument('--report', required=True, type=Path)
    args = parser.parse_args()
    manifest = json.loads((args.bundle / 'toolchain-manifest.json').read_text())
    suffix = '.exe' if manifest['target'].startswith('windows-') else ''
    ffmpeg = (args.bundle / 'bin' / ('ffmpeg' + suffix)).resolve()
    ffprobe = ffmpeg.with_name('ffprobe' + suffix)
    args.scratch.mkdir(parents=True, exist_ok=True)
    results = []

    def command(tool, flags):
        proc = subprocess.run([str(tool), *flags], stdin=subprocess.DEVNULL,
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=60)
        if len(proc.stdout) > 4 << 20 or len(proc.stderr) > 4 << 20:
            raise ValueError('recipe output exceeded 4 MiB')
        if proc.returncode:
            raise ValueError('recipe failed: ' + json.dumps(flags) + '\n' + proc.stderr.decode(errors='replace')[-4000:])
        return proc.stdout

    def convert(*flags):
        return command(ffmpeg, ['-hide_banner', '-v', 'error', '-nostdin', '-y',
                                '-threads', '1', '-filter_threads', '1', *map(str, flags)])

    def streams(path):
        return json.loads(command(ffprobe, ['-v', 'error', '-show_streams', '-of', 'json', str(path)]))['streams']

    with tempfile.TemporaryDirectory(prefix='ffmpeg-recipes-', dir=args.scratch) as directory:
        work = Path(directory)
        video = work / 'synthetic.mp4'
        convert('-f', 'lavfi', '-i', 'testsrc2=size=128x72:rate=24:duration=1',
                '-f', 'lavfi', '-i', 'sine=frequency=997:sample_rate=48000:duration=1',
                '-c:v', 'libx264', '-preset', 'ultrafast', '-threads:v', '1',
                '-c:a', 'aac', '-threads:a', '1', '-shortest', video)
        tracks = streams(video)
        video_track = next(t for t in tracks if t['codec_type'] == 'video')
        audio_track = next(t for t in tracks if t['codec_type'] == 'audio')
        decoded = convert('-i', video, '-map', '0:v:0', '-an', '-f', 'framemd5', '-')
        frames = [line for line in decoded.splitlines() if line and not line.startswith(b'#')]
        if (video_track['codec_name'] != 'h264' or video_track['r_frame_rate'] != '24/1' or
                audio_track['codec_name'] != 'aac' or len(frames) != 24):
            raise ValueError('H.264/AAC encode and continuous 24-frame decode did not agree')
        results.append({'id': 'h264-aac-continuous-24fps', 'frames': len(frames),
                        'decodedFrameMD5SHA256': hashlib.sha256(decoded).hexdigest()})

        # Encoded fixtures are generated here, never substituted licensed media.
        # TrueHD-in-MP4 is a plain codec round trip, not an Atmos/JOC fixture.
        for codec, name, expected in [('truehd', 'truehd.mp4', 'truehd'),
                                      ('dca', 'dts.mka', 'dts'), ('ac3', 'ac3.mka', 'ac3'),
                                      ('eac3', 'eac3.mka', 'eac3'), ('alac', 'alac.m4a', 'alac'),
                                      ('wavpack', 'wavpack.wv', 'wavpack'), ('tta', 'tta.tta', 'tta')]:
            path = work / name
            convert('-f', 'lavfi', '-i', 'sine=frequency=997:sample_rate=48000:duration=1',
                    '-ac', '2', '-c:a', codec, '-threads:a', '1', '-strict', '-2', path)
            track = streams(path)[0]
            pcm = convert('-i', path, '-map', '0:a:0', '-f', 's16le', '-acodec', 'pcm_s16le', '-')
            if track['codec_name'] != expected or track['sample_rate'] != '48000' or not any(pcm) or len(pcm) < 48000 * 4:
                raise ValueError('audio codec round trip failed: ' + codec)
            results.append({'id': codec + '-roundtrip', 'codec': track['codec_name'], 'decodedPCMBytes': len(pcm)})

        for rate, output_rate in [(176400, 88200), (192000, 96000)]:
            source, output = work / (str(rate) + '.wav'), work / (str(output_rate) + '.wav')
            convert('-f', 'lavfi', '-i', 'aevalsrc=0.1*sin(2*PI*997*t):s=' + str(rate) + ':d=2',
                    '-c:a', 'pcm_s32le', source)
            convert('-i', source, '-af', 'aresample=' + str(output_rate) +
                    ':resampler=soxr:precision=28:dither_method=triangular:output_sample_bits=24',
                    '-c:a', 'pcm_s24le', output)
            track = streams(output)[0]
            pcm = convert('-i', output, '-f', 's24le', '-')
            if track['sample_rate'] != str(output_rate) or len(pcm) != output_rate * 2 * 3:
                raise ValueError('soxr lost rate family or samples')
            metadata = convert('-i', source, '-af', 'ebur128=metadata=1,ametadata=mode=print:file=-', '-f', 'null', '-')
            integrated = [float(x) for x in re.findall(rb'lavfi\.r128\.I=([-+\d.]+)', metadata)]
            if not integrated or not math.isfinite(integrated[-1]) or abs(integrated[-1] - (-23.0)) > 0.6:
                raise ValueError('full-resolution BS.1770 measurement differs from the generated sine level')
            convert('-i', source, '-af', 'loudnorm=I=-16:TP=-1.5:LRA=11', '-f', 'null', '-')
            results.append({'id': 'soxr-ebur128-' + str(rate), 'sourceRate': rate,
                            'outputRate': output_rate, 'outputFrames': len(pcm) // 3,
                            'integratedLUFS': integrated[-1]})

        for multiplier in (1, 2, 4):
            bit_rate = 2822400 * multiplier
            channel_bytes, block, channels = 65536, 4096, 2
            # Schema comes from the pinned DSF demuxer: 1-bit LSBF planar,
            # sixteen full channel blocks, exact bit count and no ID3 tail.
            payload = bytes([0x69]) * (channel_bytes * channels)
            fmt = struct.pack('<4sQIIIIIIQII', b'fmt ', 52, 1, 0, 2, channels,
                              bit_rate, 1, channel_bytes * 8, block, 0)
            dsf = struct.pack('<4sQQQ', b'DSD ', 28, 28 + len(fmt) + 12 + len(payload), 0)
            dsf += fmt + struct.pack('<4sQ', b'data', 12 + len(payload)) + payload
            path = work / ('dsd' + str(64 * multiplier) + '.dsf')
            path.write_bytes(dsf)
            track = streams(path)[0]
            pcm = convert('-i', path, '-af', 'aresample=88200:resampler=soxr:precision=28', '-c:a', 'pcm_s16le', '-f', 's16le', '-')
            expected_frames = channel_bytes * 88200 // (bit_rate // 8)
            if (track['codec_name'] != 'dsd_lsbf_planar' or track['sample_rate'] != str(bit_rate // 8) or
                    len(pcm) != expected_frames * channels * 2):
                raise ValueError('DSD decode duration/rate failed: ' + str(bit_rate))
            results.append({'id': 'dsd' + str(64 * multiplier), 'bitRatePerChannel': bit_rate,
                            'decodedOutputFrames': expected_frames, 'fixtureSHA256': hashlib.sha256(dsf).hexdigest()})
    args.report.write_text(json.dumps({'target': manifest['target'], 'recipes': results,
                                      'boundary': 'Generated CPU fixtures only; no physical GPU, Atmos/JOC, Dolby Vision or playback acceptance.'}, indent=2) + '\n')
    print('Verified generated CPU codec/music recipes for ' + manifest['target'])


if __name__ == '__main__':
    main()
