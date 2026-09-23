import { QuickSessionComposer } from "@/components/quick-session-composer"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { useAppStore } from "@/state/app-store"

export function CreateSessionDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const selectSession = useAppStore((state) => state.selectSession)

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        onOpenChange(next)
      }}
    >
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>New session</DialogTitle>
          <DialogDescription>
            Start a Pi agent in an allowed project folder. Prompt is optional — expand More options for worktrees, batch create, and folder browse.
          </DialogDescription>
        </DialogHeader>
        {open && (
          <QuickSessionComposer
            variant="dialog"
            defaultMoreOptionsOpen
            onCreated={(sessionId) => selectSession(sessionId)}
            onRequestClose={() => onOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}
