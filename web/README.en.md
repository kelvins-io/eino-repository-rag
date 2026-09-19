# Eino RAG Web

[English](README.en.md) | [中文](README.md)

Vue 3 + Arco Design admin UI for backend `/api/v1`. After login it calls knowledge-base, document, Q&A, and tenant APIs with JWT.

Repo-level docs: [../README.en.md](../README.en.md).

## Development

Start the backend on `:8080`, then in this directory:

```bash
npm install
npm run dev
```

Or from the repo root:

```bash
make web-install && make web
```

Default URL: http://localhost:5173 . In development Vite proxies `/api` and `/health` to `http://localhost:8080`. `.env.development` sets `VITE_API_BASE` empty so the browser uses the same origin (the proxy).

First login: use the `default` tenant `admin` password printed in backend logs. `default` cannot self-register; other tenants can create accounts at `/register`.

## Build

```bash
npm run build      # writes dist/
npm run preview    # preview the production build
```

`.env.production` defaults to `VITE_API_BASE=http://localhost:8080` (browser talks to the API directly; needs CORS). The Docker image builds with `VITE_API_BASE` empty; nginx reverse-proxies `/api` and `/health` on the same origin, so you do not need to change that file.

From the repo root:

```bash
make web-build
# or with the backend: make docker-up
```

Docker UI: http://localhost:5173 .

## Pages

Unauthenticated visits to protected routes redirect to `/login`. Accounts other than `default` / `admin` do not see tenant-admin pages.

| Path | Description |
|------|-------------|
| `/login` | Login with tenant ID + username |
| `/register` | Register under an existing non-`default` tenant and sign in |
| `/knowledge-bases` | Knowledge-base list and CRUD |
| `/knowledge-bases/:id` | Directory tree, import/list/delete, reindex, chunks, index history, recall and citations |
| `/chat` | Q&A: standard RAG / Agent, session memory, directory filter, voice in/out, votes/scores, relevant-doc labels |
| `/users` | Users in this tenant; tenant `admin` can toggle others' login |
| `/tenant-manage` | Platform admin: tenant quotas and today's usage |
| `/tenants` | Platform admin: create a tenant |

Q&A uses SSE (`/chat/query`, `/chat/agent`). Import count, sessions, voice, and TTS follow tenant quotas; buttons disable when a cap is reached.

## Auth

Token and current user/tenant are stored in `localStorage` (`eino_rag_token`, etc.). Requests send `Authorization: Bearer <token>`. On 401 or login-disabled 403 the session is cleared and the UI returns to login.

## Layout

```
web/
  src/
    api/           # Axios + SSE
    composables/   # Voice input / playback
    layouts/       # Sidebar layout
    views/         # Pages
    router/        # Routes and auth guards
    utils/         # JWT storage, platform/tenant admin checks
    styles/        # Global styles
  Dockerfile       # Multi-stage: Vite build + nginx
  nginx.conf       # Static files + proxy /api, /health
  vite.config.js   # Dev proxy to :8080
```

## Stack

Vue 3, Vite, Arco Design, Vue Router, Axios.
