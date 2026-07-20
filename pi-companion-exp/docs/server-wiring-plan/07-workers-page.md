# 07 Workers Page

Purpose: replace placeholder with real local/remote worker status.

Create:

```text
ui/workers/WorkersScreen.kt
ui/workers/WorkerCard.kt
ui/workers/WorkersViewModel.kt
data/repository/WorkersRepository.kt
```

Endpoint:

```text
GET /v1/workers
```

Display:
- worker name/id
- host
- status/heartbeat
- active sessions
- capacity
- tags

Actions later:
- create session on selected worker
- refresh worker list
- remove unavailable worker if server supports it

UI states:
- loading
- content
- empty
- error

Acceptance:
- Workers tab no longer says coming soon
- shows real worker list or meaningful empty state
