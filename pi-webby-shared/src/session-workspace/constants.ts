/**
 * Shared constants for timeline rendering thresholds.
 * Used by both web and desktop clients.
 */

/** Above this size, markdown rendering is deferred behind an expand button. */
export const LARGE_RENDER_THRESHOLD = 60_000

/** Above this size, markdown is rendered as plain <pre> to avoid freezing. */
export const MARKDOWN_HARD_CAP = 256_000

/** Tool output preview is truncated at this size. */
export const LARGE_TOOL_OUTPUT_THRESHOLD = 80_000
