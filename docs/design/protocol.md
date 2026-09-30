# ccctl wire protocol (v1)

All routes pass the WhoIs allowlist and Host check. Mutating requests and the WebSocket
upgrade must carry `Origin` equal to `http://<Host>` (WS: Origin required; POST: if present).

## Types

```ts
type State = 'starting' | 'running' | 'needs-input' | 'exited' | 'resume-failed'
interface SlotInfo {
  slot: number            // 1..4
  task: string
  cwd: string             // working directory of claude
  repo: string            // display folder name, e.g. "folio"
  worktree: string        // worktree path or "" when none
  branch: string          // "ctl/<task>" or "" when none
  session_id: string
  state: State
  launched_at: string     // RFC3339
  resumed: boolean
  exit_code: number | null
  message: string         // exited / resume-failed text, verbatim from spec; "" otherwise
}
```

## REST (JSON)

| Method | Path | Body | Success | Errors |
|---|---|---|---|---|
| GET | `/api/state` | – | `{host, login, max_sessions: 4, slots: (SlotInfo\|null)[4]}` | – |
| GET | `/api/repos` | – | `{repos: [{name, path, display}]}` (display is `~/Documents/folio`) | – |
| GET | `/api/inspect?path=P` | – | `{path, git: bool, worktree_allowed: bool, note: string}` | 400 `{error}` for bad path |
| POST | `/api/sessions` | `{task, path, worktree: bool, prompt}` | 201 `{slot, info: SlotInfo}` | 400 `{error}` validation text; 409 `{error}` cap text |
| POST | `/api/sessions/{slot}/stop` | – | 202 `{}` | 404 |
| POST | `/api/sessions/{slot}/fresh` | – | 201 `{slot, info}` | 400/404 |
| POST | `/api/sessions/{slot}/close` | – | 200 `{}` (slot becomes null; only exited/resume-failed) | 400/404 |

## WebSocket `GET /ws`

Server → client:
- Text `{"type":"hello","host","login","slots":[...4]}` first.
- Then, per live slot, one binary replay frame: `[0x02, slot, ...bytes]` (client resets that terminal, writes bytes).
- Binary output: `[0x01, slot, ...bytes]`.
- Text `{"type":"slot","slot":n,"info":SlotInfo|null}` on any change (state, launch, close).
  A new session in a slot (new session_id) → client resets that terminal.

Client → server:
- Binary input: `[0x01, slot, ...bytes]`. Makes this client the size owner of that slot.
- Text `{"type":"resize","slot":n,"rows":r,"cols":c}`. Applied if this client owns the slot's size
  or nobody owns it yet; always remembered and applied on this client's next input.

Needs-input clears server-side on any input frame for that slot.
