import {
  productMessage,
  type DownloadPreparation,
} from "@porticomediaserver/client-core";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  PrimaryButton,
  SecondaryButton,
} from "../../components/controls/Buttons";
import { ProductLanguageIcon } from "../../components/product/ProductLanguageIcon";
import { reviewedProductErrorText } from "../../components/ProductLanguage";
import { useMediaOperations } from "../../data/DataProvider";
import { formatDetailBytes } from "../detail/detailModel";
import "./downloads.css";

type DownloadAction =
  "pause" | "resume" | "cancel" | "retry" | "remove" | "download";
type DownloadActionFailure = {
  itemId: string;
  action: DownloadAction;
  message: string;
};

const actionNames: Record<Exclude<DownloadAction, "download">, string> = {
  pause: "pause this download",
  resume: "resume this download",
  cancel: "cancel this download",
  retry: "retry this download",
  remove: "remove this download",
};

function activeDownload(item: DownloadPreparation) {
  return item.state === "queued" || item.state === "running";
}

function presentation(item: DownloadPreparation) {
  if (item.state === "ready") return productMessage("download.ready");
  if (item.state === "paused") return productMessage("download.paused");
  if (item.state === "cancelled") return productMessage("download.cancelled");
  if (item.failureMessageId === "download.storage-full")
    return productMessage("download.storage-full");
  if (item.state === "failed" || item.state === "unavailable")
    return productMessage("download.failed");
  return productMessage("download.preparing");
}

export function DownloadsPage() {
  const operations = useMediaOperations();
  const [items, setItems] = useState<DownloadPreparation[]>([]);
  const [status, setStatus] = useState<"loading" | "ready" | "error">(
    "loading",
  );
  const [busy, setBusy] = useState("");
  const [actionFailure, setActionFailure] =
    useState<DownloadActionFailure | null>(null);
  const itemsRef = useRef(items);
  itemsRef.current = items;
  const mounted = useRef(true);
  const loadGeneration = useRef(0);
  const loadInFlight = useRef<Promise<DownloadPreparation[]> | undefined>(
    undefined,
  );
  const pollingTimer = useRef<number | undefined>(undefined);
  const loadRef = useRef<(() => Promise<DownloadPreparation[]>) | undefined>(
    undefined,
  );
  const clearPollingTimer = useCallback(() => {
    if (pollingTimer.current === undefined) return;
    window.clearTimeout(pollingTimer.current);
    pollingTimer.current = undefined;
  }, []);
  const schedulePoll = useCallback(
    (nextItems: DownloadPreparation[]) => {
      clearPollingTimer();
      if (!mounted.current || !nextItems.some(activeDownload)) return;
      pollingTimer.current = window.setTimeout(() => {
        pollingTimer.current = undefined;
        void loadRef.current?.();
      }, 1_500);
    },
    [clearPollingTimer],
  );
  const load = useCallback(() => {
    if (loadInFlight.current) return loadInFlight.current;
    const generation = loadGeneration.current;
    const request = operations.downloadPreparations().then(
      (next) => {
        if (mounted.current && generation === loadGeneration.current) {
          itemsRef.current = next;
          setItems(next);
          setActionFailure((failure) => {
            if (!failure || failure.action === "download") return failure;
            const item = next.find(
              (candidate) => candidate.id === failure.itemId,
            );
            const applied =
              !item ||
              (failure.action === "pause" && item.state === "paused") ||
              (failure.action === "resume" && item.state !== "paused") ||
              (failure.action === "cancel" && item.state === "cancelled") ||
              (failure.action === "retry" &&
                item.state !== "failed" &&
                item.state !== "unavailable");
            return applied ? null : failure;
          });
          setStatus("ready");
          schedulePoll(next);
        }
        return next;
      },
      () => {
        const current = itemsRef.current;
        if (
          mounted.current &&
          generation === loadGeneration.current &&
          current.length === 0
        )
          setStatus("error");
        if (mounted.current && generation === loadGeneration.current)
          schedulePoll(current);
        return current;
      },
    );
    const tracked = request.finally(() => {
      if (loadInFlight.current === tracked) loadInFlight.current = undefined;
    });
    loadInFlight.current = tracked;
    return tracked;
  }, [operations, schedulePoll]);
  loadRef.current = load;
  useEffect(() => {
    mounted.current = true;
    const generation = ++loadGeneration.current;
    void load();
    return () => {
      mounted.current = false;
      if (loadGeneration.current === generation) loadGeneration.current += 1;
      clearPollingTimer();
    };
  }, [clearPollingTimer, load]);
  const measuredBytes = useMemo(
    () => items.reduce((total, item) => total + (item.sizeBytes ?? 0), 0),
    [items],
  );
  const unknownSizes = items.filter((item) => !item.sizeBytes).length;
  const pageDescription = productMessage("download.page-description");
  const storageSummary =
    unknownSizes === 0
      ? productMessage("download.storage-known").text
      : unknownSizes === 1
        ? productMessage("download.storage-measuring-one").text
        : productMessage("download.storage-measuring-many", {
            count: unknownSizes,
          }).text;
  const update = async (
    item: DownloadPreparation,
    action: Exclude<DownloadAction, "download">,
  ) => {
    clearPollingTimer();
    setActionFailure(null);
    setBusy(item.id);
    try {
      await operations.updateDownloadPreparation(item.id, action);
      await load();
    } catch (reason) {
      setActionFailure({
        itemId: item.id,
        action,
        message: reviewedProductErrorText(reason, "settings.action-failed", {
          actionName: actionNames[action],
        }),
      });
      schedulePoll(itemsRef.current);
    } finally {
      setBusy("");
    }
  };
  const download = async (item: DownloadPreparation) => {
    clearPollingTimer();
    setActionFailure(null);
    setBusy(item.id);
    try {
      const anchor = document.createElement("a");
      anchor.href = await operations.downloadPreparationURL(item.id);
      anchor.download = "";
      anchor.rel = "noreferrer";
      anchor.referrerPolicy = "no-referrer";
      anchor.click();
    } catch (reason) {
      setActionFailure({
        itemId: item.id,
        action: "download",
        message: reviewedProductErrorText(reason, "download.failed"),
      });
      schedulePoll(itemsRef.current);
    } finally {
      setBusy("");
    }
  };
  const retryAction = () => {
    if (!actionFailure) return;
    const item = itemsRef.current.find(
      (candidate) => candidate.id === actionFailure.itemId,
    );
    if (!item) {
      setActionFailure(null);
      return;
    }
    if (actionFailure.action === "download") void download(item);
    else void update(item, actionFailure.action);
  };

  return (
    <main className="downloads-page standard-page">
      <header className="page-header downloads-heading">
        <div>
          <h1>{productMessage("download.page-title").text}</h1>
          <p>{pageDescription.body}</p>
        </div>
      </header>
      <section
        className="downloads-storage"
        aria-label={productMessage("download.storage-usage").text}
      >
        <strong>{formatDetailBytes(measuredBytes)}</strong>
        <span>{storageSummary}</span>
      </section>
      {status === "loading" && (
        <div className="downloads-state" aria-busy="true">
          <ProductLanguageIcon id="status.loading" className="state-spinner" />
          <strong>{productMessage("download.queue-loading").title}</strong>
        </div>
      )}
      {status === "error" && (
        <div className="downloads-state error" role="alert">
          <ProductLanguageIcon id="status.offline" />
          <strong>{productMessage("download.queue-failed").title}</strong>
          <p>{productMessage("download.queue-failed").body}</p>
          <SecondaryButton onClick={() => void load()}>
            <ProductLanguageIcon id="action.retry" />{" "}
            {productMessage("action.retry").text}
          </SecondaryButton>
        </div>
      )}
      {status === "ready" && items.length === 0 && (
        <div className="downloads-state">
          <ProductLanguageIcon id="status.download" />
          <strong>{productMessage("download.queue-empty").title}</strong>
          <p>{productMessage("download.queue-empty").body}</p>
        </div>
      )}
      {status === "ready" && items.length > 0 && (
        <div className="downloads-list">
          {items.map((item) => {
            const copy = presentation(item);
            const size = item.sizeBytes
              ? formatDetailBytes(item.sizeBytes)
              : "";
            const sizeLabel =
              item.sizeKind === "estimated"
                ? productMessage("download.size-estimated", { size }).text
                : size;
            const failure =
              actionFailure?.itemId === item.id ? actionFailure : undefined;
            return (
              <article key={item.id}>
                <span className="downloads-mark">
                  <ProductLanguageIcon id={copy.icon || "status.download"} />
                </span>
                <div className="downloads-copy">
                  <strong>{item.mediaTitle}</strong>
                  <span>
                    {item.qualityProfile === "source"
                      ? productMessage("download.original").text
                      : item.qualityProfile}
                  </span>
                  <p>
                    {copy.title}
                    {item.state === "queued" || item.state === "running"
                      ? ` · ${productMessage("download.progress-percent", { progress: item.progress }).text}`
                      : ""}
                    {item.sizeBytes ? ` · ${sizeLabel}` : ""}
                  </p>
                  {(item.state === "failed" ||
                    item.state === "unavailable") && <small>{copy.body}</small>}
                </div>
                <div className="downloads-actions">
                  {item.state === "ready" && (
                    <PrimaryButton
                      disabled={busy === item.id}
                      onClick={() => void download(item)}
                    >
                      <ProductLanguageIcon id="action.download" />{" "}
                      {productMessage("action.download").text}
                    </PrimaryButton>
                  )}
                  {item.canPause && (
                    <SecondaryButton
                      disabled={busy === item.id}
                      onClick={() => void update(item, "pause")}
                    >
                      <ProductLanguageIcon id="action.pause" />{" "}
                      {productMessage("action.pause").text}
                    </SecondaryButton>
                  )}
                  {item.state === "paused" && (
                    <SecondaryButton
                      disabled={busy === item.id}
                      onClick={() => void update(item, "resume")}
                    >
                      <ProductLanguageIcon id="action.resume" />{" "}
                      {productMessage("action.resume").text}
                    </SecondaryButton>
                  )}
                  {item.canCancel && (
                    <SecondaryButton
                      disabled={busy === item.id}
                      onClick={() => void update(item, "cancel")}
                    >
                      <ProductLanguageIcon id="action.cancel" />{" "}
                      {productMessage("action.cancel").text}
                    </SecondaryButton>
                  )}
                  {item.canRetry && (
                    <SecondaryButton
                      disabled={busy === item.id}
                      onClick={() => void update(item, "retry")}
                    >
                      <ProductLanguageIcon id="action.retry" />{" "}
                      {productMessage("action.retry").text}
                    </SecondaryButton>
                  )}
                  {item.canRemove && (
                    <SecondaryButton
                      disabled={busy === item.id}
                      onClick={() => void update(item, "remove")}
                    >
                      <ProductLanguageIcon id="action.remove-download" />{" "}
                      {productMessage("action.remove-download").text}
                    </SecondaryButton>
                  )}
                </div>
                {failure && (
                  <div className="downloads-action-error" role="alert">
                    <span>{failure.message}</span>
                    <SecondaryButton
                      disabled={busy === item.id}
                      onClick={retryAction}
                    >
                      <ProductLanguageIcon id="action.retry" />{" "}
                      {productMessage("action.retry").text}
                    </SecondaryButton>
                  </div>
                )}
              </article>
            );
          })}
        </div>
      )}
    </main>
  );
}
