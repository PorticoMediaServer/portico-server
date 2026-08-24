import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { FixtureSettingsDataSource } from "./FixtureSettingsDataSource";
import { FixturePorticoDataSource } from "../../data/fixtureSource";
import { DataProvider } from "../../data/DataProvider";
import { AccountSettings } from "./PersonalSettings";
import { SettingsPage } from "./SettingsPage";
import type { SettingsViewer } from "./settingsTypes";

const porticoViewer: SettingsViewer = {
  id: "fixture-owner",
  displayName: "Portico Review",
  email: "review@portico.local",
  role: "owner",
  serverName: "EhlerFlix Test",
  authOrigin: "portico",
  authProvider: "portico",
  hasLocalPassword: true,
  permissions: { manageServer: true },
};

describe("personal account and security Settings", () => {
  it("updates Portico identity, image, password, MFA, and exposes recovery codes", async () => {
    const source = new FixtureSettingsDataSource();
    const identity = vi.spyOn(source, "updateAccountIdentity");
    const image = vi.spyOn(source, "uploadAccountImage");
    const password = vi.spyOn(source, "changePorticoPassword");
    const enableMFA = vi.spyOn(source, "enablePorticoMFA");
    const rotateRecoveryCodes = vi.spyOn(
      source,
      "rotatePorticoMFARecoveryCodes",
    );
    render(<AccountSettings viewer={porticoViewer} source={source} />);

    expect(await screen.findByText("Living Room TV")).toBeInTheDocument();
    fireEvent.change(screen.getByRole("textbox", { name: "Username" }), {
      target: { value: "justinehler" },
    });
    expect(screen.getByRole("textbox", { name: "Email" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() =>
      expect(identity).toHaveBeenCalledWith(
        "portico",
        { displayName: "justinehler", email: "review@portico.local" },
        expect.any(AbortSignal),
      ),
    );

    const upload = screen.getByLabelText("Upload profile image");
    const file = new File(["profile-image"], "profile.png", {
      type: "image/png",
    });
    fireEvent.change(upload, { target: { files: [file] } });
    await waitFor(() =>
      expect(image).toHaveBeenCalledWith(
        "portico",
        file,
        expect.any(AbortSignal),
      ),
    );

    fireEvent.change(screen.getByLabelText("Current password (if set)"), {
      target: { value: "current-password" },
    });
    fireEvent.change(screen.getByLabelText("New password"), {
      target: { value: "A-new-passphrase-123" },
    });
    fireEvent.change(screen.getByLabelText("Confirm new password"), {
      target: { value: "A-new-passphrase-123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Change password" }));
    await waitFor(() =>
      expect(password).toHaveBeenCalledWith(
        {
          currentPassword: "current-password",
          newPassword: "A-new-passphrase-123",
        },
        expect.any(AbortSignal),
      ),
    );

    expect(await screen.findByText("Not enabled")).toBeInTheDocument();
    fireEvent.change(
      screen.getByLabelText(
        "Portico Account password for two-factor setup (if set)",
      ),
      { target: { value: "current-password" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "Start setup" }));
    expect(
      await screen.findByDisplayValue("PORTICOSETUPKEY42"),
    ).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Two-factor verification code"), {
      target: { value: "123456" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Enable two-factor" }));
    await waitFor(() =>
      expect(enableMFA).toHaveBeenCalledWith(
        { code: "123456", enrollmentToken: "fixture-enrollment-token" },
        expect.any(AbortSignal),
      ),
    );
    expect(
      (
        (await screen.findByRole("textbox", {
          name: "New two-factor recovery codes",
        })) as HTMLTextAreaElement
      ).value,
    ).toContain("PORTICO-7V9K-2N4Q");
    fireEvent.click(screen.getByRole("button", { name: "Create new codes" }));
    fireEvent.change(
      screen.getByLabelText("Authenticator code to replace recovery codes"),
      { target: { value: "654321" } },
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Replace recovery codes" }),
    );
    await waitFor(() =>
      expect(rotateRecoveryCodes).toHaveBeenCalledWith(
        "654321",
        expect.any(AbortSignal),
      ),
    );
    expect(
      (
        (await screen.findByRole("textbox", {
          name: "New two-factor recovery codes",
        })) as HTMLTextAreaElement
      ).value,
    ).toContain("PORTICO-RECOVERY-01");
  });

  it("allows a recently reauthenticated SSO account to establish its first password", async () => {
    const source = new FixtureSettingsDataSource();
    const password = vi.spyOn(source, "changePorticoPassword");
    render(<AccountSettings viewer={porticoViewer} source={source} />);

    fireEvent.change(screen.getByLabelText("New password"), {
      target: { value: "A-new-passphrase-123" },
    });
    fireEvent.change(screen.getByLabelText("Confirm new password"), {
      target: { value: "A-new-passphrase-123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Change password" }));

    await waitFor(() =>
      expect(password).toHaveBeenCalledWith(
        { currentPassword: "", newPassword: "A-new-passphrase-123" },
        expect.any(AbortSignal),
      ),
    );
  });

  it("changes a local password without calling Portico Account security", async () => {
    const source = new FixtureSettingsDataSource();
    const localPassword = vi.spyOn(source, "changeLocalPassword");
    const porticoPassword = vi.spyOn(source, "changePorticoPassword");
    const mfa = vi.spyOn(source, "porticoMFAStatus");
    render(
      <AccountSettings
        viewer={{
          ...porticoViewer,
          authOrigin: "local",
          authProvider: "local",
        }}
        source={source}
      />,
    );

    fireEvent.change(screen.getByLabelText("Current password"), {
      target: { value: "current-password" },
    });
    fireEvent.change(screen.getByLabelText("New password"), {
      target: { value: "A-new-local-passphrase" },
    });
    fireEvent.change(screen.getByLabelText("Confirm new password"), {
      target: { value: "A-new-local-passphrase" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Change password" }));

    await waitFor(() =>
      expect(localPassword).toHaveBeenCalledWith(
        {
          currentPassword: "current-password",
          newPassword: "A-new-local-passphrase",
        },
        expect.any(AbortSignal),
      ),
    );
    expect(porticoPassword).not.toHaveBeenCalled();
    expect(
      await screen.findByText("This Server password changed."),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("Two-factor authentication"),
    ).not.toBeInTheDocument();
    expect(await screen.findByText("Living Room TV")).toBeInTheDocument();
    expect(mfa).not.toHaveBeenCalled();
  });

  it("keeps local password enrollment with a server administrator when no credential exists", () => {
    const source = new FixtureSettingsDataSource();
    render(
      <AccountSettings
        viewer={{
          ...porticoViewer,
          authOrigin: "local",
          authProvider: "local",
          hasLocalPassword: false,
        }}
        source={source}
      />,
    );

    expect(
      screen.getByText(/does not have a This Server password/),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("Current password")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Delete Portico Account" }),
    ).not.toBeInTheDocument();
  });

  it("accepts either a password or a recently reauthenticated SSO session after destructive confirmation, then signs out", async () => {
    const source = new FixtureSettingsDataSource();
    const remove = vi.spyOn(source, "deletePorticoAccount");
    const signOut = vi.spyOn(source, "signOut");
    render(<AccountSettings viewer={porticoViewer} source={source} />);

    fireEvent.click(
      screen.getByRole("button", { name: "Delete Portico Account" }),
    );
    const confirmButton = screen.getByRole("button", {
      name: "Delete Portico Account",
    });
    expect(confirmButton).toBeDisabled();
    fireEvent.change(
      screen.getByLabelText("Portico Account password (if set)"),
      { target: { value: "current-password" } },
    );
    fireEvent.change(screen.getByLabelText("Type DELETE to confirm"), {
      target: { value: "DELETE" },
    });
    expect(confirmButton).toBeEnabled();
    fireEvent.click(confirmButton);

    await waitFor(() =>
      expect(remove).toHaveBeenCalledWith(
        { password: "current-password" },
        expect.any(AbortSignal),
      ),
    );
    await waitFor(() =>
      expect(signOut).toHaveBeenCalledWith(expect.any(AbortSignal)),
    );
    expect(
      await screen.findByText("Portico Account deleted"),
    ).toBeInTheDocument();
  });

  it("normalizes bad-password and owned-server account deletion failures without raw detail", async () => {
    const source = new FixtureSettingsDataSource();
    vi.spyOn(source, "deletePorticoAccount")
      .mockRejectedValueOnce({
        status: 401,
        code: "invalid_password",
        message: "raw password verifier detail",
      })
      .mockRejectedValueOnce({
        status: 409,
        code: "owned_servers_require_action",
        message: "raw server ids: srv-secret",
      });
    render(<AccountSettings viewer={porticoViewer} source={source} />);

    fireEvent.click(
      screen.getByRole("button", { name: "Delete Portico Account" }),
    );
    const password = screen.getByLabelText("Portico Account password (if set)");
    const confirmation = screen.getByLabelText("Type DELETE to confirm");
    fireEvent.change(password, { target: { value: "wrong-password" } });
    fireEvent.change(confirmation, { target: { value: "DELETE" } });
    fireEvent.click(
      screen.getByRole("button", { name: "Delete Portico Account" }),
    );
    expect(
      await screen.findByText("Password wasn't accepted"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/raw password verifier/i),
    ).not.toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", { name: "Delete Portico Account" }),
    );
    expect(
      await screen.findByText("Owned servers need attention"),
    ).toBeInTheDocument();
    expect(screen.queryByText(/srv-secret/i)).not.toBeInTheDocument();
  });

  it("fences the active bundled viewer and clears remembered browser accounts after deletion", async () => {
    const settingsSource = new FixtureSettingsDataSource();
    const productSource = new FixturePorticoDataSource();
    const signOutAll = vi.spyOn(productSource, "signOutAllBrowserAccounts");
    render(
      <DataProvider source={productSource}>
        <AccountSettings viewer={porticoViewer} source={settingsSource} />
      </DataProvider>,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Delete Portico Account" }),
    );
    fireEvent.change(
      screen.getByLabelText("Portico Account password (if set)"),
      { target: { value: "current-password" } },
    );
    fireEvent.change(screen.getByLabelText("Type DELETE to confirm"), {
      target: { value: "DELETE" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Delete Portico Account" }),
    );

    await waitFor(() =>
      expect(signOutAll).toHaveBeenCalledWith(expect.any(AbortSignal)),
    );
  });

  it("requires confirmation before revoking another server session", async () => {
    const source = new FixtureSettingsDataSource();
    const revoke = vi.spyOn(source, "revokeSignedInDevice");
    render(<AccountSettings viewer={porticoViewer} source={source} />);

    const session = (await screen.findByText("Living Room TV")).closest(
      "article",
    );
    expect(session).not.toBeNull();
    fireEvent.click(
      within(session as HTMLElement).getByRole("button", { name: "Sign out" }),
    );
    expect(
      within(session as HTMLElement).getByText("Sign out this device?"),
    ).toBeInTheDocument();
    fireEvent.click(
      within(session as HTMLElement).getByRole("button", { name: "Sign out" }),
    );
    await waitFor(() =>
      expect(revoke).toHaveBeenCalledWith(
        "portico",
        "portico-device-tv",
        expect.any(AbortSignal),
      ),
    );
    await waitFor(() =>
      expect(screen.queryByText("Living Room TV")).not.toBeInTheDocument(),
    );
  });

  it("opens personal Settings without requesting privileged server settings", async () => {
    const source = new FixtureSettingsDataSource();
    const settings = vi.spyOn(source, "settings");
    const summary = vi.spyOn(source, "settingsSummary");
    const viewer: SettingsViewer = {
      ...porticoViewer,
      role: "user",
      permissions: { manageServer: true },
    };
    render(
      <MemoryRouter initialEntries={["/settings/account"]}>
        <Routes>
          <Route
            path="/settings/:section"
            element={<SettingsPage source={source} viewer={viewer} />}
          />
        </Routes>
      </MemoryRouter>,
    );

    expect(
      await screen.findByRole("heading", { name: "Account" }),
    ).toBeInTheDocument();
    expect(await screen.findByText("Account identity")).toBeInTheDocument();
    expect(
      screen.queryByRole("heading", { name: "This Server" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Server playback" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "My playback" }),
    ).toBeInTheDocument();
    expect(settings).not.toHaveBeenCalled();
    expect(summary).not.toHaveBeenCalled();
  });

  it("shows a truthful not-found state for an invalid Settings deep link", () => {
    const source = new FixtureSettingsDataSource();
    const settings = vi.spyOn(source, "settings");
    render(
      <MemoryRouter initialEntries={["/settings/does-not-exist"]}>
        <Routes>
          <Route
            path="/settings/:section"
            element={<SettingsPage source={source} viewer={porticoViewer} />}
          />
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByText("Settings section not found")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Open account settings" }),
    ).toHaveAttribute("href", "/settings/account");
    expect(settings).not.toHaveBeenCalled();
  });

  it("stops ordinary users before requesting or rendering server administration routes", async () => {
    const source = new FixtureSettingsDataSource();
    const settings = vi.spyOn(source, "settings");
    const summary = vi.spyOn(source, "settingsSummary");
    const operations = vi.spyOn(source, "settingsOperations");
    const viewer: SettingsViewer = {
      ...porticoViewer,
      role: "user",
      permissions: { manageServer: true },
    };
    render(
      <MemoryRouter initialEntries={["/settings/maintenance"]}>
        <Routes>
          <Route
            path="/settings/:section"
            element={<SettingsPage source={source} viewer={viewer} />}
          />
        </Routes>
      </MemoryRouter>,
    );

    expect(
      screen.getByText("Server settings aren’t available"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Open personal settings" }),
    ).toHaveAttribute("href", "/settings/account");
    await waitFor(() => {
      expect(settings).not.toHaveBeenCalled();
      expect(summary).not.toHaveBeenCalled();
      expect(operations).not.toHaveBeenCalled();
    });
    expect(
      screen.queryByRole("heading", { name: "Maintenance" }),
    ).not.toBeInTheDocument();
  });

  it("loads viewer-message recipients without requesting the full administration snapshot", async () => {
    const source = new FixtureSettingsDataSource();
    const productSource = new FixturePorticoDataSource();
    const settings = vi.spyOn(source, "settings");
    const summary = vi.spyOn(source, "settingsSummary");
    const operations = vi.spyOn(source, "settingsOperations");
    const recipients = vi.spyOn(
      productSource,
      "ownerViewerNotificationRecipients",
    );
    render(
      <MemoryRouter initialEntries={["/settings/viewer-messages"]}>
        <Routes>
          <Route
            path="/settings/:section"
            element={
              <SettingsPage
                source={source}
                productSource={productSource}
                viewer={porticoViewer}
              />
            }
          />
        </Routes>
      </MemoryRouter>,
    );

    expect(
      await screen.findByRole("heading", { name: "Viewer messages", level: 1 }),
    ).toBeInTheDocument();
    await waitFor(() => expect(recipients).toHaveBeenCalledTimes(1));
    expect(settings).not.toHaveBeenCalled();
    expect(summary).not.toHaveBeenCalled();
    expect(operations).not.toHaveBeenCalled();
  });

  it("does not expose viewer-message intake to an ordinary user with library permissions", async () => {
    const source = new FixtureSettingsDataSource();
    const productSource = new FixturePorticoDataSource();
    const settings = vi.spyOn(source, "settings");
    const summary = vi.spyOn(source, "settingsSummary");
    const operations = vi.spyOn(source, "settingsOperations");
    const recipients = vi.spyOn(
      productSource,
      "ownerViewerNotificationRecipients",
    );
    const libraryManager: SettingsViewer = {
      ...porticoViewer,
      role: "user",
      permissions: { manageLibraries: true, manageServer: true },
    };

    render(
      <MemoryRouter initialEntries={["/settings/viewer-messages"]}>
        <Routes>
          <Route
            path="/settings/:section"
            element={
              <SettingsPage
                source={source}
                productSource={productSource}
                viewer={libraryManager}
              />
            }
          />
        </Routes>
      </MemoryRouter>,
    );

    expect(
      screen.getByText("Server settings aren’t available"),
    ).toBeInTheDocument();
    await waitFor(() => {
      expect(settings).not.toHaveBeenCalled();
      expect(summary).not.toHaveBeenCalled();
      expect(operations).not.toHaveBeenCalled();
      expect(recipients).not.toHaveBeenCalled();
    });
    expect(
      screen.queryByRole("heading", { name: "Viewer messages" }),
    ).not.toBeInTheDocument();
  });

  it("uses Product Language when the viewer-message source is unavailable", async () => {
    const source = new FixtureSettingsDataSource();
    const settings = vi.spyOn(source, "settings");
    const summary = vi.spyOn(source, "settingsSummary");

    render(
      <MemoryRouter initialEntries={["/settings/viewer-messages"]}>
        <Routes>
          <Route
            path="/settings/:section"
            element={<SettingsPage source={source} viewer={porticoViewer} />}
          />
        </Routes>
      </MemoryRouter>,
    );

    expect(
      await screen.findByText("Viewer messages are unavailable"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Reconnect to this server and try again."),
    ).toBeInTheDocument();
    expect(settings).not.toHaveBeenCalled();
    expect(summary).not.toHaveBeenCalled();
  });

  it("renders an honest Portico Account security failure state", async () => {
    const source = new FixtureSettingsDataSource();
    vi.spyOn(source, "porticoMFAStatus").mockRejectedValue(
      new Error("Portico Account security did not answer."),
    );
    render(<AccountSettings viewer={porticoViewer} source={source} />);
    expect(
      await screen.findByText("Portico Account services unavailable"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "Portico couldn’t reach account services. It will keep trying automatically.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Try again" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText("Portico Account security did not answer."),
    ).not.toBeInTheDocument();
  });

  it("automatically reloads a transient Portico signed-in-device read", async () => {
    vi.useFakeTimers();
    const source = new FixtureSettingsDataSource();
    const devices = vi
      .spyOn(source, "signedInDevices")
      .mockRejectedValueOnce(new TypeError("Failed to fetch"));
    render(<AccountSettings viewer={porticoViewer} source={source} />);
    await act(async () => Promise.resolve());
    expect(
      screen.getByText("Portico Account services unavailable"),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Try again" }),
    ).not.toBeInTheDocument();
    act(() => vi.advanceTimersByTime(5_000));
    await act(async () => Promise.resolve());
    expect(devices).toHaveBeenCalledTimes(2);
    vi.useRealTimers();
    expect(await screen.findByText("Living Room TV")).toBeInTheDocument();
  });
});
