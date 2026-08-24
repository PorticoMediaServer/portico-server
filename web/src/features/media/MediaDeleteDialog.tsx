import {
  AlertTriangle,
  Database,
  FileWarning,
  RefreshCw,
  Trash2,
  X,
} from "#portico-icons";
import { ApiError } from "@porticomediaserver/client-core";
import { useMemo, useState } from "react";
import { IconButton, SecondaryButton } from "../../components/controls/Buttons";
import { ModalOverlay } from "../../components/overlay/OverlayPortal";
import type { MediaDeleteResult, MediaItem } from "../../data/models";
import "./media-actions.css";

type DeletableMediaItem = Pick<MediaItem, "id" | "title" | "fileCount">;

export type MediaDeleteCompletion = {
  deletedIds: string[];
  failedIds: string[];
  deletedItems: number;
  trashedFiles: number;
};

export function MediaDeleteDialog({
  items,
  onDismiss,
  onDelete,
  onComplete,
}: {
  items: DeletableMediaItem[];
  onDismiss: () => void;
  onDelete: (
    id: string,
    input: { deleteFiles: boolean; confirmation?: string },
  ) => Promise<MediaDeleteResult>;
  onComplete: (result: MediaDeleteCompletion) => void;
}) {
  const [intent, setIntent] = useState<"record" | "files">("record");
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const single = items.length === 1 ? items[0] : undefined;
  const canTrashFiles = Boolean(single && (single.fileCount ?? 0) > 0);
  const requiredConfirmation = intent === "files" ? (single?.title ?? "") : "";
  const validConfirmation =
    !requiredConfirmation || confirmation.trim() === requiredConfirmation;
  const title =
    items.length === 1
      ? (items[0]?.title ?? "this item")
      : `${items.length} selected items`;
  const shownItems = useMemo(() => items.slice(0, 5), [items]);
  const headingId = "media-delete-dialog-title";

  const submit = async () => {
    if (!items.length || busy) return;
    if (!validConfirmation) {
      setError(
        `Type “${requiredConfirmation}” exactly to move its source files to trash.`,
      );
      return;
    }
    setBusy(true);
    setError("");
    const deleteFiles = intent === "files";
    const results = await Promise.allSettled(
      items.map((item) =>
        onDelete(item.id, {
          deleteFiles,
          confirmation: deleteFiles ? item.title : undefined,
        }),
      ),
    );
    const deletedIds: string[] = [];
    const failedIds: string[] = [];
    let deletedItems = 0;
    let trashedFiles = 0;
    results.forEach((result, index) => {
      const item = items[index];
      if (!item) return;
      if (
        result.status === "fulfilled" ||
        (result.reason instanceof ApiError && result.reason.status === 404)
      ) {
        deletedIds.push(item.id);
        if (result.status === "fulfilled") {
          deletedItems += result.value.deletedItems;
          trashedFiles += result.value.trashedFiles;
        }
      } else {
        failedIds.push(item.id);
      }
    });
    const completion = { deletedIds, failedIds, deletedItems, trashedFiles };
    onComplete(completion);
    setBusy(false);
    if (failedIds.length) {
      setError(
        deletedIds.length
          ? `${deletedIds.length} removed; ${failedIds.length} could not be removed and remain selected.`
          : `Portico could not remove ${failedIds.length === 1 ? "this item" : "these items"}. Nothing changed.`,
      );
      return;
    }
    onDismiss();
  };

  return (
    <ModalOverlay
      labelledBy={headingId}
      className="media-delete-dialog"
      onDismiss={() => {
        if (!busy) onDismiss();
      }}
    >
      <header>
        <div>
          <p>Remove media</p>
          <h2 id={headingId}>
            {items.length === 1 ? title : `Remove ${title}`}
          </h2>
        </div>
        <IconButton
          label="Close remove media dialog"
          disabled={busy}
          onClick={onDismiss}
        >
          <X />
        </IconButton>
      </header>
      <div className="media-delete-body">
        <div className="media-delete-impact" role="note">
          <AlertTriangle />
          <span>
            <strong>This cannot be undone in Portico.</strong>
            <small>
              Choose whether Portico should only forget the library record or
              also move source files to the server trash.
            </small>
          </span>
        </div>
        {items.length > 1 && (
          <div className="media-delete-selection">
            <strong>{items.length} selected items</strong>
            {shownItems.map((item) => (
              <span key={item.id}>{item.title}</span>
            ))}
            {items.length > shownItems.length && (
              <small>and {items.length - shownItems.length} more</small>
            )}
          </div>
        )}
        <div
          className="media-delete-options"
          role="radiogroup"
          aria-label="Removal method"
        >
          <button
            type="button"
            role="radio"
            aria-checked={intent === "record"}
            className={intent === "record" ? "selected" : ""}
            disabled={busy}
            onClick={() => {
              setIntent("record");
              setConfirmation("");
              setError("");
            }}
          >
            <Database />
            <span>
              <strong>Remove from Portico</strong>
              <small>
                Delete the library record and personal state. Source files stay
                where they are.
              </small>
            </span>
          </button>
          {canTrashFiles && (
            <button
              type="button"
              role="radio"
              aria-checked={intent === "files"}
              className={intent === "files" ? "selected danger" : "danger"}
              disabled={busy}
              onClick={() => {
                setIntent("files");
                setError("");
              }}
            >
              <FileWarning />
              <span>
                <strong>Move source files to trash</strong>
                <small>
                  Remove the record and move every source file for this item to
                  the server trash.
                </small>
              </span>
            </button>
          )}
        </div>
        {intent === "files" && single && (
          <label className="media-delete-confirmation">
            <span>
              Type <strong>{single.title}</strong> to continue
            </span>
            <input
              autoFocus
              value={confirmation}
              disabled={busy}
              onChange={(event) => {
                setConfirmation(event.target.value);
                setError("");
              }}
              autoComplete="off"
            />
          </label>
        )}
        {error && (
          <p className="media-delete-error" role="alert">
            <AlertTriangle /> {error}
          </p>
        )}
      </div>
      <footer>
        <SecondaryButton disabled={busy} onClick={onDismiss}>
          Cancel
        </SecondaryButton>
        <button
          type="button"
          className="button danger"
          disabled={busy || !validConfirmation}
          onClick={() => void submit()}
        >
          {busy ? (
            <>
              <RefreshCw className="state-spinner" /> Removing…
            </>
          ) : (
            <>
              <Trash2 />{" "}
              {intent === "files"
                ? "Move to trash"
                : items.length === 1
                  ? "Remove item"
                  : `Remove ${items.length} items`}
            </>
          )}
        </button>
      </footer>
    </ModalOverlay>
  );
}
