#!/usr/bin/env python3
"""Generate tiny owned conformance fixtures. No server/library/user media is copied."""
import os
import pathlib
import subprocess
root=pathlib.Path(os.environ["TMPDIR"]).parent
pcm=root/"owned-48000-stereo.wav"
wavpack=root/"owned-48000-stereo.wv"
video=root/"owned-vfr-6.mkv"
subprocess.run(["ffmpeg","-v","error","-f","lavfi","-i","sine=frequency=997:sample_rate=48000:duration=2","-ac","2","-c:a","pcm_s32le",str(pcm)],check=True)
subprocess.run(["ffmpeg","-v","error","-i",str(pcm),"-c:a","wavpack",str(wavpack)],check=True)
expr=r"setpts=if(eq(N\,0)\,0\,if(eq(N\,1)\,1\,if(eq(N\,2)\,4\,if(eq(N\,3)\,5\,if(eq(N\,4)\,12\,17)))))"
subprocess.run(["ffmpeg","-v","error","-f","lavfi","-i","testsrc2=size=64x64:rate=10","-frames:v","6","-vf",expr,"-fps_mode","vfr","-c:v","ffv1",str(video)],check=True)
subprocess.run(["ffprobe","-v","error","-select_streams","v:0","-show_entries","frame=pts_time","-of","csv=p=0",str(video)],check=True)
