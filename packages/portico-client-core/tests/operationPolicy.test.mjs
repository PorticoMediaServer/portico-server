import assert from "node:assert/strict";
import test from "node:test";
import {
  ApiError,
  PORTICO_OPERATION_POLICIES,
  PorticoTransportError,
  createHostedServicesClient,
  createMemorySessionStore,
  createPorticoClient,
  getOperationPolicy,
  isAmbiguousPorticoError,
  isRetryablePorticoError
} from "../dist/index.js";
import { PORTICO_OPERATION_POLICIES as coreOperationPolicies } from "../dist/core.js";

const operationClasses = [
  "interactive read",
  "interactive mutation",
  "form/upload",
  "polling",
  "discovery/probe",
  "long poll",
  "realtime stream",
  "media/stream transfer"
];

const expectedPolicies = {
  "interactive read": {
    operationClass: "interactive read",
    defaultDeadlineMs: 30_000,
    deadlineScope: "control-plane-response",
    retry: { eligible: true, defaultBudget: 2, ceiling: 4, mode: "request" },
    idempotencyRequirement: "idempotent",
    cancellation: "abort-request",
    timeout: { problemCode: "request_timeout", messageId: "problem.timeout", action: "retry" }
  },
  "interactive mutation": {
    operationClass: "interactive mutation",
    defaultDeadlineMs: 30_000,
    deadlineScope: "control-plane-response",
    retry: { eligible: false, defaultBudget: 0, ceiling: 0, mode: "none" },
    idempotencyRequirement: "reconcile-before-retry",
    cancellation: "abort-request-and-body",
    timeout: { problemCode: "request_timeout", messageId: "problem.timeout", action: "reconcile" }
  },
  "form/upload": {
    operationClass: "form/upload",
    defaultDeadlineMs: 30_000,
    deadlineScope: "control-plane-response",
    retry: { eligible: false, defaultBudget: 0, ceiling: 0, mode: "none" },
    idempotencyRequirement: "reconcile-before-retry",
    cancellation: "abort-request-and-body",
    timeout: { problemCode: "request_timeout", messageId: "problem.timeout", action: "reconcile" }
  },
  polling: {
    operationClass: "polling",
    defaultDeadlineMs: 15_000,
    deadlineScope: "control-plane-response",
    retry: { eligible: true, defaultBudget: 1, ceiling: 2, mode: "request" },
    idempotencyRequirement: "idempotent",
    cancellation: "abort-request-and-body",
    timeout: { problemCode: "request_timeout", messageId: "problem.timeout", action: "retry" }
  },
  "discovery/probe": {
    operationClass: "discovery/probe",
    defaultDeadlineMs: 10_000,
    deadlineScope: "control-plane-response",
    retry: { eligible: true, defaultBudget: 1, ceiling: 2, mode: "request" },
    idempotencyRequirement: "idempotent",
    cancellation: "abort-request-and-body",
    timeout: { problemCode: "request_timeout", messageId: "problem.timeout", action: "try-next-candidate" }
  },
  "long poll": {
    operationClass: "long poll",
    defaultDeadlineMs: 30_000,
    deadlineScope: "control-plane-response",
    retry: { eligible: true, defaultBudget: 0, ceiling: 2, mode: "reconnect" },
    idempotencyRequirement: "idempotent",
    cancellation: "abort-request-and-body",
    timeout: { problemCode: "request_timeout", messageId: "problem.timeout", action: "reconnect" }
  },
  "realtime stream": {
    operationClass: "realtime stream",
    defaultDeadlineMs: 15_000,
    deadlineScope: "stream-open",
    retry: { eligible: true, defaultBudget: 0, ceiling: 4, mode: "reconnect" },
    idempotencyRequirement: "idempotent",
    cancellation: "abort-stream",
    timeout: { problemCode: "request_timeout", messageId: "problem.timeout", action: "reconnect" }
  },
  "media/stream transfer": {
    operationClass: "media/stream transfer",
    defaultDeadlineMs: null,
    deadlineScope: "none",
    retry: { eligible: false, defaultBudget: 0, ceiling: 0, mode: "none" },
    idempotencyRequirement: "idempotent",
    cancellation: "abort-transfer",
    timeout: { problemCode: "request_timeout", messageId: "problem.timeout", action: "resume-or-cancel" }
  }
};

function wait(milliseconds) {
  return new Promise(resolve => setTimeout(resolve, milliseconds));
}

test("operation policy exposes exactly the eight public classes", () => {
  assert.strictEqual(coreOperationPolicies, PORTICO_OPERATION_POLICIES);
  assert.deepEqual(Object.keys(PORTICO_OPERATION_POLICIES), operationClasses);

  for (const operationClass of operationClasses) {
    const policy = getOperationPolicy(operationClass);
    assert.deepEqual(policy, expectedPolicies[operationClass]);
    assert.equal(Object.isFrozen(policy), true);
    assert.equal(Object.isFrozen(policy.retry), true);
    assert.equal(Object.isFrozen(policy.timeout), true);
  }

  assert.equal(getOperationPolicy("interactive read").defaultDeadlineMs, 30_000);
  assert.equal(getOperationPolicy("discovery/probe").defaultDeadlineMs, 10_000);
  assert.equal(getOperationPolicy("realtime stream").deadlineScope, "stream-open");
  assert.equal(getOperationPolicy("media/stream transfer").defaultDeadlineMs, null);
  assert.equal(getOperationPolicy("media/stream transfer").deadlineScope, "none");
  assert.throws(() => getOperationPolicy("legacy-read"), RangeError);
});

test("request deadlines and retry delays reject unsafe overrides and accept the ceiling", async () => {
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async () => new Response(JSON.stringify({ ok: true }), { status: 200, headers: { "Content-Type": "application/json" } })
    }
  });
  await assert.rejects(client.request("/api/system", { timeoutMs: 0 }), /Portico request timeout/);
  await client.request("/api/system", { timeoutMs: 300_000 });

  const hosted = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    retryBudget: 1,
    retryDelaysMs: [300_001],
    transport: {
      fetch: async () => new Response(JSON.stringify({ code: "busy", message: "try again" }), {
        status: 503,
        headers: { "Content-Type": "application/json" }
      })
    }
  });
  await assert.rejects(hosted.request("/api/system"), error => error instanceof PorticoTransportError && error.cause instanceof TypeError && error.cause.message.includes("retry delays"));
});

test("Hosted read retries stop at the operation policy ceiling", async () => {
  let attempts = 0;
  const client = createHostedServicesClient({
    hostedApiBaseUrl: "https://api.example",
    retryBudget: 4,
    retryDelaysMs: [0, 0, 0, 0],
    transport: {
      fetch: async () => {
        attempts += 1;
        return new Response(JSON.stringify({ code: "busy", message: "try again" }), {
          status: 503,
          headers: { "Content-Type": "application/json" }
        });
      }
    }
  });

  await assert.rejects(client.request("/api/system"), error => error instanceof ApiError && error.status === 503);
  assert.equal(attempts, 5);

  await assert.rejects(
    client.request("/api/system", { operationClass: "polling", retryBudget: 3 }),
    error => error instanceof PorticoTransportError
      && error.cause instanceof TypeError
      && error.cause.message.includes("retry budget")
  );
});

test("interactive mutation timeout is stable and ambiguous after dispatch", async () => {
  let transportSignal;
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    requestTimeoutMs: 15,
    transport: {
      fetch: async (_input, init) => {
        transportSignal = init.signal;
        return new Promise(() => {});
      }
    }
  });

  await assert.rejects(
    client.request("/api/settings", {
      method: "POST",
      body: { setting: "value" },
      operationClass: "interactive mutation"
    }),
    error => error instanceof PorticoTransportError
      && error.code === "request_timeout"
      && error.messageId === "problem.timeout"
      && isAmbiguousPorticoError(error)
      && !isRetryablePorticoError(error)
  );
  assert.equal(transportSignal.aborted, true);
});

test("pre-aborted mutation and form signals never dispatch", async () => {
  let calls = 0;
  const controller = new AbortController();
  controller.abort();
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async () => {
        calls += 1;
        return new Response(JSON.stringify({ ok: true }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
    }
  });

  await assert.rejects(
    client.request("/api/settings", {
      method: "POST",
      body: { setting: "value" },
      signal: controller.signal,
      operationClass: "interactive mutation"
    }),
    { name: "AbortError" }
  );

  const form = new FormData();
  form.set("displayName", "Aborted");
  await assert.rejects(client.uploadProfileImage(form, { signal: controller.signal }), { name: "AbortError" });
  assert.equal(calls, 0);
});

test("form upload timeout remains ambiguous and idempotent timeout remains retryable", async () => {
  let calls = 0;
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    transport: {
      fetch: async () => {
        calls += 1;
        return new Promise(() => {});
      }
    }
  });
  const form = new FormData();
  form.set("displayName", "Timed out");
  await assert.rejects(client.uploadProfileImage(form, { timeoutMs: 15 }), error => error instanceof PorticoTransportError && isAmbiguousPorticoError(error) && !isRetryablePorticoError(error));
  await assert.rejects(client.request("/api/system", { operationClass: "polling", timeoutMs: 15 }), error => error instanceof PorticoTransportError && !isAmbiguousPorticoError(error) && isRetryablePorticoError(error));
  assert.equal(calls, 2);
});

test("media transfer has no default body deadline but honors caller cancellation", async () => {
  let transportSignal;
  const controller = new AbortController();
  const client = createPorticoClient({
    apiBaseUrl: "https://server.example",
    sessionStore: createMemorySessionStore({
      apiBaseUrl: "https://server.example",
      accessToken: "access-token"
    }),
    transport: {
      fetch: async (_input, init) => {
        transportSignal = init.signal;
        return new Response(new ReadableStream({ start() {} }), { status: 200 });
      }
    }
  });

  const pending = client.bitrateTest(1, { signal: controller.signal });
  const beforeCancellation = await Promise.race([
    pending.then(() => "settled", error => error),
    wait(40).then(() => "still-pending")
  ]);
  assert.equal(beforeCancellation, "still-pending");

  controller.abort();
  await assert.rejects(pending, { name: "AbortError" });
  assert.equal(transportSignal.aborted, true);
});
