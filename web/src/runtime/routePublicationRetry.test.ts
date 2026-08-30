import { describe, expect, it } from "vitest";
import { ROUTE_PUBLICATION_RETRY_HORIZON_MS, routePublicationRetryDelay, routePublicationRetryPlan } from "./routePublicationRetry";

describe("routePublicationRetryDelay", () => {
  it("uses stable equal jitter and caps prolonged setup polling", () => {
    const first = routePublicationRetryDelay(0, "installation", "server");
    expect(first).toBeGreaterThanOrEqual(1_001);
    expect(first).toBeLessThanOrEqual(2_000);
    expect(routePublicationRetryDelay(0, "installation", "server")).toBe(first);
    expect(routePublicationRetryDelay(99, "installation", "server")).toBeGreaterThanOrEqual(15_001);
    expect(routePublicationRetryDelay(99, "installation", "server")).toBeLessThanOrEqual(30_000);
  });

  it("keeps setup recovery eligible beyond the observed 102-second convergence and stops at three minutes", () => {
    const observedConvergenceMs = 102_000;
    const plan = routePublicationRetryPlan(8, observedConvergenceMs, "installation", "server");
    expect(plan?.delayMs).toBeGreaterThan(0);
    expect(plan?.delayMs).toBeLessThanOrEqual(30_000);
    expect(routePublicationRetryPlan(20, ROUTE_PUBLICATION_RETRY_HORIZON_MS, "installation", "server")).toBeUndefined();
  });
});
