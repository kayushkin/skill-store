# skill-store

SQLite-backed registry for Claude Code / Agent `SKILL.md` skills. Ingests
upstream GitHub repos by cloning them, walking for `SKILL.md`, and recording
each skill's metadata plus copying its sibling files into the store's data
directory.

- **Port:** `:8301`
- **Data dir:** `~/.config/skill-store/` (`skill-store.db`, `skills/<id>/`, `repos/<source>/`)
- **Systemd unit:** `skill-store.service` (user)
- **Binary:** `~/bin/skill-store`

## Seed sources

Initial registered sources (see `cmd/seed`):

| Name          | Repo                                              | Tier       | License |
|---------------|---------------------------------------------------|------------|---------|
| `anthropics`  | `anthropics/skills`                               | official   | —       |
| `antigravity` | `sickn33/antigravity-awesome-skills`              | community  | MIT     |

Add more with `POST /sources` or by extending `cmd/seed/main.go`.

## Deploy

```bash
./deploy.sh
```

Then, on first run:

```bash
go run ./cmd/seed
```

## API

```
GET    /health
GET    /sources
POST   /sources                     {name, repo_url, branch?, subpath?, trust_tier?, license?}
GET    /sources/{id}
DELETE /sources/{id}
POST   /sources/{id}/sync           → {found, inserted, updated, pruned, commit}

GET    /skills                      ?source=NAME  ?source_id=N  ?q=text  ?limit=N
GET    /skills/{id}
DELETE /skills/{id}
GET    /skills/{id}/files
GET    /skills/{id}/files/{path...}
```
