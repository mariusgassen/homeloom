import { useState } from "react";
import { useTranslation } from "react-i18next";
import { api, type Moment, type User } from "../api/client";
import { Icon } from "../ui/Icon";

// The emoji a moment can be answered with; the API accepts only these.
const emoji = ["❤️", "😂", "🥹", "🙌", "👍"] as const;

// What each of you answered a moment with: the emoji in use with their counts, and a button that
// opens the choice. Tapping your own emoji again takes it off.
export function Reactions({ moment, me }: { moment: Moment; me: User }) {
  const { t } = useTranslation();
  const [reactions, setReactions] = useState(moment.reactions ?? []);
  const [choosing, setChoosing] = useState(false);
  const mine = reactions.find((r) => r.userId === me.id)?.emoji;

  async function toggle(e: (typeof emoji)[number]) {
    setChoosing(false);
    const params = { params: { path: { momentId: moment.id } } };
    const res = await (mine === e ? api.DELETE("/v1/moments/{momentId}/reaction", params) : api.PUT("/v1/moments/{momentId}/reaction", { ...params, body: { emoji: e } })).catch(() => undefined);
    if (res?.data) setReactions(res.data.reactions ?? []);
  }

  const used = emoji.filter((e) => reactions.some((r) => r.emoji === e));
  return (
    <div className="reactions">
      {used.map((e) => (
        <button key={e} type="button" className="reaction" aria-pressed={mine === e} onClick={() => toggle(e)}>
          {e} {reactions.filter((r) => r.emoji === e).length}
        </button>
      ))}
      {choosing ? (
        emoji.map((e) => (
          <button key={e} type="button" className="reaction" onClick={() => toggle(e)}>
            {e}
          </button>
        ))
      ) : (
        <button type="button" className="reaction" aria-label={t("moments.react")} onClick={() => setChoosing(true)}>
          <Icon name="plus" size="sm" />
        </button>
      )}
    </div>
  );
}
