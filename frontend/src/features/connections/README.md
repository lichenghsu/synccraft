# features/connections

The connections screen: saved connections on the left, the connection form on the right.

## Files

| File | Contents |
|---|---|
| `ConnectionsPage.tsx` | Loads saved connections (`api.listSaved`), selection, new / saved / deleted handling |
| `ConnectionForm.tsx` | Form fields per engine; Test connection (cancellable), Load list, Save, Delete (click twice) |
| `FileField.tsx` | Read-only path plus dialog buttons; the only way to fill a file path |
| `model.ts` | `FormState`, defaults, labels, `cleanConfig`, `toDraft` |

## Passwords

The frontend never receives a saved password. A saved connection opens with an empty password field and `secretsDirty: false`:

| User action | `updateSecrets` sent | Effect |
|---|---|---|
| Leaves the password field alone | `false` | Backend uses the stored password (also for Test / Load list) |
| Types a new password | `true` | Replaces the stored password on Save |
| Types, then empties the field | `true` | Save clears the stored password; "Keep saved password" undoes it |

New connections always send `updateSecrets: true`.

## Engine-specific fields

`cleanConfig` drops fields that don't apply to the selected engine before anything is sent, so a value left over from another type can't fail validation:

| Engine | Extra fields shown |
|---|---|
| MySQL | none yet (TLS files and SSH pending in the backend) |
| PostgreSQL | SSL mode; CA certificate, client certificate and key |
| SQL Server | Trust server certificate; CA certificate |
| SQLite | Database file (Open… / New…) instead of host, port, user, password |

SSH tunnel fields are not in the form until the backend supports tunnels.
