import "@testing-library/jest-dom/vitest";
import "./i18n";
import { afterEach } from "vitest";
import { forget } from "./cache";
import { clear } from "./outbox";

// What one test loaded is never there for the next.
afterEach(() => {
  forget();
  clear();
});
