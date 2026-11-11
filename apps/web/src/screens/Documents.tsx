import { useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { api, type Document, type HouseholdDetail } from "../api/client";
import {
  fileUrl,
  kindOf,
  maxDocumentMB,
  sizeLabel,
  typeOf,
  uploadDocument,
  useDocuments,
} from "../documents";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { Empty } from "../ui/Empty";
import { FormGroup, SwitchRow } from "../ui/Form";
import { Icon } from "../ui/Icon";
import { ListRow } from "../ui/ListRow";
import { Sheet } from "../ui/Sheet";
import { Swipe } from "../ui/Swipe";
import { Screen } from "./Screen";

// The vault: files only you can see until you share them with the household.
export function Documents() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { session } = useSession();
  if (session.state !== "signedIn" || !session.household)
    throw new Error("page without a household");
  const { household } = session;
  const { documents, failed, reload } = useDocuments(household.id);
  const [sheet, setSheet] = useState<Document | null>(null);
  const [progress, setProgress] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const picker = useRef<HTMLInputElement>(null);

  async function add(file: File | undefined) {
    if (!file) return;
    if (file.size > maxDocumentMB << 20)
      return setError(t("documents.tooBig", { max: maxDocumentMB }));
    if (!typeOf(file)) return setError(t("documents.wrongType"));
    setError(null);
    setProgress(0);
    const sent = await uploadDocument(household.id, file, setProgress);
    setProgress(null);
    setError(sent.document ? null : (sent.error ?? t("documents.failed")));
    reload();
  }

  async function remove(d: Document) {
    if (!window.confirm(t("documents.deleteConfirm"))) return;
    await api
      .DELETE("/v1/households/{householdId}/documents/{documentId}", {
        params: { path: { householdId: household.id, documentId: d.id } },
      })
      .catch(() => undefined);
    reload();
  }

  const owner = (d: Document) =>
    household.members.find((m) => m.user.id === d.ownerId)?.user.name ?? "";
  const subtitle = (d: Document) =>
    [
      t(`documents.kind.${kindOf(d.contentType)}`),
      sizeLabel(d.size),
      d.mine
        ? t(d.shared ? "documents.shared" : "documents.private")
        : t("documents.from", { name: owner(d) }),
    ].join(" · ");
  return (
    <Screen
      title={t("documents.title")}
      back={
        <Button
          variant="plain"
          size="sm"
          className="back"
          onClick={() => navigate("/household")}
        >
          <span className="back-icon">
            <Icon name="chevron" size="sm" />
          </span>
          {t("tabs.household")}
        </Button>
      }
      add={{
        label: t("documents.add"),
        onClick: () => picker.current?.click(),
      }}
    >
      <input
        ref={picker}
        type="file"
        hidden
        onChange={(e) =>
          void add(e.target.files?.[0]).then(() => (e.target.value = ""))
        }
      />
      {(failed || error) && (
        <p className="callout error" role="alert">
          {error ?? t("documents.failed")}
        </p>
      )}
      {progress !== null && (
        <p className="callout muted" role="status">
          {t("documents.sending", { percent: Math.round(progress * 100) })}
        </p>
      )}
      {documents?.length === 0 && (
        <Empty
          title={t("documents.emptyTitle")}
          action={
            <Button size="sm" onClick={() => picker.current?.click()}>
              {t("documents.add")}
            </Button>
          }
        >
          {t("documents.emptyBody")}
        </Empty>
      )}
      {documents && documents.length > 0 && (
        <div className="hl-list">
          {documents.map((d) => {
            const row = (
              <ListRow
                title={d.name}
                subtitle={subtitle(d)}
                leading={<Icon name={d.shared ? "shared" : "lock"} />}
                onClick={() => setSheet(d)}
              />
            );
            return d.mine ? (
              <Swipe
                key={d.id}
                end={{
                  label: t("documents.delete"),
                  icon: "trash",
                  tone: "danger",
                  onAction: () => void remove(d),
                }}
              >
                {row}
              </Swipe>
            ) : (
              <div key={d.id}>{row}</div>
            );
          })}
        </div>
      )}
      {sheet && (
        <DocumentSheet
          household={household}
          document={sheet}
          onClose={() => setSheet(null)}
          onSaved={() => {
            setSheet(null);
            reload();
          }}
        />
      )}
    </Screen>
  );
}

// One document: open it, and if it's yours, rename it, share it with the household or delete it.
function DocumentSheet({
  household,
  document: d,
  onClose,
  onSaved,
}: {
  household: HouseholdDetail;
  document: Document;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(d.name);
  const [shared, setShared] = useState(d.shared);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const path = { householdId: household.id, documentId: d.id };

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const res = await api
      .PATCH("/v1/households/{householdId}/documents/{documentId}", {
        params: { path },
        body: { name: name.trim(), shared },
      })
      .catch(() => undefined);
    setBusy(false);
    if (res?.data) onSaved();
    else setError(res?.error?.message ?? t("documents.failed"));
  }

  async function remove() {
    if (!window.confirm(t("documents.deleteConfirm"))) return;
    setBusy(true);
    const res = await api
      .DELETE("/v1/households/{householdId}/documents/{documentId}", {
        params: { path },
      })
      .catch(() => undefined);
    setBusy(false);
    if (res?.response.ok) onSaved();
    else setError(res?.error?.message ?? t("documents.failed"));
  }

  return (
    <Sheet
      title={d.name}
      onClose={onClose}
      onSubmit={d.mine ? submit : undefined}
      leading={
        <Button variant="plain" onClick={onClose}>
          {t("entries.cancel")}
        </Button>
      }
      trailing={
        d.mine ? (
          <Button type="submit" size="sm" disabled={busy || !name.trim()}>
            {t("entries.save")}
          </Button>
        ) : undefined
      }
    >
      <a
        className="hl-btn hl-btn-secondary hl-btn-block"
        href={fileUrl(household.id, d)}
        target="_blank"
        rel="noreferrer"
      >
        {t("documents.open")}
      </a>
      {d.mine && (
        <>
          <label className="field">
            <span className="label">{t("documents.name")}</span>
            <input
              className="input"
              value={name}
              maxLength={200}
              onChange={(e) => setName(e.target.value)}
              enterKeyHint="done"
              required
            />
          </label>
          {household.members.length > 1 && (
            <FormGroup>
              <SwitchRow
                label={t("documents.shareWith")}
                checked={shared}
                onChange={setShared}
              />
            </FormGroup>
          )}
        </>
      )}
      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
      {d.mine && (
        <Button variant="danger" block disabled={busy} onClick={remove}>
          {t("documents.delete")}
        </Button>
      )}
    </Sheet>
  );
}
