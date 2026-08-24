import { useCallback, useEffect, useRef, useState } from "react";
import {
  productMessage,
  unrestrictedProfilePolicy,
  type ProductMessageId,
  type ProductMessagePresentation,
  type ServerManagedProfileDirectory,
} from "@porticomediaserver/client-core";
import {
  PrimaryButton,
  SecondaryButton,
} from "../../components/controls/Buttons";
import { PasswordInput } from "../../components/controls/PasswordInput";
import {
  ProductMessageIcon,
  SemanticProductIcon,
  productProblem,
  productText,
} from "../../components/ProductLanguage";
import { useAuthSession } from "../../data/DataProvider";
import type { PorticoDataSource } from "../../data/models";
import { useWebDisplayPreferences } from "../../preferences/WebDisplayPreferencesProvider";
import {
  ChoiceControl,
  InlineNotice,
  SettingRow,
  SettingsGroup,
  SettingsLoading,
  TextControl,
} from "./SettingsControls";
import { combineAbortSignals, timeoutSignal } from "../../runtime/abortSignal";

type Proof = { token: string; expiresAt: string };
const SETTINGS_REQUEST_DEADLINE_MS = 15_000;

function boundedSignal(controller: AbortController) {
  return combineAbortSignals([
    controller.signal,
    timeoutSignal(SETTINGS_REQUEST_DEADLINE_MS),
  ]);
}

export function ProfilesSettings({ source }: { source: PorticoDataSource }) {
  const auth = useAuthSession();
  const display = useWebDisplayPreferences();
  const [directory, setDirectory] = useState<ServerManagedProfileDirectory>();
  const [status, setStatus] = useState<"loading" | "ready" | "error">(
    "loading",
  );
  const [error, setError] = useState<ProductMessagePresentation>();
  const [notice, setNotice] = useState<ProductMessagePresentation>();
  const [proofInput, setProofInput] = useState("");
  const [proof, setProof] = useState<Proof>();
  const [newName, setNewName] = useState("");
  const [newPin, setNewPin] = useState("");
  const [pinEditor, setPinEditor] = useState<{
    profileId: string;
    pin: string;
    password: string;
    secondFactor: string;
    clear?: boolean;
  }>();
  const [recovery, setRecovery] = useState<{
    replacementPin: string;
    password: string;
    verification: string;
    emailToken: string;
    mfaEnabled: boolean;
    emailSent: boolean;
  }>();
  const [deleteProfileId, setDeleteProfileId] = useState<string>();
  const [busy, setBusy] = useState(false);
  const proofRef = useRef<Proof | undefined>(undefined);
  const loadController = useRef<AbortController | undefined>(undefined);
  const mutationController = useRef<AbortController | undefined>(undefined);

  const load = useCallback(async () => {
    loadController.current?.abort();
    const controller = new AbortController();
    loadController.current = controller;
    setStatus("loading");
    try {
      const value = await source.accountProfiles(boundedSignal(controller));
      if (controller.signal.aborted) return;
      setDirectory(value);
      setStatus("ready");
      setError(undefined);
    } catch (reason) {
      if (controller.signal.aborted) return;
      setStatus("error");
      setError(productProblem(reason));
    } finally {
      if (loadController.current === controller)
        loadController.current = undefined;
    }
  }, [source]);

  useEffect(() => {
    void load();
    return () => {
      loadController.current?.abort();
      mutationController.current?.abort();
    };
  }, [load]);

  const run = async (
    operation: (signal: AbortSignal) => Promise<unknown>,
    success: ProductMessageId,
  ) => {
    mutationController.current?.abort();
    const controller = new AbortController();
    mutationController.current = controller;
    setBusy(true);
    setError(undefined);
    setNotice(undefined);
    try {
      await operation(boundedSignal(controller));
      if (controller.signal.aborted) return false;
      setNotice(productMessage(success));
      await load();
      return true;
    } catch (reason) {
      if (!controller.signal.aborted) setError(productProblem(reason));
      return false;
    } finally {
      if (mutationController.current === controller) {
        mutationController.current = undefined;
        setBusy(false);
      }
    }
  };

  const expireProof = useCallback((announce: boolean) => {
    proofRef.current = undefined;
    setProof(undefined);
    setPinEditor(undefined);
    setDeleteProfileId(undefined);
    setRecovery(undefined);
    if (announce) setNotice(productMessage("profiles.management-expired"));
  }, []);

  const acceptProof = useCallback((nextProof: Proof) => {
    proofRef.current = nextProof;
    setProof(nextProof);
  }, []);

  const requireProofToken = useCallback(() => {
    const current = proofRef.current;
    if (
      !current ||
      !Number.isFinite(Date.parse(current.expiresAt)) ||
      Date.parse(current.expiresAt) <= Date.now()
    ) {
      expireProof(Boolean(current));
      return undefined;
    }
    return current.token;
  }, [expireProof]);

  const runProtected = async (
    operation: (token: string, signal: AbortSignal) => Promise<unknown>,
    success: ProductMessageId,
  ) => {
    const token = requireProofToken();
    if (!token) return false;
    return run((signal) => operation(token, signal), success);
  };

  const prove = () =>
    run(async (signal) => {
      const response = await source.createProfileAdministrationProof(
        /^\d{4}$/.test(proofInput)
          ? { pin: proofInput }
          : { password: proofInput },
        signal,
      );
      acceptProof(response);
      setProofInput("");
    }, "profiles.management-unlocked");

  const openRecovery = async () => {
    mutationController.current?.abort();
    const controller = new AbortController();
    mutationController.current = controller;
    setBusy(true);
    setError(undefined);
    setNotice(undefined);
    try {
      const status = await source.porticoProfileMFAStatus(
        boundedSignal(controller),
      );
      if (controller.signal.aborted) return;
      setRecovery({
        replacementPin: "",
        password: "",
        verification: "",
        emailToken: "",
        mfaEnabled: status.enabled,
        emailSent: false,
      });
    } catch (reason) {
      if (!controller.signal.aborted) setError(productProblem(reason));
    } finally {
      if (mutationController.current === controller) {
        mutationController.current = undefined;
        setBusy(false);
      }
    }
  };

  const requestRecoveryEmail = async () => {
    if (!recovery) return;
    mutationController.current?.abort();
    const controller = new AbortController();
    mutationController.current = controller;
    setBusy(true);
    setError(undefined);
    setNotice(undefined);
    try {
      await source.requestPorticoProfileRecoveryEmail(
        boundedSignal(controller),
      );
      if (controller.signal.aborted) return;
      setRecovery({ ...recovery, emailSent: true });
    } catch (reason) {
      if (!controller.signal.aborted) setError(productProblem(reason));
    } finally {
      if (mutationController.current === controller) {
        mutationController.current = undefined;
        setBusy(false);
      }
    }
  };

  const recoverPrimaryPIN = async () => {
    if (!recovery) return;
    await run(async (signal) => {
      const verification = recovery.verification.trim();
      const response = await source.createProfileAdministrationProof(
        {
          replacementPin: recovery.replacementPin,
          password: recovery.password,
          ...(recovery.mfaEnabled
            ? /^\d{6}$/.test(verification)
              ? { mfaCode: verification }
              : { recoveryCode: verification }
            : { emailRecoveryToken: recovery.emailToken.trim() }),
        },
        signal,
      );
      acceptProof(response);
      setRecovery(undefined);
    }, "profiles.management-unlocked");
  };

  const pinReauthentication = (password: string, secondFactor: string) => {
    const verification = secondFactor.trim();
    return {
      password,
      ...(verification
        ? /^\d{6}$/.test(verification)
          ? { mfaCode: verification }
          : { recoveryCode: verification }
        : {}),
    };
  };

  useEffect(() => {
    proofRef.current = proof;
    if (!proof) return;
    const expiresAt = Date.parse(proof.expiresAt);
    if (!Number.isFinite(expiresAt) || expiresAt <= Date.now()) {
      expireProof(true);
      return;
    }
    const timer = window.setTimeout(
      () => expireProof(true),
      Math.max(0, expiresAt - Date.now()),
    );
    const recheck = () => {
      if (document.visibilityState === "visible") requireProofToken();
    };
    window.addEventListener("focus", recheck);
    document.addEventListener("visibilitychange", recheck);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener("focus", recheck);
      document.removeEventListener("visibilitychange", recheck);
    };
  }, [expireProof, proof, requireProofToken]);

  const validProof =
    proof &&
    Number.isFinite(Date.parse(proof.expiresAt)) &&
    Date.parse(proof.expiresAt) > Date.now();
  const profileSelection =
    display.bundle?.accountServerInstallation.values.profileSelection ??
    "last-used";

  if (status === "loading" && !directory)
    return <SettingsLoading label={productText("profiles.loading")} />;
  return (
    <div className="portico-settings-content profiles-settings">
      {(error || notice) && (
        <InlineNotice tone={error ? "error" : "success"}>
          <strong>{(error ?? notice)?.title}</strong>
          {(error ?? notice)?.body && <> {(error ?? notice)?.body}</>}
        </InlineNotice>
      )}
      {!directory ? (
        <div className="portico-settings-state error">
          <ProductMessageIcon
            presentation={
              error ?? productMessage("problem.profile-request-failed")
            }
          />
          <strong>
            {error?.title ??
              productMessage("problem.profile-request-failed").title}
          </strong>
          <p>
            {error?.body ??
              productMessage("problem.profile-request-failed").body}
          </p>
          <SecondaryButton onClick={() => void load()}>
            <SemanticProductIcon id="action.retry" />{" "}
            {productText("action.retry")}
          </SecondaryButton>
        </div>
      ) : (
        <>
          <SettingsGroup
            title={productText("profiles.choose-title")}
            description={productText("profiles.choose-description")}
          >
            <div className="profile-directory-list">
              {directory.profiles.map((profile) => (
                <div
                  key={profile.id}
                  className={
                    profile.id === auth.viewer?.viewerScope?.profileId
                      ? "active"
                      : ""
                  }
                >
                  <span className="profile-directory-avatar">
                    {profile.name.trim().slice(0, 1).toUpperCase()}
                  </span>
                  <span>
                    <strong>{profile.name}</strong>
                    <small>
                      {productText(
                        profile.isPrimary
                          ? "profiles.status.primary"
                          : profile.hasPIN
                            ? "profiles.status.pin-protected"
                            : "profiles.status.no-pin",
                      )}
                      {profile.id === auth.viewer?.viewerScope?.profileId
                        ? ` · ${productText("profiles.status.current")}`
                        : ""}
                    </small>
                  </span>
                  {profile.id === auth.viewer?.viewerScope?.profileId && (
                    <SemanticProductIcon id="status.success" />
                  )}
                </div>
              ))}
            </div>
          </SettingsGroup>
          <SettingsGroup
            title={productText("profiles.selection-title")}
            description={productText("profiles.selection-description")}
          >
            <SettingRow
              label={productText("profiles.selection-when-open")}
              description={productText(
                "profiles.selection-behavior-description",
              )}
            >
              <ChoiceControl
                label={productText("profiles.selection-title")}
                value={profileSelection}
                options={[
                  {
                    value: "last-used",
                    label: productText("profiles.selection-last-used"),
                  },
                  {
                    value: "ask",
                    label: productText("profiles.selection-ask"),
                  },
                ]}
                onChange={(value) => {
                  setError(undefined);
                  void display
                    .patchScope("account-server-installation", {
                      profileSelection: value,
                    })
                    .catch((reason) => setError(productProblem(reason)));
                }}
              />
            </SettingRow>
            <SettingRow
              label={productText("profiles.remember-title")}
              description={productText("profiles.remember-description")}
            >
              <div className="profile-trust-actions">
                <SecondaryButton
                  disabled={busy}
                  onClick={() =>
                    void run(
                      (signal) => source.createAutomaticProfileTrust(signal),
                      "profiles.remembered",
                    )
                  }
                >
                  <SemanticProductIcon id="status.success" />{" "}
                  {productText("action.remember-profile")}
                </SecondaryButton>
                <SecondaryButton
                  disabled={busy}
                  onClick={() =>
                    void run(
                      (signal) => source.revokeAutomaticProfileTrust(signal),
                      "profiles.trust-revoked",
                    )
                  }
                >
                  <SemanticProductIcon id="action.delete" />{" "}
                  {productText("action.revoke-profile-trust")}
                </SecondaryButton>
              </div>
            </SettingRow>
          </SettingsGroup>
          {directory.canManage && (
            <SettingsGroup
              title={productText("profiles.manage-title")}
              description={productText("profiles.manage-description")}
            >
              {!validProof ? (
                <>
                  <div className="profile-proof-form">
                    <SemanticProductIcon id="action.pin" />
                    <span>
                      <strong>
                        {productText("profiles.confirm-primary-title")}
                      </strong>
                      <small>
                        {productText(
                          directory.authority === "hosted"
                            ? "profiles.confirm-primary-hosted"
                            : "profiles.confirm-primary-local",
                        )}
                      </small>
                    </span>
                    <PasswordInput
                      aria-label={productText("profiles.confirm-primary-title")}
                      value={proofInput}
                      onChange={(event) => setProofInput(event.target.value)}
                    />
                    <SecondaryButton
                      disabled={!proofInput || busy}
                      onClick={prove}
                    >
                      <SemanticProductIcon id="status.locked" />{" "}
                      {productText("action.unlock")}
                    </SecondaryButton>
                    {directory.authority === "hosted" && (
                      <button
                        type="button"
                        className="button secondary"
                        disabled={busy}
                        onClick={() => void openRecovery()}
                      >
                        Forgot primary PIN?
                      </button>
                    )}
                  </div>
                  {directory.authority === "hosted" && recovery && (
                    <form
                      className="profile-inline-confirmation"
                      onSubmit={(event) => {
                        event.preventDefault();
                        void recoverPrimaryPIN();
                      }}
                    >
                      <p>
                        <strong>Recover the primary profile PIN</strong>
                        <span>
                          {recovery.mfaEnabled
                            ? "Confirm a fresh Google or Apple sign-in, or your Portico Account password, plus an authenticator or recovery code."
                            : "Confirm a fresh Google or Apple sign-in, or your Portico Account password, plus the recovery token sent to your email."}
                        </span>
                      </p>
                      <label>
                        New four-digit PIN
                        <PasswordInput
                          required
                          aria-label="New primary profile PIN"
                          autoComplete="one-time-code"
                          inputMode="numeric"
                          pattern="[0-9]{4}"
                          maxLength={4}
                          value={recovery.replacementPin}
                          onChange={(event) =>
                            setRecovery({
                              ...recovery,
                              replacementPin: event.target.value
                                .replace(/\D/g, "")
                                .slice(0, 4),
                            })
                          }
                        />
                      </label>
                      <label>
                        Portico Account password (if set)
                        <PasswordInput
                          aria-label="Portico Account password for PIN recovery (if set)"
                          autoComplete="current-password"
                          value={recovery.password}
                          onChange={(event) =>
                            setRecovery({
                              ...recovery,
                              password: event.target.value,
                            })
                          }
                        />
                      </label>
                      {recovery.mfaEnabled ? (
                        <label>
                          Authenticator or recovery code
                          <input
                            required
                            aria-label="Authenticator or recovery code for PIN recovery"
                            autoComplete="one-time-code"
                            value={recovery.verification}
                            onChange={(event) =>
                              setRecovery({
                                ...recovery,
                                verification: event.target.value.slice(0, 64),
                              })
                            }
                          />
                        </label>
                      ) : (
                        <>
                          <label>
                            Email recovery token
                            <input
                              required
                              aria-label="Email recovery token for PIN recovery"
                              autoComplete="one-time-code"
                              value={recovery.emailToken}
                              onChange={(event) =>
                                setRecovery({
                                  ...recovery,
                                  emailToken: event.target.value.slice(0, 256),
                                })
                              }
                            />
                          </label>
                          <SecondaryButton
                            disabled={busy || recovery.emailSent}
                            onClick={() => void requestRecoveryEmail()}
                          >
                            {recovery.emailSent
                              ? "Recovery email sent"
                              : "Send recovery email"}
                          </SecondaryButton>
                        </>
                      )}
                      <PrimaryButton
                        type="submit"
                        disabled={
                          busy ||
                          recovery.replacementPin.length !== 4 ||
                          (recovery.mfaEnabled
                            ? !recovery.verification.trim()
                            : !recovery.emailToken.trim())
                        }
                      >
                        Recover and unlock
                      </PrimaryButton>
                      <SecondaryButton
                        disabled={busy}
                        onClick={() => setRecovery(undefined)}
                      >
                        {productText("action.cancel")}
                      </SecondaryButton>
                    </form>
                  )}
                </>
              ) : (
                <>
                  <form
                    className="profile-create-form"
                    onSubmit={(event) => {
                      event.preventDefault();
                      void runProtected(
                        (token, signal) =>
                          source.createAccountProfile(
                            {
                              name: newName,
                              pin:
                                directory.authority === "local"
                                  ? newPin || undefined
                                  : undefined,
                              policy: structuredClone(
                                unrestrictedProfilePolicy,
                              ),
                            },
                            token,
                            signal,
                          ),
                        "profiles.created",
                      ).then((created) => {
                        if (created) {
                          setNewName("");
                          setNewPin("");
                        }
                      });
                    }}
                  >
                    <TextControl
                      label={productText("profiles.label.new-name")}
                      value={newName}
                      placeholder={productText("profiles.label.profile")}
                      onChange={setNewName}
                    />
                    {directory.authority === "local" && (
                      <PasswordInput
                        aria-label={productText("profiles.label.optional-pin")}
                        autoComplete="one-time-code"
                        inputMode="numeric"
                        pattern="[0-9]{4}"
                        maxLength={4}
                        placeholder={productText("profiles.label.optional-pin")}
                        value={newPin}
                        onChange={(event) =>
                          setNewPin(
                            event.target.value.replace(/\D/g, "").slice(0, 4),
                          )
                        }
                      />
                    )}
                    <PrimaryButton
                      type="submit"
                      disabled={
                        !newName.trim() ||
                        Boolean(
                          directory.authority === "local" &&
                          newPin &&
                          newPin.length !== 4,
                        ) ||
                        busy
                      }
                    >
                      <SemanticProductIcon id="status.profile" />{" "}
                      {productText("action.add-profile")}
                    </PrimaryButton>
                  </form>
                  <div className="profile-manage-list">
                    {directory.profiles.map((profile, index) => (
                      <div key={profile.id}>
                        <span className="profile-directory-avatar">
                          {profile.name.trim().slice(0, 1).toUpperCase()}
                        </span>
                        <span>
                          <strong>{profile.name}</strong>
                          <small>
                            {productText(
                              profile.isPrimary
                                ? "profiles.status.primary"
                                : profile.hasPIN
                                  ? "profiles.status.pin-protected"
                                  : "profiles.status.no-pin",
                            )}
                          </small>
                        </span>
                        <div className="profile-manage-actions">
                          <button
                            type="button"
                            aria-label={productText("action.move-profile-up", {
                              profileName: profile.name,
                            })}
                            disabled={index === 0 || busy}
                            onClick={() => {
                              const ids = directory.profiles.map(
                                (item) => item.id,
                              );
                              [ids[index - 1], ids[index]] = [
                                ids[index],
                                ids[index - 1],
                              ];
                              void runProtected(
                                (token, signal) =>
                                  source.reorderAccountProfiles(
                                    ids,
                                    token,
                                    signal,
                                  ),
                                "profiles.order-saved",
                              );
                            }}
                          >
                            <SemanticProductIcon id="action.move-up" />
                          </button>
                          <button
                            type="button"
                            aria-label={productText(
                              "action.move-profile-down",
                              { profileName: profile.name },
                            )}
                            disabled={
                              index === directory.profiles.length - 1 || busy
                            }
                            onClick={() => {
                              const ids = directory.profiles.map(
                                (item) => item.id,
                              );
                              [ids[index + 1], ids[index]] = [
                                ids[index],
                                ids[index + 1],
                              ];
                              void runProtected(
                                (token, signal) =>
                                  source.reorderAccountProfiles(
                                    ids,
                                    token,
                                    signal,
                                  ),
                                "profiles.order-saved",
                              );
                            }}
                          >
                            <SemanticProductIcon id="action.move-down" />
                          </button>
                          {profile.hasPIN ? (
                            <>
                              <button
                                type="button"
                                disabled={busy}
                                onClick={() => {
                                  setDeleteProfileId(undefined);
                                  setPinEditor({
                                    profileId: profile.id,
                                    pin: "",
                                    password: "",
                                    secondFactor: "",
                                  });
                                }}
                              >
                                <SemanticProductIcon id="action.pin" /> Change
                                PIN
                              </button>
                              <button
                                type="button"
                                disabled={busy}
                                onClick={() => {
                                  setDeleteProfileId(undefined);
                                  setPinEditor({
                                    profileId: profile.id,
                                    pin: "",
                                    password: "",
                                    secondFactor: "",
                                    clear: true,
                                  });
                                }}
                              >
                                <SemanticProductIcon id="action.pin" />{" "}
                                {productText("action.remove-pin")}
                              </button>
                            </>
                          ) : (
                            <button
                              type="button"
                              disabled={busy}
                              onClick={() => {
                                setDeleteProfileId(undefined);
                                setPinEditor({
                                  profileId: profile.id,
                                  pin: "",
                                  password: "",
                                  secondFactor: "",
                                });
                              }}
                            >
                              <SemanticProductIcon id="action.pin" />{" "}
                              {productText("action.add-pin")}
                            </button>
                          )}
                          {!profile.isPrimary && (
                            <button
                              className="danger"
                              type="button"
                              disabled={busy}
                              onClick={() => {
                                setPinEditor(undefined);
                                setDeleteProfileId(profile.id);
                              }}
                            >
                              <SemanticProductIcon id="action.delete" />{" "}
                              {productText("action.remove-profile")}
                            </button>
                          )}
                        </div>
                        {pinEditor?.profileId === profile.id && (
                          <form
                            className="profile-inline-confirmation"
                            onSubmit={(event) => {
                              event.preventDefault();
                              const reauthentication = pinReauthentication(
                                pinEditor.password,
                                pinEditor.secondFactor,
                              );
                              const operation = pinEditor.clear
                                ? (token: string, signal: AbortSignal) =>
                                    source.clearAccountProfilePin(
                                      profile.id,
                                      reauthentication,
                                      token,
                                      signal,
                                    )
                                : (token: string, signal: AbortSignal) =>
                                    source.setAccountProfilePin(
                                      profile.id,
                                      {
                                        pin: pinEditor.pin,
                                        ...reauthentication,
                                      },
                                      token,
                                      signal,
                                    );
                              void runProtected(
                                operation,
                                pinEditor.clear
                                  ? "profiles.pin-removed"
                                  : "profiles.pin-saved",
                              ).then((saved) => {
                                if (saved) setPinEditor(undefined);
                              });
                            }}
                          >
                            {!pinEditor.clear && (
                              <label>
                                {productText("profiles.label.profile-pin")}
                                <PasswordInput
                                  autoFocus
                                  required
                                  aria-label={`${productText("profiles.label.profile-pin")} — ${profile.name}`}
                                  autoComplete="one-time-code"
                                  inputMode="numeric"
                                  pattern="[0-9]{4}"
                                  maxLength={4}
                                  value={pinEditor.pin}
                                  onChange={(event) =>
                                    setPinEditor({
                                      ...pinEditor,
                                      pin: event.target.value
                                        .replace(/\D/g, "")
                                        .slice(0, 4),
                                    })
                                  }
                                />
                              </label>
                            )}
                            <label>
                              {productText(
                                directory.authority === "hosted"
                                  ? "profiles.label.portico-account-password"
                                  : "profiles.label.account-password",
                              )}
                              {directory.authority === "hosted" && " (if set)"}
                              <PasswordInput
                                autoFocus={Boolean(pinEditor.clear)}
                                required={directory.authority === "local"}
                                aria-label={`${productText(directory.authority === "hosted" ? "profiles.label.portico-account-password" : "profiles.label.account-password")}${directory.authority === "hosted" ? " (if set)" : ""}`}
                                autoComplete="current-password"
                                value={pinEditor.password}
                                onChange={(event) =>
                                  setPinEditor({
                                    ...pinEditor,
                                    password: event.target.value,
                                  })
                                }
                              />
                            </label>
                            {directory.authority === "hosted" && (
                              <label>
                                Authenticator or recovery code
                                <input
                                  aria-label={`Authenticator or recovery code — ${profile.name}`}
                                  autoComplete="one-time-code"
                                  value={pinEditor.secondFactor}
                                  onChange={(event) =>
                                    setPinEditor({
                                      ...pinEditor,
                                      secondFactor: event.target.value.slice(
                                        0,
                                        64,
                                      ),
                                    })
                                  }
                                />
                                <small>
                                  Required when two-factor authentication is
                                  enabled.
                                </small>
                              </label>
                            )}
                            <PrimaryButton
                              type="submit"
                              disabled={
                                (!pinEditor.clear &&
                                  pinEditor.pin.length !== 4) ||
                                (directory.authority === "local" &&
                                  !pinEditor.password) ||
                                busy
                              }
                            >
                              <SemanticProductIcon id="action.pin" />{" "}
                              {productText(
                                pinEditor.clear
                                  ? "action.remove-pin"
                                  : "action.save-pin",
                              )}
                            </PrimaryButton>
                            <SecondaryButton
                              onClick={() => setPinEditor(undefined)}
                            >
                              {productText("action.cancel")}
                            </SecondaryButton>
                          </form>
                        )}
                        {deleteProfileId === profile.id && (
                          <div
                            className="profile-inline-confirmation"
                            role="alert"
                          >
                            <p>
                              <strong>
                                {productText("action.remove-profile")} —{" "}
                                {profile.name}?
                              </strong>
                              <span>
                                {productText(
                                  directory.authority === "hosted"
                                    ? "profiles.remove-confirmation-hosted"
                                    : "profiles.remove-confirmation-local",
                                  { profileName: profile.name },
                                )}
                              </span>
                            </p>
                            <button
                              className="button secondary danger"
                              type="button"
                              disabled={busy}
                              onClick={() =>
                                void runProtected(
                                  (token, signal) =>
                                    source.deleteAccountProfile(
                                      profile.id,
                                      token,
                                      signal,
                                    ),
                                  "profiles.removed",
                                ).then((removed) => {
                                  if (removed) setDeleteProfileId(undefined);
                                })
                              }
                            >
                              <SemanticProductIcon id="action.delete" />{" "}
                              {productText("action.remove-profile")}
                            </button>
                            <SecondaryButton
                              disabled={busy}
                              onClick={() => setDeleteProfileId(undefined)}
                            >
                              {productText("action.cancel")}
                            </SecondaryButton>
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                </>
              )}
            </SettingsGroup>
          )}
        </>
      )}
    </div>
  );
}
