export type SlashCommand = { name: string; description: string }

// Keep this list intentionally small and stable. Pi also supports user-defined
// commands, which remain usable even when they are not listed here.
export function matchingSlashCommands(value: string, available: SlashCommand[] = []): SlashCommand[] {
  const token = value.trim().split(/\s+/, 1)[0]?.toLowerCase() ?? ""
  if (!token.startsWith("/")) return []
  return available.filter((command) => command.name.startsWith(token))
}

export function parseSlashCommands(data: unknown): SlashCommand[] {
  const commands = (data as { data?: { commands?: unknown[] } } | undefined)?.data?.commands
  if (!Array.isArray(commands)) return []
  return commands.flatMap((item) => {
    if (typeof item === "string") return [{ name: item.startsWith("/") ? item : `/${item}`, description: "Pi command" }]
    if (!item || typeof item !== "object") return []
    const value = item as { name?: unknown; command?: unknown; description?: unknown; source?: unknown }
    const rawName = typeof value.name === "string" ? value.name : typeof value.command === "string" ? value.command : ""
    if (!rawName) return []
    return [{ name: rawName.startsWith("/") ? rawName : `/${rawName}`, description: typeof value.description === "string" ? value.description : typeof value.source === "string" ? value.source : "Pi command" }]
  })
}
