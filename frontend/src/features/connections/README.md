# features/connections

The connections screen: saved connections on the left, the connection form on the right.

## Files

| File | Contents |
|---|---|
| `ConnectionsPage.tsx` | Loads saved connections (`api.listSaved`), selection, new / saved / deleted handling |
| `ConnectionForm.tsx` | Form fields per engine; Test connection (cancellable, shows the detected server such as "MariaDB 10.11.6"), Load list, Save, Delete (click twice) |
| `FileField.tsx` | Read-only path plus dialog buttons; the only way to fill a file path |
| `SecretField.tsx` | Password-like input that never shows the stored value |
| `K8sSection.tsx` | Kubernetes route: kubeconfig, context, namespace, target (CloudNativePG / service / pod) with lists loaded from the cluster |
| `model.ts` | `FormState`, defaults, labels, `cleanConfig`, `toDraft` |

## Passwords

The frontend never receives a saved secret (DB password, SSH password, SSH key passphrase). A saved connection opens with empty secret fields and `dirtySecrets: []`; typing into a field adds its name (`SECRET` in `model.ts`, mirroring `store.Secret*`).

| User action on one secret field | Effect |
|---|---|
| Leaves it alone | Backend uses the stored value (also for Test / Load list) |
| Types a new value | Replaces only that secret on Save |
| Types, then empties it | Save deletes that secret; "Keep saved" undoes it |

New connections send every secret. Switching SSH auth method marks the secrets the new method doesn't use as dirty and empty, so they are deleted on save.

## Engine-specific fields

`cleanConfig` drops fields that don't apply to the selected engine before anything is sent, so a value left over from another type can't fail validation:

| Engine | Extra fields shown |
|---|---|
| MySQL | CA certificate, client certificate and key |
| PostgreSQL | SSL mode; CA certificate, client certificate and key |
| SQL Server | Trust server certificate; CA certificate |
| SQLite | Database file (Open… / New…) instead of host, port, user, password; no TLS or SSH |

Form order for server engines: route (Direct / SSH tunnel / Kubernetes) → transport (TCP / Unix socket) → host and port or socket path → user and password → database → TLS. You first say how to reach the server, then where the database is.

Switching routes (`route`) keeps the other routes' fields in the form; only `toDraft` drops them, so switching back restores what was typed.

**Kubernetes**: pick a kubeconfig (optional), context, namespace and a target. With **CloudNativePG** (PostgreSQL only) the host, user, password, SSL mode and TLS fields are hidden: they come from the cluster's secrets at connect time and nothing is stored. With **Service** or **Pod** you give the port and type the database user and password as usual. "List" and "Find in namespace" fill the suggestions; everything can also be typed, since RBAC often forbids listing.

Server engines can connect through an SSH tunnel: host, port (22), user, auth method (password / private key + passphrase / agent) and an optional known_hosts file. Turning the tunnel on fills an empty database host with `127.0.0.1` (the SSH server itself); the host is entered as seen from the SSH server.

For MySQL and PostgreSQL the tunnel can reach the database through a **Unix socket** instead of TCP (`viaSocket`). Host, port and TLS are then hidden, and an empty database user defaults to the SSH user, since socket auth matches the OS account name. This is how root on a Debian/Ubuntu MariaDB logs in without a password.

The socket path can stay empty: Test connection, Load list and Save first call `api.detectSocket`, which tries the usual locations for that engine on the SSH server, and fill in what it found. "Detect" does only that step.
