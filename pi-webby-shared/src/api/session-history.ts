import {
  useInfiniteQuery,
  useQueryClient,
} from "@tanstack/react-query"
import { useCallback, useEffect, useRef } from "react"

import {
  useAppStore,
  usePiServerClient,
  useServerConfigured,
  piQueryKeys,
} from "./hooks-base"

/** Match PiServerClient.getSessionMessages default page size so prefetch shares the cache. */
export const SESSION_HISTORY_PAGE_SIZE = 75
export const SESSION_HISTORY_STALE_TIME_MS = 30_000
export const SESSION_HISTORY_GC_TIME_MS = 30 * 60_000
export const SESSION_HISTORY_PREFETCH_DEBOUNCE_MS = 100

function sessionHistoryNextPageParam(page: { data?: unknown }) {
  const history = (page.data as { history?: { hasOlder?: boolean; nextOffset?: number } } | undefined)?.history
  return history?.hasOlder ? history.nextOffset : undefined
}

export function useSessionHistory(sessionId?: string) {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  // queryKey includes sessionId so a remounted SessionWorkspace immediately
  // receives that session's cached pages (never another session's messages).
  // Prefetch + gcTime keep recently hovered/visited sessions warm across switches.
  return useInfiniteQuery({
    queryKey: piQueryKeys.sessionHistory(client.cacheScope, sessionId ?? "none"),
    queryFn: ({ pageParam }) => client.getSessionMessages(sessionId!, pageParam, SESSION_HISTORY_PAGE_SIZE),
    initialPageParam: 0,
    getNextPageParam: sessionHistoryNextPageParam,
    enabled: configured && Boolean(sessionId),
    staleTime: SESSION_HISTORY_STALE_TIME_MS,
    gcTime: SESSION_HISTORY_GC_TIME_MS,
    maxPages: 10,
  })
}

/**
 * Prefetch the first/newest history page for a session into the shared React Query
 * cache used by `useSessionHistory`. No-ops for the already-selected session.
 */
export function usePrefetchSessionHistory() {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  const queryClient = useQueryClient()
  const selectedSessionId = useAppStore((state) => state.selectedSessionId)

  return useCallback((sessionId: string) => {
    if (!configured || !sessionId || sessionId === selectedSessionId) return
    void queryClient.prefetchInfiniteQuery({
      queryKey: piQueryKeys.sessionHistory(client.cacheScope, sessionId),
      queryFn: ({ pageParam }) => client.getSessionMessages(sessionId, pageParam, SESSION_HISTORY_PAGE_SIZE),
      initialPageParam: 0,
      getNextPageParam: sessionHistoryNextPageParam,
      staleTime: SESSION_HISTORY_STALE_TIME_MS,
      pages: 1,
    })
  }, [client, configured, queryClient, selectedSessionId])
}

/** Pure debounce helper for sidebar hover/focus prefetch (unit-testable). */
export function createDebouncedSessionHistoryPrefetch(
  prefetch: (sessionId: string) => void,
  delayMs = SESSION_HISTORY_PREFETCH_DEBOUNCE_MS,
) {
  let timer: ReturnType<typeof setTimeout> | undefined
  return {
    schedule(sessionId: string) {
      if (timer !== undefined) clearTimeout(timer)
      timer = setTimeout(() => {
        timer = undefined
        prefetch(sessionId)
      }, delayMs)
    },
    cancel() {
      if (timer !== undefined) {
        clearTimeout(timer)
        timer = undefined
      }
    },
  }
}

/**
 * Debounced mouse/focus handlers that prefetch session history for sidebar rows.
 * Cancels the pending prefetch when the pointer/focus leaves before the delay.
 */
export function useSessionHistoryPrefetchHandlers(sessionId: string, disabled = false) {
  const prefetch = usePrefetchSessionHistory()
  const prefetchRef = useRef(prefetch)
  prefetchRef.current = prefetch

  const schedulerRef = useRef<ReturnType<typeof createDebouncedSessionHistoryPrefetch> | null>(null)
  if (schedulerRef.current === null) {
    schedulerRef.current = createDebouncedSessionHistoryPrefetch((id) => prefetchRef.current(id))
  }

  useEffect(() => () => schedulerRef.current?.cancel(), [])

  const schedule = useCallback(() => {
    if (disabled) return
    schedulerRef.current?.schedule(sessionId)
  }, [disabled, sessionId])

  const cancel = useCallback(() => {
    schedulerRef.current?.cancel()
  }, [])

  return {
    onMouseEnter: schedule,
    onMouseLeave: cancel,
    onFocus: schedule,
    onBlur: cancel,
  }
}
