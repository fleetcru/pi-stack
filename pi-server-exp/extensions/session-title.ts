import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { Type } from "typebox";

const MAX_TITLE_LENGTH = 60;

function titleFromPrompt(text: string): string | undefined {
  const cleaned = text
    .replace(/```[\s\S]*?```/g, " ")
    .replace(/[`*_#>[\]{}]/g, " ")
    .replace(/\s+/g, " ")
    .trim();
  if (!cleaned) return undefined;
  const words = cleaned.split(" ").slice(0, 8).join(" ");
  const title = words.charAt(0).toUpperCase() + words.slice(1);
  return title.length > MAX_TITLE_LENGTH ? `${title.slice(0, MAX_TITLE_LENGTH - 1).trimEnd()}…` : title;
}

/**
 * Gives every session an immediate useful title, then lets the agent replace
 * it with a concise task-oriented title once it understands the work.
 */
export default function sessionTitle(pi: ExtensionAPI) {
  pi.on("message_end", async (event) => {
    if (event.message.role !== "user" || pi.getSessionName()) return;
    const content = event.message.content;
    const text = typeof content === "string"
      ? content
      : Array.isArray(content)
        ? content.filter((part): part is { type: "text"; text: string } => part?.type === "text" && typeof part.text === "string").map((part) => part.text).join(" ")
        : "";
    const title = titleFromPrompt(text);
    if (title) pi.setSessionName(title);
  });

  pi.registerTool({
    name: "set_session_title",
    label: "Set session title",
    description: "Set a concise, task-oriented title for the current Pi session.",
    promptSnippet: "Set a concise title for the current task",
    promptGuidelines: ["Use set_session_title once the task is clear, and when the task changes materially. Keep titles under 60 characters."],
    parameters: Type.Object({ title: Type.String({ minLength: 1, maxLength: MAX_TITLE_LENGTH }) }),
    async execute(_toolCallId, params) {
      const title = params.title.trim().replace(/\s+/g, " ").slice(0, MAX_TITLE_LENGTH);
      pi.setSessionName(title);
      return { content: [{ type: "text", text: `Session title set to: ${title}` }] };
    },
  });
}
