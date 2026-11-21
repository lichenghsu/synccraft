# internal/store

Saved connections. Settings go to a JSON file, secrets to the OS keychain. Does not import Wails.

## Files

| File | Contents |
|---|---|
| `store.go` | `Store` (`List`, `Get`, `Save`, `Delete`), file format, atomic writes |
| `secrets.go` | `Secrets` interface, `Keyring` (OS keychain via `zalando/go-keyring`), secret field helpers |

## Where things are stored

| What | Where |
|---|---|
| Settings | `os.UserConfigDir()/synccraft/connections.json` (`~/.config` on Linux, `~/Library/Application Support` on macOS, `%AppData%` on Windows); dir `0700`, file `0600` |
| Secrets | OS keychain, service `synccraft`, key `<connection id>/<name>` |

Secret names: `password`, `ssh-password`, `ssh-passphrase`. These fields are always empty in the JSON file and in anything `List` or `Save` returns. Only `Get` (backend use) fills them in.

## File format

```json
{
  "version": 1,
  "connections": [
    { "id": "<32 hex chars>", "name": "prod", "config": { "type": "postgresql", "host": "...", "password": "" } }
  ]
}
```

A file that doesn't parse or has an unknown `version` makes every call fail. It is **never overwritten**, so a newer app version's file or a hand-edited file is not lost.

## Saving

- Empty `ID` creates a connection with a random ID; otherwise the ID must exist.
- Secrets are written when creating, or when `updateSecrets` is true. With `false` the stored secrets are kept, so the frontend can edit a connection without ever holding its password. With `true`, an empty value deletes the secret (e.g. local root without password).
- Removing the SSH tunnel deletes its secrets.
- Writes are atomic: temp file in the same directory, `fsync`, `rename`.

## Tests

`memSecrets` replaces the keychain in unit tests; `TestKeyringAdapter` uses `keyring.MockInit()`. The real keychain needs a running Secret Service on Linux (GNOME Keyring, KeePassXC, …); without one, saving fails with a keychain error.
