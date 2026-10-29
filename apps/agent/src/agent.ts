import { cli, defineAgent, llm, ServerOptions, voice, type JobContext, type JobProcess } from "@livekit/agents";
import * as anthropic from "@livekit/agents-plugin-anthropic";
import * as openai from "@livekit/agents-plugin-openai";
import * as silero from "@livekit/agents-plugin-silero";
import { createServer } from "node:http";
import { fileURLToPath } from "node:url";
import { greeting, instructions, parseCaller } from "./caller.js";
import { DTLN, loadModels, sampleRate, type Models } from "./dtln.js";

type TTSVoice = NonNullable<ConstructorParameters<typeof openai.TTS>[0]>["voice"];

// The API dispatches the agent by this name when someone taps the phone in the assistant sheet.
const agentName = "homeloom";
const mcpURL = process.env.HOMELOOM_MCP_URL || "http://homeloom-api:8080/mcp";
const model = process.env.VOICE_LLM_MODEL || "claude-sonnet-5-5";
const timeZone = process.env.TIME_ZONE || "UTC";
// "dtln" denoises the caller's microphone before speech-to-text; "off" leaves it as it is.
const noiseCancellation = (process.env.VOICE_NOISE_CANCELLATION || "dtln").toLowerCase();
const modelDir = fileURLToPath(new URL("../models/", import.meta.url));

export default defineAgent({
  prewarm: async (proc: JobProcess) => {
    proc.userData.vad = await silero.VAD.load();
    if (noiseCancellation === "dtln") {
      // Without the models the agent still answers, just without noise cancellation.
      proc.userData.dtln = await loadModels(modelDir).catch((err) => {
        console.warn(`noise cancellation is off: ${err}`);
        return undefined;
      });
    }
  },
  entry: async (ctx: JobContext) => {
    const caller = parseCaller(ctx.job.metadata);
    const dtln = ctx.proc.userData.dtln as Models | undefined;
    // Homeloom's own tools, acting as the caller with the key the API minted for this call.
    const homeloom = new llm.MCPServerHTTP({
      url: mcpURL,
      transportType: "streamable_http",
      headers: { Authorization: `Bearer ${caller.key}` },
      allowInsecureHttp: mcpURL.startsWith("http:"),
    });
    ctx.addShutdownCallback(() => homeloom.aclose());

    const session = new voice.AgentSession({
      vad: ctx.proc.userData.vad as silero.VAD,
      stt: new openai.STT({ model: "gpt-4o-transcribe", language: caller.lang }),
      llm: new anthropic.LLM({ model }),
      tts: new openai.TTS({
        model: "gpt-4o-mini-tts",
        // The plugin lists only the older voices; the API checks the name against the model's.
        voice: caller.voice.voice as TTSVoice,
        speed: caller.voice.speed,
        instructions: caller.voice.instructions,
      }),
    });

    // Calls are end-to-end encrypted: the agent decrypts with the call's key, which LiveKit never sees.
    await ctx.connect({ keyProviderOptions: { sharedKey: new TextEncoder().encode(caller.e2ee) } });
    await session.start({
      agent: new voice.Agent({
        instructions: instructions(caller, new Date(), timeZone),
        tools: [new llm.MCPToolset({ id: "homeloom", mcpServer: homeloom })],
      }),
      room: ctx.room,
      // DTLN runs at 16 kHz mono, which is plenty for voice detection and speech-to-text.
      inputOptions: dtln && { audioSampleRate: sampleRate, audioNumChannels: 1, noiseCancellation: new DTLN(dtln) },
    });
    session.say(greeting(caller));
  },
});

// Without speech or Claude there's nothing to run, and the API keeps the feature off too. The
// container stays up and healthy rather than exiting, so the stack doesn't look broken.
if (!process.env.OPENAI_API_KEY || !process.env.ANTHROPIC_API_KEY) {
  console.log("OPENAI_API_KEY or ANTHROPIC_API_KEY is not set; the voice agent is off");
  createServer((_, res) => res.end("off")).listen(8081);
} else {
  // Coolify passes an unset variable as an empty string, which the CLI rejects as a log level.
  if (!process.env.LOG_LEVEL) delete process.env.LOG_LEVEL;
  cli.runApp(new ServerOptions({ agent: fileURLToPath(import.meta.url), agentName }));
}
