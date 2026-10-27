#!/usr/bin/env python3
"""Serve the private generated live source; child encoder follows service lifetime."""
from pathlib import Path
import argparse, functools, http.server, signal, subprocess, threading


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('root',type=Path)
    parser.add_argument('--ffmpeg',default='/opt/portico-ffmpeg/bin/ffmpeg')
    parser.add_argument('--port',type=int,default=32511)
    args=parser.parse_args()
    root=args.root.resolve()
    encoder=subprocess.Popen([args.ffmpeg,'-hide_banner','-loglevel','error','-nostdin','-re',
        '-stream_loop','-1','-i',str(root/'source.mp4'),'-c','copy','-f','hls','-hls_time','2',
        '-hls_list_size','8','-hls_flags','delete_segments+temp_file','-hls_segment_filename',
        str(root/'live-%06d.ts'),str(root/'live.m3u8')])
    class Handler(http.server.SimpleHTTPRequestHandler):
        def log_message(self, fmt, *args):
            pass
    server=http.server.ThreadingHTTPServer(('127.0.0.1',args.port),functools.partial(Handler,directory=str(root)))
    server.daemon_threads=True
    stopping=threading.Event()
    failed=threading.Event()
    def monitor():
        encoder.wait()
        if not stopping.is_set():
            failed.set()
            server.shutdown()
    threading.Thread(target=monitor,daemon=True).start()
    def stop(*_):
        stopping.set()
        threading.Thread(target=server.shutdown,daemon=True).start()
    signal.signal(signal.SIGTERM,stop)
    signal.signal(signal.SIGINT,stop)
    try:
        server.serve_forever(poll_interval=.5)
    finally:
        stopping.set()
        server.server_close()
        encoder.terminate()
        try: encoder.wait(timeout=10)
        except subprocess.TimeoutExpired:
            encoder.kill(); encoder.wait()
    if failed.is_set():
        raise SystemExit(1)


if __name__=='__main__':
    main()
