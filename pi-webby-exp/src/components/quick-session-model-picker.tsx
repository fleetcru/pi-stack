import { Sparkles } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"

type Model = { provider: string; id: string; name?: string }

export function QuickSessionModelPicker({
  open,
  onOpenChange,
  models,
  modelsByProvider,
  selectedModel,
  selectedModelKey,
  loading,
  onSelectDefault,
  onSelectModel,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  models: Model[]
  modelsByProvider: Array<[string, Model[]]>
  selectedModel?: Model
  selectedModelKey: string
  loading: boolean
  onSelectDefault: () => void
  onSelectModel: (model: Model) => void
}) {
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger
        render={
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="max-w-64 justify-start px-2 text-muted-foreground"
            aria-label="Choose model"
            disabled={loading || models.length === 0}
          />
        }
      >
        <Sparkles data-icon="inline-start" />
        <span className="truncate text-foreground">
          {loading ? "Loading models..." : selectedModel?.name || selectedModel?.id || "Default model"}
        </span>
        {selectedModel && <span className="truncate text-xs">{selectedModel.provider}</span>}
      </PopoverTrigger>
      <PopoverContent align="start" className="w-96 max-w-[calc(100vw-2rem)] gap-0 p-0">
        <Command>
          <CommandInput placeholder="Search models..." />
          <CommandList>
            <CommandEmpty>No matching models.</CommandEmpty>
            <CommandGroup heading="Session default">
              <CommandItem
                value="default model"
                data-checked={selectedModelKey === "__default"}
                onSelect={onSelectDefault}
              >
                <Sparkles />
                <span>Default model</span>
              </CommandItem>
            </CommandGroup>
            {modelsByProvider.map(([modelProvider, providerModels]) => (
              <CommandGroup key={modelProvider} heading={modelProvider}>
                {providerModels.map((model) => {
                  const key = JSON.stringify([model.provider, model.id])
                  return (
                    <CommandItem
                      key={key}
                      value={`${model.name || model.id} ${model.id} ${model.provider}`}
                      data-checked={selectedModelKey === key}
                      onSelect={() => onSelectModel(model)}
                    >
                      <span className="min-w-0 flex-1 truncate">{model.name || model.id}</span>
                      {model.name && model.name !== model.id && (
                        <span className="max-w-40 truncate text-xs text-muted-foreground">{model.id}</span>
                      )}
                    </CommandItem>
                  )
                })}
              </CommandGroup>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
