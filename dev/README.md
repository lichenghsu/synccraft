# dev

Local test databases. Source (src) and target (dst) schemas and data are **deliberately different**, to verify comparison results.

## Start

```bash
podman compose -f dev/docker-compose.yml up -d     # or docker compose (needs the compose plugin)
podman compose -f dev/docker-compose.yml down -v   # also deletes data; the next up re-imports seeds
```

Images use fully qualified names (`docker.io/library/...`) because podman refuses short names without a TTY.

`seed/<service>/*.sql` is imported automatically on the container's **first** start (`docker-entrypoint-initdb.d`). After editing seeds, run `down -v` to re-import.

## Connection details

| Service | Address | User / password | Database |
|---|---|---|---|
| mysql-src | 127.0.0.1:3306 | root / root | shop |
| mysql-dst | 127.0.0.1:3307 | root / root | shop |
| pg-src | 127.0.0.1:5432 | postgres / postgres | shop |
| pg-dst | 127.0.0.1:5433 | postgres / postgres | shop |
| mssql | 127.0.0.1:1433 | sa / Passw0rd! | (no seed) |
| ssh-bastion | 127.0.0.1:2222 | ops / ops-pass | SSH only; reaches the databases as `mysql-src:3306`, `pg-src:5432`, `mssql:1433` |

The bastion also mounts the Unix socket directories of `mysql-src` (`/run/mysql-src/mysqld.sock`) and `pg-src` (`/run/pg-src`) through shared volumes, for "Unix socket over SSH". The volumes are mounted with `:z` so SELinux lets both containers use them, and the bastion starts after the databases so the volumes get the images' directory permissions. If a database fails with "Permission denied" on its socket after changing this, remove the volumes (`podman volume rm dev_mysql-src-sock dev_pg-src-sock`) and start again.

`ssh-bastion` is built from `ssh/Dockerfile` (alpine + OpenSSH with TCP forwarding). Its host key changes on every rebuild; to use it from the app, add it with `ssh-keyscan -p 2222 127.0.0.1 >> ~/.ssh/known_hosts`. Test-only credentials.

## Expected differences in the seed data

`users` table, src → dst:

| Kind | Expected result |
|---|---|
| Schema | `MODIFY email` (varchar 255 vs 100), `ADD nickname`, `ADD created_at`, `ADD INDEX idx_email` |
| Data | `INSERT` id=3, `UPDATE` id=2 (different email), `DELETE` id=9 |

`pg-src` also has a mixed-case table `"UserLog"` to test identifier quoting; dst lacks it, so schema comparison should report "table added".
