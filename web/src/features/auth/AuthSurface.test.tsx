import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StrictMode } from "react";
import { SetupSurface } from "./AuthSurface";

const auth = vi.hoisted(() => ({
  busy: false,
  porticoSetupStatus: vi.fn(),
  setup: vi.fn(),
  startPorticoSetup: vi.fn(),
}));

vi.mock("../../data/DataProvider", () => ({
  useAuthSession: () => auth,
}));

vi.mock("../../runtime/RuntimeContext", () => ({
  useOptionalRuntime: () => undefined,
}));

describe("SetupSurface Portico Account recovery", () => {
  beforeEach(() => {
    auth.porticoSetupStatus.mockReset();
    auth.setup.mockReset();
    auth.startPorticoSetup.mockReset();
    window.history.replaceState(null, "", "/");
    window.sessionStorage.clear();
  });

  it("explains the loopback-only first-run boundary without offering a futile retry", async () => {
    auth.porticoSetupStatus.mockRejectedValue({ code: "setup_local_access_required", status: 403 });
    render(<SetupSurface serverName="Fresh Portico Server" />);
    expect(await screen.findByText("Finish setup on the server computer")).toBeInTheDocument();
    expect(screen.getByText(/http:\/\/127\.0\.0\.1:32600/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Try again" })).not.toBeInTheDocument();
    expect(screen.queryByText(/account or profile/i)).not.toBeInTheDocument();
  });

  it("checks plain-root state before enabling a new setup choice", async () => {
    let resolveStatus:
      | ((status: {
          setupRequired: boolean;
          claimStatus: string;
          porticoConnected: boolean;
        }) => void)
      | undefined;
    auth.porticoSetupStatus.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveStatus = resolve;
        }),
    );

    render(<SetupSurface serverName="Portico Public Demo" />);

    expect(screen.getByText("Checking server setup…")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Use A Portico Account/i }),
    ).not.toBeInTheDocument();

    resolveStatus?.({
      setupRequired: true,
      claimStatus: "not_claimed",
      porticoConnected: false,
    });

    expect(
      await screen.findByRole("button", { name: /Use A Portico Account/i }),
    ).toBeEnabled();
  });

  it("completes the plain-root setup check when StrictMode replays the effect", async () => {
    auth.porticoSetupStatus.mockResolvedValue({
      setupRequired: true,
      claimStatus: "not_claimed",
      porticoConnected: false,
    });

    render(
      <StrictMode>
        <SetupSurface serverName="Portico Public Demo" />
      </StrictMode>,
    );

    expect(
      await screen.findByRole("button", { name: /Use A Portico Account/i }),
    ).toBeEnabled();
    expect(auth.porticoSetupStatus).toHaveBeenCalledTimes(2);
    expect(screen.queryByText("Checking server setup…")).not.toBeInTheDocument();
  });

  it("resumes an already-claimed setup from plain root without starting a second claim", async () => {
    auth.porticoSetupStatus
      .mockResolvedValueOnce({
        setupRequired: true,
        claimStatus: "claimed",
        porticoConnected: true,
      })
      .mockResolvedValue({
        setupRequired: true,
        claimStatus: "claimed",
        porticoConnected: true,
      });

    render(<SetupSurface serverName="Portico Public Demo" />);

    expect(await screen.findByText("Finishing server setup…")).toBeInTheDocument();
    expect(auth.startPorticoSetup).not.toHaveBeenCalled();
    expect(
      screen.queryByRole("button", { name: /Use A Portico Account/i }),
    ).not.toBeInTheDocument();
    await waitFor(() => expect(auth.porticoSetupStatus).toHaveBeenCalledTimes(2));
  });

  it("treats owner-link-pending as authoritative evidence of resumable setup", async () => {
    auth.porticoSetupStatus
      .mockRejectedValueOnce(
        Object.assign(new Error("owner sync pending"), {
          code: "owner_link_pending",
          status: 503,
          retryable: true,
        }),
      )
      .mockResolvedValue({
        setupRequired: true,
        claimStatus: "claimed",
        porticoConnected: true,
      });

    render(<SetupSurface serverName="Portico Public Demo" />);

    expect(await screen.findByText("Finishing server setup…")).toBeInTheDocument();
    expect(auth.startPorticoSetup).not.toHaveBeenCalled();
    await waitFor(() => expect(auth.porticoSetupStatus).toHaveBeenCalledTimes(2));
  });

  it("preserves the explicit return-query recovery path", async () => {
    window.history.replaceState(null, "", "/?porticoSetup=continue");
    auth.porticoSetupStatus.mockResolvedValue({
      setupRequired: true,
      claimStatus: "claimed",
      porticoConnected: true,
    });

    render(<SetupSurface serverName="Portico Public Demo" />);

    expect(await screen.findByText("Finishing server setup…")).toBeInTheDocument();
    expect(auth.startPorticoSetup).not.toHaveBeenCalled();
    expect(auth.porticoSetupStatus).toHaveBeenCalledOnce();
  });
});
