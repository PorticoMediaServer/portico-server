import type {
  Job,
  PlaybackSession,
  RemoteAccessStatus,
} from "@porticomediaserver/client-core";
import { PlaybackTechnicalStatsIcon, StatusWarningIcon, StatusSuccessIcon, NavigationExpandIcon, StatusErrorIcon, PlaybackQualityIcon, DeviceNetworkIcon, DeviceStorageIcon, PlaybackPlayIcon, ActionRefreshIcon, StatusActiveIcon } from "#portico-icons";
import { useMemo, useState } from "react";
import { reviewedProductErrorText } from "../../components/ProductLanguage";
import { Link } from "react-router-dom";
import { SecondaryButton } from "../../components/controls/Buttons";
import {
  InlineNotice,
  SettingsError,
  SettingsLoading,
} from "./SettingsControls";
import { useAbortableMutation, useSettingsQuery } from "./settingsHooks";
import type {
  SettingsDataSource,
  SettingsStatusSnapshot,
  SettingsViewer,
} from "./settingsTypes";

const loadStatus = (source: SettingsDataSource, signal: AbortSignal) =>
  source.settingsStatus(signal);

function bytes(value: number | undefined): string {
  if (value === undefined || !Number.isFinite(value)) return "Unavailable";
  if (value < 1024) return `${value} B`;
  const units = ["KB", "MB", "GB", "TB", "PB"];
  let size = value;
  let index = -1;
  do {
    size /= 1024;
    index += 1;
  } while (size >= 1024 && index < units.length - 1);
  return `${size >= 100 ? size.toFixed(0) : size >= 10 ? size.toFixed(1) : size.toFixed(2)} ${units[index]}`;
}

function duration(value: number): string {
  const seconds = Math.max(0, Math.floor(value));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainder = seconds % 60;
  return hours > 0
    ? `${hours}:${String(minutes).padStart(2, "0")}:${String(remainder).padStart(2, "0")}`
    : `${minutes}:${String(remainder).padStart(2, "0")}`;
}

function relativeTime(value: string): string {
  const elapsed = Date.now() - Date.parse(value);
  if (!Number.isFinite(elapsed) || elapsed < 0) return "just now";
  const minutes = Math.floor(elapsed / 60_000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

type TelemetryStatus = { status?: string; reason?: string } | undefined;
type TelemetryState = "ok" | "unavailable" | "warming_up" | "unsupported";
type TelemetryPresentation = {
  state: TelemetryState;
  label: string;
  reason?: string;
  ok: boolean;
};
type ActivityTelemetry = NonNullable<SettingsStatusSnapshot["activity"]> & {
  cpuStatus?: TelemetryStatus;
  memoryStatus?: TelemetryStatus;
};
type GPUTelemetrySample = NonNullable<
  NonNullable<NonNullable<SettingsStatusSnapshot["dashboard"]>["system"]>["gpu"]
>[number] & {
  usageStatus?: TelemetryStatus;
  memoryStatus?: TelemetryStatus;
  encoderStatus?: TelemetryStatus;
  headroomStatus?: TelemetryStatus;
};
type DashboardSystemTelemetry = NonNullable<
  NonNullable<SettingsStatusSnapshot["dashboard"]>["system"]
> & { gpu?: GPUTelemetrySample[] };

function telemetryState(status: TelemetryStatus): TelemetryState {
  const value = status?.status?.trim().toLowerCase();
  if (value === "ok") return "ok";
  if (value === "warming_up" || value === "warming-up") return "warming_up";
  if (value === "unsupported") return "unsupported";
  return "unavailable";
}

function telemetryStateLabel(state: TelemetryState): string {
  if (state === "ok") return "OK";
  if (state === "warming_up") return "Warming up";
  if (state === "unsupported") return "Unsupported";
  return "Unavailable";
}

function telemetryReason(status: TelemetryStatus, fallback: string): string {
  const reason = status?.reason?.trim();
  return reason || fallback;
}

function presentTelemetryMetric(
  value: number | undefined,
  status: TelemetryStatus,
  format: (value: number) => string,
  fallbackReason: string,
): TelemetryPresentation {
  const state = telemetryState(status);
  if (state === "ok" && value !== undefined && Number.isFinite(value))
    return { state, label: format(value), ok: true };
  const displayState = state === "ok" ? "unavailable" : state;
  return {
    state: displayState,
    label: telemetryStateLabel(displayState),
    reason: telemetryReason(status, fallbackReason),
    ok: false,
  };
}

function TelemetryReadout({
  metric,
  okDetail,
}: {
  metric: TelemetryPresentation;
  okDetail: string;
}) {
  const detail = metric.ok ? okDetail : metric.reason;
  return (
    <>
      <strong data-telemetry-status={metric.state}>{metric.label}</strong>
      <em title={detail}>{detail}</em>
    </>
  );
}

function TelemetryStatusRow({
  label,
  metric,
  icon: Icon,
  testId,
}: {
  label: string;
  metric: TelemetryPresentation;
  icon: typeof PlaybackTechnicalStatsIcon;
  testId: string;
}) {
  return (
    <div data-testid={testId}>
      <Icon />
      <span>
        <strong>{label}</strong>
        <small title={metric.ok ? metric.label : metric.reason}>
          {metric.ok ? metric.label : metric.reason}
        </small>
      </span>
      <b
        className={metric.ok ? "healthy" : ""}
        data-telemetry-status={metric.state}
      >
        {metric.ok ? telemetryStateLabel(metric.state) : metric.label}
      </b>
    </div>
  );
}

function remoteHealth(remote: RemoteAccessStatus | undefined): {
  status: string;
  label: string;
  detail: string;
  healthy: boolean;
} {
  if (!remote)
    return {
      status: "Unavailable",
      label: "Remote status unavailable",
      detail: "Portico couldn't load the direct-access result.",
      healthy: false,
    };
  if (!remote.settings.enabled)
    return {
      status: "Off",
      label: "Remote access is off",
      detail: "Open Connectivity settings to enable it.",
      healthy: false,
    };
  if (remote.settings.claimStatus !== "claimed")
    return {
      status: "Unavailable",
      label: "Direct access is unavailable",
      detail: "Connect this server to a Portico Account to enable direct access.",
      healthy: false,
    };
  const reachable = remote.connectivity.troubleshootingStatus === "ok";
  if (reachable)
    return {
      status: "Available",
      label: "Direct remote access is available",
      detail: remote.publicEndpoint.url || "Hosted Services can reach this server directly.",
      healthy: true,
    };
  if (remote.connectivity.troubleshootingStatus === "checking")
    return {
      status: "Checking",
      label: "Checking direct access",
      detail: "Portico is confirming that this server can be reached directly.",
      healthy: false,
    };
  if (remote.connectivity.troubleshootingStatus === "hosted_unreachable")
    return {
      status: "Unavailable",
      label: "Direct access is unavailable",
      detail: "This server can't reach Hosted Services. Check its internet connection, DNS, and firewall.",
      healthy: false,
    };
  if (remote.connectivity.troubleshootingStatus === "public_route_missing")
    return {
      status: "Unavailable",
      label: "Direct access is unavailable",
      detail: "Portico doesn't have a usable public route. Check the public port in Connectivity settings.",
      healthy: false,
    };
  return {
    status: "Unavailable",
    label: "Direct access is unavailable",
    detail:
      remote.connectivity.troubleshootingHint ||
      "Portico can't reach this server directly. Check router forwarding, firewall settings, or carrier-grade NAT.",
    healthy: false,
  };
}

function uniqueJobs(snapshot: SettingsStatusSnapshot): Job[] {
  const values = [
    ...(snapshot.dashboard?.jobs ?? []),
    ...(snapshot.jobs ?? []),
  ];
  return [...new Map(values.map((job) => [job.id, job])).values()].sort(
    (left, right) => Date.parse(right.updatedAt) - Date.parse(left.updatedAt),
  );
}

function ResourceLedger({ snapshot }: { snapshot: SettingsStatusSnapshot }) {
  const activity = snapshot.activity as ActivityTelemetry | undefined;
  const cpu = presentTelemetryMetric(
    activity?.cpuPercent,
    activity?.cpuStatus,
    (value) => `${Math.round(value)}%`,
    "No current CPU sample.",
  );
  const memory = presentTelemetryMetric(
    activity?.memoryUsedBytes,
    activity?.memoryStatus,
    (value) => `${bytes(value)} / ${bytes(activity?.memoryTotalBytes)}`,
    "No current memory sample.",
  );
  const memoryDetail =
    memory.ok && activity
      ? `${Math.round((activity.memoryUsedBytes / Math.max(activity.memoryTotalBytes, 1)) * 100)}% in use`
      : (memory.reason ?? "Memory telemetry is unavailable.");
  return (
    <section className="portico-resource-ledger" aria-label="Server resources">
      <div data-testid="server-resource-cpu">
        <PlaybackTechnicalStatsIcon />
        <span>
          <small>CPU</small>
          <TelemetryReadout
            metric={cpu}
            okDetail={
              activity
                ? `${activity.activeTranscodes} active ${activity.activeTranscodes === 1 ? "transcode" : "transcodes"}`
                : "No current sample."
            }
          />
        </span>
      </div>
      <div data-testid="server-resource-memory">
        <DeviceStorageIcon />
        <span>
          <small>Memory</small>
          <TelemetryReadout metric={memory} okDetail={memoryDetail} />
        </span>
      </div>
      <div>
        <DeviceStorageIcon />
        <span>
          <small>Portico data</small>
          <strong>{bytes(snapshot.storage?.totalBytes)}</strong>
          <em>
            {snapshot.storage
              ? `${snapshot.storage.categories.length} managed categories`
              : "Storage report unavailable"}
          </em>
        </span>
      </div>
      <div>
        <DeviceNetworkIcon />
        <span>
          <small>Outbound</small>
          <strong>
            {activity
              ? `${activity.bandwidthMbps.toFixed(1)} Mbps`
              : "Unavailable"}
          </strong>
          <em>
            {activity
              ? `${activity.activeStreams} active ${activity.activeStreams === 1 ? "stream" : "streams"}`
              : "No current sample"}
          </em>
        </span>
      </div>
    </section>
  );
}

function GPUTelemetry({ snapshot }: { snapshot: SettingsStatusSnapshot }) {
  const system = snapshot.dashboard?.system as
    DashboardSystemTelemetry | undefined;
  const sample = system?.gpu?.[system.gpu.length - 1];
  const info = system?.gpuInfo;
  const fallbackStatus: TelemetryStatus = sample
    ? undefined
    : system === undefined
      ? { status: "unavailable", reason: "GPU telemetry did not respond." }
      : info?.available === true
        ? {
            status: "warming_up",
            reason: info.note || "Waiting for the first GPU telemetry sample.",
          }
        : info?.available === false
          ? {
              status: "unsupported",
              reason:
                info.note ||
                "GPU hardware or driver telemetry is not exposed to this server.",
            }
          : {
              status: "unavailable",
              reason: "GPU telemetry did not report its availability.",
            };
  const usage = presentTelemetryMetric(
    sample?.usage,
    sample?.usageStatus ?? fallbackStatus,
    (value) => `${Math.round(value)}%`,
    "GPU usage is unavailable.",
  );
  const memory = presentTelemetryMetric(
    sample?.memory,
    sample?.memoryStatus ?? fallbackStatus,
    (value) => `${Math.round(value)}%`,
    "GPU memory telemetry is unavailable.",
  );
  const encoder = presentTelemetryMetric(
    sample?.encoder,
    sample?.encoderStatus ?? fallbackStatus,
    (value) => `${Math.round(value)}%`,
    "GPU encoder telemetry is unavailable.",
  );
  const headroom = presentTelemetryMetric(
    sample?.headroom,
    sample?.headroomStatus ?? fallbackStatus,
    (value) => `${Math.round(value)}%`,
    "GPU headroom telemetry is unavailable.",
  );
  const label =
    sample?.label || info?.device || info?.provider || "GPU telemetry";
  return (
    <section className="portico-status-section" aria-label="GPU telemetry">
      <header>
        <div>
          <h2>GPU telemetry</h2>
          <p>{label}</p>
        </div>
      </header>
      <div className="portico-health-list">
        <TelemetryStatusRow
          label="Usage"
          metric={usage}
          icon={PlaybackTechnicalStatsIcon}
          testId="gpu-telemetry-usage"
        />
        <TelemetryStatusRow
          label="Memory"
          metric={memory}
          icon={DeviceStorageIcon}
          testId="gpu-telemetry-memory"
        />
        <TelemetryStatusRow
          label="Encoder"
          metric={encoder}
          icon={PlaybackQualityIcon}
          testId="gpu-telemetry-encoder"
        />
        <TelemetryStatusRow
          label="Headroom"
          metric={headroom}
          icon={PlaybackQualityIcon}
          testId="gpu-telemetry-headroom"
        />
      </div>
    </section>
  );
}

function StreamRow({
  stream,
  expanded,
  busy,
  onExpand,
  onStop,
}: {
  stream: PlaybackSession;
  expanded: boolean;
  busy: boolean;
  onExpand: () => void;
  onStop: () => Promise<void>;
}) {
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState("");
  const runtime = stream.media.durationSeconds ?? 0;
  const artwork =
    stream.media.images?.thumb ||
    stream.media.images?.backdrop ||
    stream.media.images?.poster;
  const stop = async () => {
    setError("");
    try {
      await onStop();
      setConfirming(false);
    } catch (reason) {
      setError(
        reviewedProductErrorText(reason, "settings.action-failed", {
          actionName: "stop this stream",
        }),
      );
    }
  };
  return (
    <div className="portico-stream-entry">
      <button
        type="button"
        className={`portico-stream-row ${expanded ? "active" : ""}`}
        aria-expanded={expanded}
        onClick={onExpand}
      >
        <span className="portico-stream-title">
          <span className="portico-stream-art">
            {artwork ? <img src={artwork} alt="" /> : <PlaybackPlayIcon />}
          </span>
          <span>
            <strong>{stream.media.title}</strong>
            <small>
              {duration(stream.positionSeconds)}
              {runtime > 0 ? ` / ${duration(runtime)}` : ""} · {stream.state}
            </small>
          </span>
        </span>
        <span>
          <strong>{stream.user}</strong>
          <small>
            {stream.device} · {stream.app}
          </small>
        </span>
        <span>
          <strong>
            {stream.decision || stream.transcode?.method || "Playback"}
          </strong>
          <small>
            {stream.videoTarget ||
              stream.transcode?.quality ||
              stream.videoDecision ||
              "Source quality"}
          </small>
        </span>
        <span>{stream.bandwidthMbps.toFixed(1)} Mbps</span>
        <NavigationExpandIcon />
      </button>
      {expanded && (
        <div className="portico-stream-disclosure">
          <dl>
            <div>
              <dt>Connection</dt>
              <dd>
                {stream.location} · {stream.location === "Unknown" ? "Route unavailable" : "Route class reported"}
              </dd>
            </div>
            <div>
              <dt>Video</dt>
              <dd>
                {[stream.videoSource, stream.videoDecision, stream.videoTarget]
                  .filter(Boolean)
                  .join(" → ") || "Not reported"}
              </dd>
            </div>
            <div>
              <dt>Audio</dt>
              <dd>
                {[stream.audioSource, stream.audioDecision, stream.audioTarget]
                  .filter(Boolean)
                  .join(" → ") || "Not reported"}
              </dd>
            </div>
            <div>
              <dt>Started</dt>
              <dd>{relativeTime(stream.startedAt)}</dd>
            </div>
          </dl>
          {error && (
            <p className="portico-stream-error" role="alert">
              {error}
            </p>
          )}
          <div>
            {confirming ? (
              <>
                <span>Stop playback on {stream.device}?</span>
                <SecondaryButton
                  disabled={busy}
                  onClick={() => setConfirming(false)}
                >
                  Cancel
                </SecondaryButton>
                <button
                  type="button"
                  className="button secondary portico-destructive-button"
                  disabled={busy}
                  onClick={() => void stop()}
                >
                  <StatusActiveIcon />
                  {busy ? "Stopping…" : "Stop stream"}
                </button>
              </>
            ) : (
              <button
                type="button"
                className="button secondary portico-destructive-button"
                onClick={() => setConfirming(true)}
              >
                <StatusActiveIcon /> Stop stream
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function ActiveStreams({
  snapshot,
  source,
  onChanged,
}: {
  snapshot: SettingsStatusSnapshot;
  source: SettingsDataSource;
  onChanged: () => void;
}) {
  const streams = snapshot.dashboard?.nowPlaying ?? [];
  const [expanded, setExpanded] = useState<string>(streams[0]?.id ?? "");
  const { busy, run } = useAbortableMutation();
  return (
    <section className="portico-status-section">
      <header>
        <div>
          <h2>Now playing</h2>
          <p>
            {streams.length} active{" "}
            {streams.length === 1 ? "stream" : "streams"} ·{" "}
            {snapshot.activity?.activeTranscodes ??
              snapshot.dashboard?.transcodes.length ??
              0}{" "}
            transcoding
          </p>
        </div>
      </header>
      {streams.length === 0 ? (
        <div className="portico-status-empty">
          <StatusErrorIcon />
          <span>
            <strong>No active streams</strong>
            <p>Playback sessions will appear here while they are active.</p>
          </span>
        </div>
      ) : (
        <div className="portico-stream-table">
          <div className="portico-stream-table-head">
            <span>Title</span>
            <span>Member and device</span>
            <span>Playback</span>
            <span>Bandwidth</span>
            <span />
          </div>
          {streams.map((stream) => (
            <StreamRow
              key={stream.id}
              stream={stream}
              expanded={expanded === stream.id}
              busy={busy}
              onExpand={() =>
                setExpanded((current) =>
                  current === stream.id ? "" : stream.id,
                )
              }
              onStop={async () => {
                try {
                  await run((signal) => source.stopPlayback(stream, signal));
                } finally {
                  onChanged();
                }
              }}
            />
          ))}
        </div>
      )}
    </section>
  );
}

function ConnectivityLedger({
  remote,
}: {
  remote: RemoteAccessStatus | undefined;
}) {
  const direct = remoteHealth(remote);
  return (
    <section className="portico-status-section">
      <header>
        <div>
          <h2>Connectivity</h2>
          <p>Remote availability</p>
        </div>
      </header>
      <div className="portico-health-list">
        <div>
          {direct.healthy ? <StatusSuccessIcon /> : <DeviceNetworkIcon />}
          <span>
            <strong>Direct access</strong>
            <small>{direct.detail}</small>
          </span>
          <b className={direct.healthy ? "healthy" : ""}>{direct.status}</b>
        </div>
      </div>
      <Link
        className="portico-settings-inline-link"
        to="/settings/connectivity"
      >
        Open connectivity settings
      </Link>
    </section>
  );
}

function WorkLedger({ snapshot }: { snapshot: SettingsStatusSnapshot }) {
  const jobs = useMemo(
    () => uniqueJobs(snapshot).filter((job) => job.status === 'queued' || job.status === 'running').slice(0, 6),
    [snapshot],
  );
  const progressLabel = (job: Job): string | undefined => {
    const current = job.progressCurrent;
    const total = job.progressTotal;
    if (typeof current === 'number' && Number.isFinite(current) && typeof total === 'number' && Number.isFinite(total) && total > 0) {
      return `${Math.round(Math.max(0, Math.min(1, current / total)) * 100)}%`;
    }
    if (Number.isFinite(job.progress) && job.progress > 0) return `${Math.max(0, Math.min(100, Math.round(job.progress)))}%`;
    return undefined;
  };
  const jobTitle = (job: Job): string => {
    const library = job.metadata?.libraryName?.trim();
    if (job.type === 'library_scan') return `${job.status === 'queued' ? 'Waiting to scan' : 'Scanning'}${library ? ` ${library}` : ' library'}`;
    if (job.type === 'metadata_refresh_library') return `${job.status === 'queued' ? 'Waiting to refresh metadata for' : 'Refreshing metadata for'}${library ? ` ${library}` : ' library'}`;
    if (job.type === 'metadata_refresh') return `${job.status === 'queued' ? 'Waiting to refresh' : 'Refreshing'}${job.metadata?.mediaTitle ? ` ${job.metadata.mediaTitle}` : ' metadata'}`;
    const readable = job.type.replaceAll('_', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase());
    return job.status === 'queued' ? `Waiting: ${readable}` : readable;
  };
  const jobDetail = (job: Job): string => {
    const message = job.message.trim().replace(/[.]+$/, '');
    if (message && message.toLocaleLowerCase() !== jobTitle(job).toLocaleLowerCase()) return message;
    return job.status === 'queued' ? 'Waiting for available capacity.' : 'In progress.';
  };
  return (
    <section className="portico-status-section">
      <header>
        <div>
          <h2>Work</h2>
          <p>In progress</p>
        </div>
      </header>
      {jobs.length === 0 ? (
        <div className="portico-status-empty compact">
          <PlaybackTechnicalStatsIcon />
          <span>
            <strong>No active server work</strong>
            <p>Background work will appear here while it is running.</p>
          </span>
        </div>
      ) : (
        <div className="portico-work-list">
          {jobs.map((job) => (
            <div key={job.id}>
              <span className={`portico-job-state ${job.status}`}>
                <PlaybackTechnicalStatsIcon />
              </span>
              <span>
                <strong>{jobTitle(job)}{progressLabel(job) ? ` · ${progressLabel(job)}` : ''}</strong>
                <small>{jobDetail(job)}</small>
              </span>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function AlertsLedger({ snapshot }: { snapshot: SettingsStatusSnapshot }) {
  const alerts = snapshot.dashboard?.alerts ?? [];
  return (
    <section className="portico-status-section">
      <header>
        <div>
          <h2>Needs attention</h2>
          <p>
            {alerts.length
              ? `${alerts.length} ${alerts.length === 1 ? "alert" : "alerts"}`
              : "No active alerts"}
          </p>
        </div>
      </header>
      {alerts.length > 0 ? (
        <div className="portico-alert-list">
          {alerts.map((alert) => (
            <div className={alert.level} key={alert.id}>
              <StatusWarningIcon />
              <span>
                <strong>{alert.title}</strong>
                <small>
                  {alert.message} · {relativeTime(alert.time)}
                </small>
              </span>
            </div>
          ))}
        </div>
      ) : (
        <div className="portico-status-empty compact healthy">
          <StatusSuccessIcon />
          <span>
            <strong>No action is required</strong>
            <p>Portico has not reported any active server alerts.</p>
          </span>
        </div>
      )}
    </section>
  );
}

export function StatusDashboard({
  source,
  viewer,
}: {
  source: SettingsDataSource;
  viewer: SettingsViewer;
}) {
  const [revision, setRevision] = useState(0);
  const [checkError, setCheckError] = useState("");
  const state = useSettingsQuery(loadStatus, source, revision, { refreshIntervalMs: 5_000, keepPreviousData: true });
  const checks = useAbortableMutation();
  if (state.status === "loading")
    return <SettingsLoading label="Loading server status" />;
  if (state.status === "error")
    return (
      <SettingsError
        title="Server status is unavailable"
        message={reviewedProductErrorText(state.error, "settings.load-failed", {
          sectionName: "Server status",
        })}
        onRetry={() => setRevision((current) => current + 1)}
      />
    );
  const snapshot = state.data;
  const health = remoteHealth(snapshot.remoteAccess);
  const failedPanels = Object.keys(snapshot.failures ?? {});
  const serverName = snapshot.activity?.serverName || viewer.serverName;
  const statusHeadline = failedPanels.length > 0
    ? `${serverName} status is partially unavailable`
    : `${serverName} status`;
  return (
    <div className="portico-status-dashboard">
      <div
        className={`portico-status-command ${health.healthy ? "healthy" : "warn"}`}
      >
        <div>
          {health.healthy ? <StatusSuccessIcon /> : <StatusWarningIcon />}
          <span>
            <strong>{statusHeadline}</strong>
            <small>
              {health.label} · {health.detail}
            </small>
          </span>
        </div>
        {viewer.role !== "user" && (
          <SecondaryButton
            disabled={checks.busy}
            onClick={() => {
              setCheckError("");
              void checks
                .run((signal) => source.runConnectivityCheck(signal))
                .then(
                  () => setRevision((current) => current + 1),
                  (reason) =>
                    setCheckError(
                      reviewedProductErrorText(
                        reason,
                        "settings.action-failed",
                        { actionName: "complete the connectivity check" },
                      ),
                    ),
                );
            }}
          >
            <ActionRefreshIcon
              className={checks.busy ? "portico-settings-spinner" : ""}
            />
            {checks.busy ? "Checking…" : "Run checks"}
          </SecondaryButton>
        )}
      </div>
      {checkError && <InlineNotice tone="error">{checkError}</InlineNotice>}
      {Object.keys(snapshot.failures ?? {}).length > 0 && (
        <InlineNotice tone="warn">
          Some server status sources could not be refreshed. Available sections
          are shown with their most recent data.
        </InlineNotice>
      )}
      <ResourceLedger snapshot={snapshot} />
      <GPUTelemetry snapshot={snapshot} />
      <ActiveStreams
        snapshot={snapshot}
        source={source}
        onChanged={() => setRevision((current) => current + 1)}
      />
      <div className="portico-status-columns">
        <ConnectivityLedger remote={snapshot.remoteAccess} />
        <WorkLedger snapshot={snapshot} />
      </div>
      <AlertsLedger snapshot={snapshot} />
    </div>
  );
}
