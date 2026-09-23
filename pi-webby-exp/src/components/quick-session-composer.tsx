import {
  ArrowUp,
  BrainCircuit,
  ChevronDown,
  Folder,
  LoaderCircle,
  Monitor,
  SlidersHorizontal,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { QuickSessionModelPicker } from "@/components/quick-session-model-picker"
import { QuickSessionMoreOptions } from "@/components/quick-session-more-options"
import { THINKING_LEVELS } from "@/components/quick-session-composer-state"
import { useQuickSessionCreateFlow } from "@/components/use-quick-session-create-flow"
import { cn } from "@/lib/utils"

export function QuickSessionComposer({
  onCreated,
  variant = "inline",
  defaultMoreOptionsOpen = false,
  expandMoreOptionsToken,
  focusToken,
  onRequestClose,
}: {
  onCreated: (sessionId: string) => void
  /** inline = empty workspace card; dialog = modal create surface */
  variant?: "inline" | "dialog"
  defaultMoreOptionsOpen?: boolean
  /** Bump to expand the in-composer Advanced panel (tests / deep-links). */
  expandMoreOptionsToken?: number
  /** Bump to focus the prompt (sidebar + on empty workspace). */
  focusToken?: number
  onRequestClose?: () => void
}) {
  // Destructure so eslint react-hooks/refs does not treat property access on an
  // object that also contains promptRef as "accessing a ref during render".
  const {
    promptRef,
    prompt, setPrompt,
    error,
    workerId,
    cwd, setCwd,
    thinkingLevel, setThinkingLevel,
    modelPickerOpen, setModelPickerOpen,
    moreOptionsOpen, setMoreOptionsOpen,
    title, setTitle,
    isolated, setIsolated,
    sessionCount, setSessionCount,
    args, setArgs,
    labels, setLabels,
    advancedOpen, setAdvancedOpen,
    browsing, setBrowsing,
    browserLoading,
    browserPath,
    browserParent,
    directories,
    browserError,
    rootsQuery,
    modelsQuery,
    roots,
    remoteWorkers,
    effectiveCwd,
    selectedRoot,
    selectedWorker,
    selectedModelKey,
    selectedModel,
    models,
    modelsByProvider,
    setProvider,
    setModelId,
    busy,
    rootsUnavailable,
    canSubmit,
    submitLabel,
    loadDirectories,
    changeWorker,
    submit,
  } = useQuickSessionCreateFlow({
    variant,
    defaultMoreOptionsOpen,
    expandMoreOptionsToken,
    focusToken,
    onCreated,
    onRequestClose,
  })

  return (
    <div className={cn("w-full", variant === "inline" ? "max-w-4xl" : "max-w-none")}>
      <form
        onSubmit={submit}
        className={cn(
          "overflow-hidden rounded-[20px] border border-border/80 !bg-muted/70 shadow-[0_18px_60px_-36px_rgba(0,0,0,0.75)] ring-1 ring-foreground/[0.025] transition-[border-color,box-shadow] focus-within:border-foreground/30 focus-within:shadow-[0_22px_70px_-38px_rgba(0,0,0,0.9)]",
          variant === "dialog" && "shadow-none ring-0",
        )}
      >
        <Textarea
          ref={promptRef}
          autoFocus
          value={prompt}
          onChange={(event) => setPrompt(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
              event.preventDefault()
              event.currentTarget.form?.requestSubmit()
            }
          }}
          placeholder="Do anything..."
          aria-label="First message for the new session"
          className="min-h-20 resize-none rounded-none border-0 !bg-muted/70 px-5 pt-5 pb-3 text-sm leading-6 shadow-none focus-visible:ring-0"
        />
        <div className="flex items-center gap-1 overflow-x-auto !bg-muted/70 px-3 py-2">
          <QuickSessionModelPicker
            open={modelPickerOpen}
            onOpenChange={setModelPickerOpen}
            models={models}
            modelsByProvider={modelsByProvider}
            selectedModel={selectedModel}
            selectedModelKey={selectedModelKey}
            loading={modelsQuery.isLoading}
            onSelectDefault={() => {
              setProvider("")
              setModelId("")
              setModelPickerOpen(false)
            }}
            onSelectModel={(model) => {
              setProvider(model.provider)
              setModelId(model.id)
              setModelPickerOpen(false)
            }}
          />
          <Select value={thinkingLevel} onValueChange={(value) => setThinkingLevel(String(value))}>
            <SelectTrigger
              size="sm"
              className="max-w-44 shrink-0 border-transparent bg-transparent px-2 text-muted-foreground shadow-none hover:bg-muted"
              aria-label="Thinking effort"
              title="Thinking effort"
            >
              <BrainCircuit />
              <span className="truncate text-foreground">
                {THINKING_LEVELS.find((level) => level.value === thinkingLevel)?.label ?? "Default effort"}
              </span>
            </SelectTrigger>
            <SelectContent side="bottom" align="start" alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectLabel>Thinking / effort</SelectLabel>
                {THINKING_LEVELS.map((level) => (
                  <SelectItem key={level.value} value={level.value}>{level.label}</SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <div className="min-w-2 flex-1" />
          <span className="hidden shrink-0 px-2 text-[11px] text-muted-foreground sm:inline">Ctrl/⌘ + Enter</span>
          {variant === "dialog" || moreOptionsOpen ? (
            <Button type="submit" size="sm" className="rounded-full px-3" aria-label={submitLabel} disabled={!canSubmit}>
              {busy ? <LoaderCircle className="animate-spin" /> : null}
              {busy ? "Starting…" : sessionCount > 1 ? `Start ${sessionCount}` : prompt.trim() ? "Start" : "Create"}
            </Button>
          ) : (
            <Button
              type="submit"
              size="icon"
              className="rounded-full"
              aria-label="Create session and send message"
              disabled={!canSubmit}
            >
              {busy ? <LoaderCircle className="animate-spin" /> : <ArrowUp />}
            </Button>
          )}
        </div>
      </form>
      {error && <p role="alert" className="mt-2 px-3 text-sm text-destructive">{error}</p>}
      {rootsQuery.isError && <p role="alert" className="mt and-2 px-3 text-sm text-destructive">Could not load allowed project folders.</p>}
