import { useEffect, useId, useState, type InputHTMLAttributes } from "react";
import { api } from "../api/client";

// A text field for a town that suggests places as you type, through the browser's own suggestion
// list. When the search is down it is just a text field.
export function PlaceInput({ value, onChange, ...rest }: { value: string; onChange: (v: string) => void } & Omit<InputHTMLAttributes<HTMLInputElement>, "value" | "onChange" | "list">) {
  const listId = useId();
  const [labels, setLabels] = useState<string[]>([]);

  useEffect(() => {
    const text = value.trim();
    if (text.length < 2) return;
    let current = true;
    const timer = setTimeout(() => {
      api
        .GET("/v1/places", { params: { query: { q: text } } })
        .catch(() => undefined)
        .then((res) => current && setLabels(res?.data?.map((p) => p.label) ?? []));
    }, 250);
    return () => {
      current = false;
      clearTimeout(timer);
    };
  }, [value]);

  return (
    <>
      <input {...rest} value={value} list={listId} onChange={(e) => onChange(e.target.value)} />
      <datalist id={listId}>
        {(value.trim().length < 2 ? [] : labels).map((l) => (
          <option key={l} value={l} />
        ))}
      </datalist>
    </>
  );
}
