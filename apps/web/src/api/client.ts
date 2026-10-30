import createClient from "openapi-fetch";
import type { components, paths } from "./schema.gen";

// Same origin: Caddy (production) or the Vite proxy (development) forwards API paths, so the
// HttpOnly session cookie travels with every call and no token ever reaches this code.
export const api = createClient<paths>({
  baseUrl: window.location.origin,
  credentials: "same-origin",
  // Looked up per call rather than captured once, so tests can stub fetch.
  fetch: (req) => globalThis.fetch(req),
});

export type User = components["schemas"]["User"];
export type Household = components["schemas"]["Household"];
export type HouseholdDetail = components["schemas"]["HouseholdDetail"];
export type Question = components["schemas"]["Question"];
export type Member = components["schemas"]["Member"];
export type Suggestions = components["schemas"]["Suggestions"];
export type Recipe = components["schemas"]["Recipe"];
export type PlannedMeal = components["schemas"]["PlannedMeal"];
export type MealIdea = components["schemas"]["MealIdea"];
export type Home = components["schemas"]["Home"];
export type Light = components["schemas"]["Light"];
export type Scene = components["schemas"]["Scene"];
export type Reading = components["schemas"]["Reading"];
export type Room = components["schemas"]["Room"];
export type WatchShelf = components["schemas"]["WatchShelf"];
export type WatchLibrary = components["schemas"]["WatchLibrary"];
export type WatchTitle = components["schemas"]["WatchTitle"];
export type WatchIdea = components["schemas"]["WatchIdea"];
export type TonightPick = components["schemas"]["TonightPick"];
export type Entry = components["schemas"]["Entry"];
export type Moment = components["schemas"]["Moment"];
export type MomentKind = components["schemas"]["MomentKind"];
export type MomentMonth = components["schemas"]["MomentMonth"];
export type Resurfaced = components["schemas"]["Resurfaced"];
export type ShoppingItem = components["schemas"]["ShoppingItem"];
export type ShoppingAisle = components["schemas"]["ShoppingAisle"];
export type EntryKind = components["schemas"]["EntryKind"];
export type EntryFields = components["schemas"]["EntryFields"];
export type Visibility = components["schemas"]["Visibility"];
export type Repeat = components["schemas"]["Repeat"];
export type Call = components["schemas"]["Call"];
export type Conversation = components["schemas"]["Conversation"];
export type ChatMessage = components["schemas"]["ChatMessage"];
export type ChatRequest = components["schemas"]["ChatRequest"];
export type McpToken = components["schemas"]["McpToken"];
export type TokenAccess = components["schemas"]["TokenAccess"];
export type VoiceSettings = components["schemas"]["VoiceSettings"];
export type ApiError = components["schemas"]["Error"];
