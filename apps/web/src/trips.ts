import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, type Trip } from "./api/client";
import { toLocalDate } from "./entries";
import { local } from "./outbox";
import { useRefresh } from "./refresh";

// The household's trips, soonest first, each with its places, votes and packing list. Every
// change to a trip answers with the trip as it is then, which apply puts in its place.
export function useTrips(householdId: string) {
  const [trips, setTrips] = useState<Trip[] | null>(() => local<Trip[]>(`/v1/households/${householdId}/trips`) ?? null);
  const [failed, setFailed] = useState(false);
  const [generation, setGeneration] = useState(0);
  const { generation: refreshed, track } = useRefresh();

  useEffect(() => {
    let current = true;
    const request = api
      .GET("/v1/households/{householdId}/trips", { params: { path: { householdId } } })
      .then((res) => {
        if (!current) return;
        if (res.data) setTrips(res.data);
        setFailed(!res.data);
      })
      .catch(() => current && setFailed(true));
    track(request);
    return () => {
      current = false;
    };
  }, [householdId, generation, refreshed, track]);

  const reload = useCallback(() => setGeneration((g) => g + 1), []);
  const apply = useCallback((trip: Trip) => setTrips((ts) => ts?.map((x) => (x.id === trip.id ? trip : x)) ?? ts), []);
  return { trips, failed, reload, apply };
}

// The trip to show first: the one under way or coming up next, else one without dates.
export function nextTrip(trips: Trip[], now = toLocalDate(new Date().toISOString())): Trip | undefined {
  const upcoming = trips.filter((t) => t.startOn && (t.endOn ?? t.startOn) >= now);
  return upcoming[0] ?? trips.find((t) => !t.startOn);
}

// A trip's dates as short text: "10 – 24 Jul 2027", or just the start; empty without dates.
export function useTripDates() {
  const { i18n } = useTranslation();
  return (trip: Pick<Trip, "startOn" | "endOn">) => {
    if (!trip.startOn) return "";
    const start = new Date(`${trip.startOn}T00:00`);
    const fmt = new Intl.DateTimeFormat(i18n.language, { day: "numeric", month: "short", year: "numeric" });
    if (!trip.endOn || trip.endOn === trip.startOn) return fmt.format(start);
    return fmt.formatRange(start, new Date(`${trip.endOn}T00:00`));
  };
}

// How many of the packing list are ticked off.
export const packed = (trip: Trip) => trip.packing.filter((p) => p.done).length;

// The option most members favour, if one leads on its own.
export function leading(trip: Trip) {
  const best = Math.max(0, ...trip.options.map((o) => o.votes.length));
  const top = trip.options.filter((o) => o.votes.length === best);
  return best > 0 && top.length === 1 ? top[0] : undefined;
}

// A trip's cover: the chosen place's first photo, else the first photo of any place.
export function cover(trip: Trip) {
  const chosen = trip.options.find((o) => o.id === trip.chosenOptionId);
  return chosen?.photos[0]?.url ?? trip.options.find((o) => o.photos.length > 0)?.photos[0]?.url;
}

// Sends a picked JPEG to a place; the trip as it is then, or the API's message.
export async function uploadTripPhoto(householdId: string, tripId: string, optionId: string, photo: Blob) {
  const res = await api
    .POST("/v1/households/{householdId}/trips/{tripId}/options/{optionId}/photos", {
      params: { path: { householdId, tripId, optionId } },
      // The contract types a binary body as a string; the photo goes as it is.
      body: photo as unknown as string,
      bodySerializer: (b) => b,
      headers: { "Content-Type": "image/jpeg" },
    })
    .catch(() => undefined);
  return { trip: res?.data, error: res?.error?.message };
}
