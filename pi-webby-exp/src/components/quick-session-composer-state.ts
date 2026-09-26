export const THINKING_LEVELS = [
  { value: "__default", label: "Default effort" },
  { value: "off", label: "Off" },
  { value: "minimal", label: "Minimal" },
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
  { value: "max", label: "Maximum" },
  { value: "xhigh", label: "Extra high" },
  { value: "ultra", label: "Ultra" },
] as const

export function initialComposerState() {
  return {
    prompt: "",
    error: undefined as string | undefined,
    submitting: false,
    draftSessionId: undefined as string | undefined,
    workerId: "local",
    cwd: "",
    provider: "",
    modelId: "",
    thinkingLevel: "__default",
    title: "",
    isolated: false,
    sessionCount: 1,
    args: "",
    labels: "",
    browsing: false,
    browserLoading: false,
    browserPath: undefined as string | undefined,
    browserParent: undefined as string | undefined,
    directories: [] as Array<{ name: string; path: string }>,
    browserError: undefined as string | undefined,
    advancedOpen: false,
  }
}
