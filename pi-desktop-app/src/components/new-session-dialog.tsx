import { QuickSessionComposer } from "@/components/quick-session-composer"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { useAppStore } from "@/state/app-store"

/** Thin modal that embeds only QuickSessionComposer (no legacy create form). */
export function NewSessionDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const selectSession = useAppStore((state) => state.selectSession)

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>New session</DialogTitle>
          <DialogDescription>
            Start a Pi agent with the same composer as the home workspace. Prompt is optional.
          </DialogDescription>
        </DialogHeader>
        {open && (
          <QuickSessionComposer
            variant="dialog"
            onCreated={(sessionId) => selectSession(sessionId)}
            onRequestClose={() => onOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}
