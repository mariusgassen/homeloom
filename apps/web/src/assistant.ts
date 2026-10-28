import { createContext, useContext } from "react";
import type { ApiError, ChatMessage, ChatRequest, Conversation } from "./api/client";

// Opens the assistant from any screen's header.
export const AssistantContext = createContext<{ open: () => void }>({ open: () => undefined });
export const useAssistant = () => useContext(AssistantContext);

export type ChatEvents = {
  conversation: (c: Conversation) => void;
  tool: (name: string) => void;
  text: (text: string) => void;
  done: (m: ChatMessage) => void;
  error: (e: ApiError) => void;
};

// chat sends a message and reads the answer as it streams in (server-sent events on a POST, which
// EventSource can't do). A failure before the stream starts arrives as an error event too.
export async function chat(householdId: string, body: ChatRequest, on: ChatEvents, signal?: AbortSignal) {
  const failed: ApiError = { code: "internal", message: "" };
  let res: Response;
  try {
    res = await globalThis.fetch(
      new Request(`${window.location.origin}/v1/households/${householdId}/chat`, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
        body: JSON.stringify(body),
        signal,
      }),
    );
  } catch {
    on.error(failed);
    return;
  }
  if (!res.ok || !res.body) {
    on.error(((await res.json().catch(() => null)) as ApiError | null) ?? failed);
    return;
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let finished = false;
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      let end: number;
      while ((end = buffer.indexOf("\n\n")) >= 0) {
        const block = buffer.slice(0, end);
        buffer = buffer.slice(end + 2);
        let event = "message";
        let data = "";
        for (const line of block.split("\n")) {
          if (line.startsWith("event: ")) event = line.slice(7);
          else if (line.startsWith("data: ")) data += line.slice(6);
        }
        const v = JSON.parse(data || "null");
        switch (event) {
          case "conversation":
            on.conversation(v);
            break;
          case "tool":
            on.tool(v.name);
            break;
          case "text":
            on.text(v.text);
            break;
          case "done":
            finished = true;
            on.done(v);
            break;
          case "error":
            finished = true;
            on.error(v);
            break;
        }
      }
    }
  } catch {
    // The connection dropped; the answer is still stored on the server.
  }
  if (!finished) on.error(failed);
}

// toolStep is the catalog key for what the assistant is doing while it uses a tool.
export function toolStep(name: string) {
  if (name === "list_entries") return "assistant.steps.lists";
  if (name.startsWith("add_") && name !== "add_to_shopping_list") return "assistant.steps.write";
  if (name === "complete_reminder") return "assistant.steps.done";
  if (name.includes("recipe")) return "assistant.steps.recipes";
  if (name.includes("shopping")) return "assistant.steps.shopping";
  if (["get_home", "activate_scene", "set_light"].includes(name)) return "assistant.steps.home";
  return "assistant.steps.other";
}
