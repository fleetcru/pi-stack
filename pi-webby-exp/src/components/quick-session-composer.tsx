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
  const flow = useQuickSessionCreateFlow({
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
        onSubmit={flow.submit}
        className={cn(
          "overflow-hidden rounded-[20px] border border-border/80 !bg-muted/70 shadow-[0_18px_60px_-36px_rgba(0,0,0,0.75)] ring-1 ring-foreground/[0.025] transition-[border-color,box-shadow] focus-within:border-foreground/30 focus-within:shadow-[0_22px_70px_-38px_rgba(0,0,0,0.9)]",
          variant === "dialog" && "shadow-none ring-0",
        )}
      >
        <Textarea
          ref={flow.promptRef}
          autoFocus
          value={flow.prompt}
          onChange={(event) => flow.setPrompt(event.target.value)}
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
            open={flow.modelPickerOpen}
            onOpenChange={flow.setModelPickerOpen}
            models={flow.models}
            modelsByProvider={flow.modelsByProvider}
            selectedModel={flow.selectedModel}
            selectedModelKey={flow.selectedModelKey}
            loading={flow.modelsQuery.isLoading}
            onSelectDefault={() => {
              flow.setProvider("")
              flow.setModelId("")
              flow.setModelPickerOpen(false)
            }}
            onSelectModel={(model) => {
              flow.setProvider(model.provider)
              flow.setModelId(model.id)
              flow.setModelPickerOpen(false)
            }}
          />
          <Select value={flow.thinkingLevel} onValueChange={(value) => flow.setThinkingLevel(String(value))}>
            <SelectTrigger
              size="sm"
              className="max-w-44 shrink-0 border-transparent bg-transparent px-2 text-muted-foreground shadow-none hover:bg-muted"
              aria-label="Thinking effort"
              title="Thinking effort"
            >
              <BrainCircuit />
              <span className="truncate text-foreground">
                {THINKING_LEVELS.find((level) => level.value === flow.thinkingLevel)?.label ?? "Default effort"}
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
          {variant === "dialog" || flow.moreOptionsOpen ? (
            <Button type="submit" size="sm" className="rounded-full px-3" aria-label={flow.submitLabel} disabled={!flow.canSubmit}>
              {flow.busy ? <LoaderCircle className="animate-spin" /> : null}
              {flow.busy ? "Starting…" : flow.sessionCount > 1 ? `Start ${flow.sessionCount}` : flow.prompt.trim() ? "Start" : "Create"}
            </Button>
          ) : (
            <Button
              type="submit"
              size="icon"
              className="rounded-full"
              aria-label="Create session and send message"
              disabled={!flow.canSubmit}
            >
              {flow.busy ? <LoaderCircle className="animate-spin" /> : <ArrowUp />}
            </Button>
          )}
        </div>
      </form>
      {flow.error && <p role="alert" className="mt-2 px-3 text-sm text-destructive">{flow.error}</p>}
      {flow.rootsQuery.isError && <p role="alert" className="mt-2 px-3 text-sm text-destructive">Could not load allowed project folders.</p>}
      <div className="mt-2 flex min-w-0 items-center gap-1 overflow-x-auto px-2 py-1 text-xs text-muted-foreground">
        <Select value={flow.effectiveCwd || null} onValueChange={(value) => flow.setCwd(String(value))}>
          <SelectTrigger
            size="sm"
            className="max-w-72 shrink-0 border-transparent bg-transparent px-2 shadow-none hover:bg-muted"
            aria-label="Project folder"
            title={flow.effectiveCwd || "No allowed project folders"}
            disabled={flow.rootsUnavailable && !flow.cwd}
          >
            <Folder />
            <span className="truncate">
              {flow.rootsQuery.isLoading
                ? "Loading projects..."
                : flow.selectedRoot?.name || (flow.effectiveCwd ? flow.effectiveCwd.split(/[\\/]/).filter(Boolean).at(-1) || flow.effectiveCwd : "No allowed projects")}
            </span>
          </SelectTrigger>
          <SelectContent side="bottom" align="start" alignItemWithTrigger={false} className="w-96 max-w-[calc(100vw-2rem)]">
            <SelectGroup>
              <SelectLabel>Allowed project folders</SelectLabel>
              {flow.roots.map((root) => (
                <SelectItem key={root.path} value={root.path}>
                  <span className="min-w-0 flex-1 truncate">{root.name}</span>
                  <span className="max-w-64 truncate text-xs text-muted-foreground">{root.path}</span>
                </SelectItem>
              ))}
              {flow.cwd && !flow.roots.some((root) => root.path === flow.cwd) && (
                <SelectItem value={flow.cwd}>
                  <span className="min-w-0 flex-1 truncate">{flow.cwd.split(/[\\/]/).filter(Boolean).at(-1) || flow.cwd}</span>
                  <span className="max-w-64 truncate text-xs text-muted-foreground">{flow.cwd}</span>
                </SelectItem>
              )}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Select value={flow.workerId} onValueChange={(value) => flow.changeWorker(String(value))}>
          <SelectTrigger size="sm" className="max-w-52 shrink-0 border-transparent bg-transparent px-2 shadow-none hover:bg-muted" aria-label="Worker">
            <Monitor />
            <span className="truncate">{flow.workerId === "local" ? "Local" : flow.selectedWorker?.id || flow.workerId}</span>
          </SelectTrigger>
          <SelectContent side="bottom" align="start" alignItemWithTrigger={false} className="min-w-56">
            <SelectGroup>
              <SelectLabel>Session location</SelectLabel>
              <SelectItem value="local">Local</SelectItem>
            </SelectGroup>
            {flow.remoteWorkers.length > 0 && (
              <SelectGroup>
                <SelectLabel>Remote workers</SelectLabel>
                {flow.remoteWorkers.map((worker) => (
                  <SelectItem key={worker.id} value={worker.id} disabled={worker.status === "error"}>
                    <span className="min-w-0 flex-1 truncate">{worker.id}</span>
                    {worker.status && <span className="text-xs text-muted-foreground">{worker.status}</span>}
                  </SelectItem>
                ))}
              </SelectGroup>
            )}
          </SelectContent>
        </Select>
        <Button
          type="button"
          size="xs"
          variant="ghost"
          aria-expanded={flow.moreOptionsOpen}
          aria-label="Advanced session options"
          onClick={() => flow.setMoreOptionsOpen((open) => !open)}
        >
          <SlidersHorizontal data-icon="inline-start" />
          Advanced
          <ChevronDown className={cn("size-3.5 transition-transform", flow.moreOptionsOpen && "rotate-180")} />
        </Button>
      </div>

      {flow.moreOptionsOpen && (
        <QuickSessionMoreOptions
          cwd={flow.cwd}
          effectiveCwd={flow.effectiveCwd}
          title={flow.title}
          isolated={flow.isolated}
          sessionCount={flow.sessionCount}
          args={flow.args}
          labels={flow.labels}
          advancedOpen={flow.advancedOpen}
          browsing={flow.browsing}
          browserLoading={flow.browserLoading}
          browserPath={flow.browserPath}
          browserParent={flow.browserParent}
          directories={flow.directories}
          browserError={flow.browserError}
          onCwdChange={flow.setCwd}
          onTitleChange={flow.setTitle}
          onIsolatedChange={flow.setIsolated}
          onSessionCountChange={flow.setSessionCount}
          onArgsChange={flow.setArgs}
          onLabelsChange={flow.setLabels}
          onAdvancedOpenChange={flow.setAdvancedOpen}
          onToggleBrowsing={() => {
            const next = !flow.browsing
            flow.setBrowsing(next)
            if (next) void flow.loadDirectories()
          }}
          onLoadDirectories={(path) => void flow.loadDirectories(path)}
          onUseBrowserPath={() => {
            if (flow.browserPath) {
              flow.setCwd(flow.browserPath)
              flow.setBrowsing(false)
            }
          }}
        />
      )}
    </div>
  )
}
