#!/usr/bin/env python3
"""Serve the private generated live source; child encoder follows service lifetime."""
from pathlib import Path
import argparse, functools, http.server, json, signal, subprocess, threading


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('root',type=Path)
    parser.add_argument('--ffmpeg',default='/opt/portico-ffmpeg/bin/ffmpeg')
    parser.add_argument('--port',type=int,default=32511)
    parser.add_argument('--webhook-port',type=int,default=32512)
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
    counts={'received':0}
    lock=threading.Lock()
    class Sink(http.server.BaseHTTPRequestHandler):
        def log_message(self,*_): pass
        def do_GET(self):
            payload=json.dumps(counts).encode()
            self.send_response(200);self.send_header('Content-Type','application/json')
            self.send_header('Content-Length',str(len(payload)));self.end_headers();self.wfile.write(payload)
        def do_POST(self):
            try: length=int(self.headers.get('Content-Length','0'))
            except ValueError: self.send_error(400);return
            if not 0<=length<=1024*1024: self.send_error(413);return
            if self.path not in ('/portico-webhook','/archive'): self.send_error(404);return
            self.rfile.read(length)  # Deliberately discard bodies and authentication headers.
            with lock: counts['received']+=1
            self.send_response(204);self.send_header('Content-Length','0');self.end_headers()
    sink=http.server.ThreadingHTTPServer(('127.0.0.1',args.webhook_port),Sink)
    sink.daemon_threads=True
    threading.Thread(target=sink.serve_forever,daemon=True).start()
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
        sink.shutdown();sink.server_close()
        encoder.terminate()
        try: encoder.wait(timeout=10)
        except subprocess.TimeoutExpired:
            encoder.kill(); encoder.wait()
    if failed.is_set():
        raise SystemExit(1)


if __name__=='__main__':
    main()
