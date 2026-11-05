import { useCallback, useEffect, useState } from "react";
import { api, type Document } from "./api/client";
import { local } from "./outbox";
import { useRefresh } from "./refresh";

// The largest file the API takes, in MB.
export const maxDocumentMB = 50;

// The documents you can see: your own and those shared with the household.
export function useDocuments(householdId: string) {
  const [documents, setDocuments] = useState<Document[] | null>(
    () => local<Document[]>(`/v1/households/${householdId}/documents`) ?? null,
  );
  const [failed, setFailed] = useState(false);
  const [generation, setGeneration] = useState(0);
  const { generation: refreshed, track } = useRefresh();

  useEffect(() => {
    let current = true;
    const request = api
      .GET("/v1/households/{householdId}/documents", {
        params: { path: { householdId } },
      })
      .then((res) => {
        if (!current) return;
        if (res.data) setDocuments(res.data);
        setFailed(!res.data);
      })
      .catch(() => current && setFailed(true));
    track(request);
    return () => {
      current = false;
    };
  }, [householdId, generation, refreshed, track]);

  const reload = useCallback(() => setGeneration((g) => g + 1), []);
  return { documents, failed, reload };
}

export const fileUrl = (householdId: string, d: Document) =>
  `/v1/households/${householdId}/documents/${d.id}/file`;

// Phones sometimes pick a file without saying what it is; its ending tells.
const byEnding: Record<string, string> = {
  pdf: "application/pdf",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  png: "image/png",
  webp: "image/webp",
  heic: "image/heic",
  txt: "text/plain",
  csv: "text/csv",
  doc: "application/msword",
  docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
  xls: "application/vnd.ms-excel",
  xlsx: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
  ppt: "application/vnd.ms-powerpoint",
  pptx: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
  odt: "application/vnd.oasis.opendocument.text",
  ods: "application/vnd.oasis.opendocument.spreadsheet",
};

export function typeOf(file: File) {
  const type = file.type.split(";")[0]?.trim().toLowerCase();
  return (
    type || byEnding[file.name.split(".").pop()?.toLowerCase() ?? ""] || ""
  );
}

// A short word for a file's kind, such as "PDF" or "Image".
export function kindOf(
  contentType: string,
): "pdf" | "image" | "text" | "office" {
  if (contentType === "application/pdf") return "pdf";
  if (contentType.startsWith("image/")) return "image";
  if (contentType.startsWith("text/")) return "text";
  return "office";
}

export function sizeLabel(bytes: number) {
  if (bytes < 1 << 20) return `${Math.max(1, Math.round(bytes / 1024))} KB`;
  return `${(bytes / (1 << 20)).toFixed(1)} MB`;
}

// uploadDocument sends a picked file as it is, reporting how much has gone. It uses XHR, because
// fetch can't tell how far an upload is. It resolves to the document, or to the API's message.
export function uploadDocument(
  householdId: string,
  file: File,
  onProgress: (share: number) => void,
) {
  return new Promise<{ document?: Document; error?: string }>((resolve) => {
    const xhr = new XMLHttpRequest();
    xhr.open(
      "POST",
      `/v1/households/${householdId}/documents?name=${encodeURIComponent(file.name)}`,
    );
    xhr.setRequestHeader("Content-Type", typeOf(file));
    xhr.responseType = "json";
    xhr.upload.onprogress = (e) =>
      e.lengthComputable && onProgress(e.loaded / e.total);
    xhr.onload = () => {
      const body = xhr.response as (Document & { message?: string }) | null;
      resolve(
        xhr.status === 201 && body
          ? { document: body }
          : { error: body?.message },
      );
    };
    xhr.onerror = () => resolve({});
    xhr.send(file);
  });
}
