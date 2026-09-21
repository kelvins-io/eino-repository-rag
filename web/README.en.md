# Eino RAG Web

[English](README.en.md) | [中文](README.md)

Vue 3 + Element Plus admin UI for backend `/api/v1`. After login it calls knowledge-base, document, Q&A, and tenant APIs with JWT. The UI can switch between Chinese and English.

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

First login: change the tenant ID to `default` and use that tenant's `admin` password printed in backend logs. `default` cannot self-register; other tenants can create accounts at `/register`.

Login and register read `?tenant_id=` and fill the tenant ID field. If the parameter is missing or blank, the field defaults to `guest`. Links between the two pages keep the current value; leaving login while it is still `default` does not carry that value, so register falls back to `guest`.

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
| `/login` | Login with tenant ID + username; `?tenant_id=` prefills, otherwise `guest` |
| `/register` | Register under an existing non-`default` tenant and sign in; `?tenant_id=` prefills, otherwise `guest` |
| `/knowledge-bases` | Knowledge-base list and CRUD |
| `/knowledge-bases/:id` | Directory tree, import/list/delete, reindex, chunks, index history, recall and citations |
| `/chat` | Q&A: standard RAG / Agent, session memory, directory filter, voice in/out, votes/scores, relevant-doc labels |
| `/users` | Users in this tenant; tenant `admin` can toggle others' login |
| `/tenant-manage` | Platform admin: tenant quotas and today's usage |
| `/tenants` | Platform admin: create a tenant |

Q&A uses SSE (`/chat/query`, `/chat/agent`). Import count, sessions, voice, and TTS follow tenant quotas; buttons disable when a cap is reached. After a successful index the source file is removed, so rows with `source_available=false` cannot be reindexed.

## Language

Login, register, and the top bar can switch **中文 / English**. The choice is stored in `localStorage` (`eino_rag_locale`) and kept across reloads. First visit guesses from the browser language and defaults to Chinese. Copy lives in `vue-i18n` (`src/i18n/locales/`). Element Plus locale and the document title follow the current language.

## Auth

Token and current user/tenant are stored in `localStorage` (`eino_rag_token`, etc.). Requests send `Authorization: Bearer <token>`. On 401 or login-disabled 403 the session is cleared and the UI returns to login.

## Layout

```
web/
  src/
    api/           # Axios + SSE
    components/    # Shared widgets (language switch)
    composables/   # Voice input / playback
    i18n/          # vue-i18n and zh-CN / en messages
    layouts/       # Sidebar layout
    views/         # Pages
    router/        # Routes and auth guards
    utils/         # JWT storage, platform/tenant admin checks
  Dockerfile       # Multi-stage: Vite build + nginx
  nginx.conf       # Static files + proxy /api, /health
  vite.config.js   # Dev proxy to :8080
```

## Stack

Vue 3, Vite, Element Plus, Vue Router, Vue I18n, Axios.
