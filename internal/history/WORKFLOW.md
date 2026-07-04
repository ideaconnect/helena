# history — workflows

## Recording a send (#65)

The GUI records every interactive send; the headless runner does not.

```
session.New(cfgPath)
  -> history.New(historyPath(cfgPath), 0)   (history.yml next to config.yml; "" → in-memory)
  -> s.history                              (reachable via Session.History())

MainUI.send()  (internal/ui/send.go, worker goroutine)
  -> exec.ExecuteOnce(...) -> view
  -> m.recordHistory(req, view, leafErr)
       -> history.Entry{ Method/URL: resolved (view.Request) or authored (req),
                         Status/Duration/Size: view.Response,
                         Err: leafErr when no HTTP completed,
                         Request: req (authored snapshot) }
       -> Store.Record(e)
            -> storage.ScrubRequestSecrets(e.Request)   (blank auth secrets + Secret vars)
            -> stamp zero Time = now
            -> append; trim to max (drop oldest)
            -> save()  (marshal -> temp file -> rename; best-effort)
```

`Record` runs on the send worker goroutine (the `Store` is mutex-guarded), before
the `fyne.Do` that delivers the response to the UI. WebSocket / SSE sends use
other paths and are not recorded.

## Viewing / restoring / resending

```
Help -> History  (internal/ui/history.go: showHistory)
  -> entries := Session.History().Entries()   (newest-first)
  -> widget.NewList of historySummary(entry)  (method, status, URL, relative time)
  -> Restore : m.openScratchWith(entry.Request)         (open in a new scratch tab)
     Resend  : m.openScratchWith(entry.Request); m.send()
     Clear   : confirm -> Session.History().Clear()
```

A restored request carries no secret (scrubbed at record time), so a resend
re-resolves credentials from the active environment / `{{variables}}`.

## Persistence lifecycle

- **Load** — `New` reads `history.yml` best-effort: a missing, unreadable, or
  malformed file yields an empty history; an over-cap file is trimmed to the
  newest `max`.
- **Save** — every `Record` / `Clear` rewrites the file atomically (stage +
  rename). A write failure is swallowed; the in-memory history is unaffected.
- **In-memory mode** — a blank `path` (headless run, tests) skips all disk I/O.
