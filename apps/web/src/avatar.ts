import { api } from "./api/client";

// Pictures are cropped to a square of this many pixels before they are sent.
const avatarSize = 512;

// square crops a picked photo to its centre and makes it a small JPEG; createImageBitmap reads
// HEIC on iOS and turns the picture upright.
export async function square(file: Blob): Promise<Blob> {
  const bitmap = await createImageBitmap(file);
  const side = Math.min(bitmap.width, bitmap.height);
  const canvas = document.createElement("canvas");
  canvas.width = canvas.height = Math.min(avatarSize, side);
  canvas.getContext("2d")?.drawImage(bitmap, (bitmap.width - side) / 2, (bitmap.height - side) / 2, side, side, 0, 0, canvas.width, canvas.height);
  bitmap.close();
  return new Promise((resolve, reject) => canvas.toBlob((b) => (b ? resolve(b) : reject(new Error("photo"))), "image/jpeg", 0.85));
}

// Whose picture it is: your own, or, as the household's owner, another member's.
export type AvatarOwner = { householdId: string; userId: string } | undefined;

export async function uploadAvatar(photo: Blob, member?: AvatarOwner) {
  // The contract types a binary body as a string; the photo goes as it is.
  const init = { body: photo as unknown as string, bodySerializer: (b: string) => b, headers: { "Content-Type": "image/jpeg" } };
  const res = await (member
    ? api.PUT("/v1/households/{householdId}/members/{userId}/avatar", { ...init, params: { path: member } })
    : api.PUT("/v1/me/avatar", init)
  ).catch(() => undefined);
  return res?.data;
}

export async function removeAvatar(member?: AvatarOwner) {
  const res = await (member
    ? api.DELETE("/v1/households/{householdId}/members/{userId}/avatar", { params: { path: member } })
    : api.DELETE("/v1/me/avatar")
  ).catch(() => undefined);
  return res?.data;
}

// ownAvatar tells a picture set in Homeloom from the one the sign-in sends.
export function ownAvatar(url: string | undefined) {
  return !!url?.startsWith("/v1/users/");
}
