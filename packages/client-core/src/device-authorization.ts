import {randomId} from './random-id.ts';
import { ApiError, HttpLocalApi, type HostedSession } from "./index.ts";
export type DeviceAuthorization = {
  deviceCode: string;
  userCode: string;
  verificationUri: string;
  verificationUriComplete: string;
  expiresIn: number;
  expiresAt: string;
  interval: number;
};
export interface DeviceAuthorizationApi {
  create(
    requestId: string,
    deviceName: string,
    platform: string,
    appVersion: string,
    signal?: AbortSignal
  ): Promise<DeviceAuthorization>;
  poll(deviceCode: string, signal?: AbortSignal): Promise<HostedSession>;
  cancel(deviceCode: string): Promise<void>;
  revoke(session: HostedSession): Promise<void>;
}
export class HttpDeviceAuthorizationApi implements DeviceAuthorizationApi {
  private http: HttpLocalApi;
  private baseUrl: string;
  constructor(baseUrl: string) {
    this.baseUrl = baseUrl;
    this.http = new HttpLocalApi(baseUrl);
  }
  create(
    requestId: string,
    deviceName: string,
    platform: string,
    appVersion: string,
    signal?: AbortSignal
  ) {
    return this.http.request<DeviceAuthorization>(
      "/v1/device-authorizations",
      "POST",
      { requestId, deviceName, platform, appVersion },
      signal
    );
  }
  poll(deviceCode: string, signal?: AbortSignal) {
    return this.http.request<HostedSession>(
      "/v1/device-authorizations/token",
      "POST",
      { deviceCode },
      signal
    );
  }
  cancel(deviceCode: string) {
    return this.http.request<void>("/v1/device-authorizations/cancel", "POST", {
      deviceCode,
    });
  }
  revoke(session: HostedSession) {
    return new HttpLocalApi(this.baseUrl, session.accessToken).request<void>(
      "/v1/sessions/current",
      "DELETE"
    );
  }
}
export type DeviceAuthorizationSnapshot = {
  phase: "idle" | "creating" | "waiting" | "approved" | "error";
  userCode?: string;
  verificationUri?: string;
  verificationUriComplete?: string;
  expiresAt?: string;
  error?: string;
  session?: HostedSession;
};
/** Device secrets never leave this owner; presentation receives only public code/URI. */
export class DeviceAuthorizationService {
  private state: DeviceAuthorizationSnapshot = { phase: "idle" };
  private listeners = new Set<() => void>();
  private generation = 0;
  private secret?: string;
  private timer?: ReturnType<typeof setTimeout>;
  private controller?: AbortController;
  private interval = 5000;
  private expires = 0;
  private api: DeviceAuthorizationApi;
  private timing: { minimumPollMs: number; requestTimeoutMs: number };
  private requestId: () => Promise<string>;
  constructor(
    api: DeviceAuthorizationApi,
    timing = { minimumPollMs: 5000, requestTimeoutMs: 15000 },
    requestId: () => Promise<string> = async () =>
      randomId()
  ) {
    this.api = api;
    this.timing = timing;
    this.requestId = requestId;
  }
  getSnapshot = () => this.state;
  subscribe = (fn: () => void) => {
    this.listeners.add(fn);
    return () => {
      this.listeners.delete(fn);
    };
  };
  private update(value: DeviceAuthorizationSnapshot) {
    this.state = value;
    this.listeners.forEach((fn) => fn());
  }
  private async bounded<T>(
    work: (signal: AbortSignal) => Promise<T>
  ): Promise<T> {
    const c = new AbortController();
    this.controller = c;
    let timer: ReturnType<typeof setTimeout>;
    try {
      return await Promise.race([
        work(c.signal),
        new Promise<never>((_, reject) => {
          timer = setTimeout(() => {
            c.abort();
            reject(new Error("The activation request timed out."));
          }, this.timing.requestTimeoutMs);
        }),
      ]);
    } finally {
      clearTimeout(timer!);
      if (this.controller === c) this.controller = undefined;
    }
  }
  async start(deviceName: string, platform: string, appVersion = "0.1.0") {
    this.cancel();
    const generation = this.generation;
    this.update({ phase: "creating" });
    try {
      const requestId = await this.requestId();
      if (generation !== this.generation) return;
      const create = () =>
        this.bounded((signal) =>
          this.api
            .create(requestId, deviceName, platform, appVersion, signal)
            .then((value) => {
              if (generation !== this.generation)
                void this.api.cancel(value.deviceCode).catch(() => {});
              return value;
            })
        );
      let created: DeviceAuthorization;
      try {
        created = await create();
      } catch (e) {
        if (generation !== this.generation) return;
        if (e instanceof ApiError && !e.retryable) throw e;
        created = await create(); // Recover the same receipt after an ambiguous transport result.
      }
      if (generation !== this.generation) return;
      this.secret = created.deviceCode;
      this.interval = Math.max(
        this.timing.minimumPollMs,
        created.interval * 1000
      );
      this.expires = Date.parse(created.expiresAt);
      if (!Number.isFinite(this.expires) || this.expires <= Date.now())
        throw new Error("This activation code has expired.");
      this.update({
        phase: "waiting",
        userCode: created.userCode,
        verificationUri: created.verificationUri,
        verificationUriComplete: created.verificationUriComplete,
        expiresAt: created.expiresAt,
      });
      this.schedule(generation);
    } catch (e) {
      if (generation === this.generation) {
        ++this.generation;
        const secret = this.secret;
        this.secret = undefined;
        if (secret) void this.api.cancel(secret).catch(() => {});
        this.update({
          phase: "error",
          error: e instanceof Error ? e.message : "Activation is unavailable.",
        });
      }
    }
  }
  private schedule(generation: number) {
    this.timer = setTimeout(
      () => void this.poll(generation),
      Math.min(this.interval, Math.max(0, this.expires - Date.now()))
    );
  }
  private async poll(generation: number) {
    if (generation !== this.generation || !this.secret) return;
    if (Date.now() >= this.expires) {
      this.cancel();
      this.update({
        phase: "error",
        error: "This activation code has expired. Generate a new code.",
      });
      return;
    }
    try {
      const session = await this.bounded((signal) =>
        this.api.poll(this.secret!, signal).then((value) => {
          if (generation !== this.generation)
            void this.api.revoke(value).catch(() => {});
          return value;
        })
      );
      if (generation !== this.generation) return;
      this.update({ phase: "approved", session });
    } catch (e) {
      if (generation !== this.generation) return;
      if (
        e instanceof ApiError &&
        ["access_denied", "expired_token", "invalid_grant"].includes(e.code)
      ) {
        this.cancel();
        this.update({ phase: "error", error: e.message });
        return;
      }
      if (e instanceof ApiError && e.code === "slow_down")
        this.interval += 5000;
      if (e instanceof ApiError && e.retryAfterSeconds)
        this.interval = Math.max(this.interval, e.retryAfterSeconds * 1000);
      if (
        e instanceof ApiError &&
        !["authorization_pending", "slow_down"].includes(e.code) &&
        !e.retryable &&
        e.status !== 429
      ) {
        this.cancel();
        this.update({ phase: "error", error: e.message });
        return;
      }
      this.schedule(generation);
    }
  }
  accept(session: HostedSession): boolean {
    if (this.state.phase !== "approved" || this.state.session !== session) return false;
    this.secret = undefined;
    return true;
  }
  cancel() {
    ++this.generation;
    clearTimeout(this.timer);
    this.controller?.abort();
    const secret = this.secret;
    this.secret = undefined;
    if (secret) void this.api.cancel(secret).catch(() => {});
    this.update({ phase: "idle" });
  }
}
