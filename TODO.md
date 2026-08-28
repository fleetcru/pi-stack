# TODO

- Improve session loading so the app restores the active session quickly and reliably.
- Make session history rendering stable during initial load, refresh, reconnect, and pagination.
- Preserve scroll position and avoid duplicate or missing history items after reloads.
- Improve background and foreground handling so returning to the app reconnects cleanly and refreshes session state without losing messages.
- Test session recovery after app backgrounding, process death, network loss, and server reconnects.
