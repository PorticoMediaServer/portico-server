import { useState } from 'react';
import { ChevronDown, Copy, FileVideo } from 'lucide-react';
import type { MediaVersion } from '../../data/types';
import { fmtBytes } from '../../lib/hooks';
import { cls } from '../../components/ui';
import { toast } from '../../components/overlay';

export function VersionsPanel({ versions }: { versions: MediaVersion[] }) {
  const [open, setOpen] = useState<string | null>(null);
  return (
    <div className="versions">
      {versions.map((v) => {
        const isOpen = open === v.id;
        return (
          <div key={v.id} className={cls('version', isOpen && 'open')}>
            <button className="version-head" onClick={() => setOpen(isOpen ? null : v.id)} aria-expanded={isOpen}>
              <span className="version-icon">
                <FileVideo />
              </span>
              <span className="version-main">
                <span className="version-title">{v.label}</span>
                <span className="version-sub tnum">
                  {v.videoCodec}
                  {v.hdr ? ` · ${v.hdr}` : ''} · {(v.bitrateKbps / 1000).toFixed(1)} Mbps · {v.container} · {fmtBytes(v.sizeBytes)}
                </span>
              </span>
              <ChevronDown className="version-chev" />
            </button>
            {isOpen && (
              <div className="version-body">
                <div className="version-path">
                  <code>{v.path}</code>
                  <button
                    className="icon-btn sm plain"
                    aria-label="Copy path"
                    onClick={() => {
                      navigator.clipboard?.writeText(v.path).catch(() => undefined);
                      toast('Path copied', <Copy />);
                    }}
                  >
                    <Copy />
                  </button>
                </div>
                <div className="track-cols">
                  <div>
                    <h4>Audio</h4>
                    <table className="tracks">
                      <tbody>
                        {v.audio.map((a) => (
                          <tr key={a.id}>
                            <td>{a.label}</td>
                            <td className="t-mono">{a.codec}</td>
                            <td className="t-mono">{a.channels}</td>
                            <td>{a.isDefault ? <span className="badge">Default</span> : a.commentary ? <span className="badge">Commentary</span> : null}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                  <div>
                    <h4>Subtitles</h4>
                    <table className="tracks">
                      <tbody>
                        {v.subtitles.map((s) => (
                          <tr key={s.id}>
                            <td>{s.label}</td>
                            <td className="t-mono">{s.format}</td>
                            <td>{s.forced ? <span className="badge">Forced</span> : s.sdh ? <span className="badge">SDH</span> : null}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
