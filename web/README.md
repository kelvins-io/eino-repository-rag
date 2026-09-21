# Eino RAG Web

[English](README.en.md) | [中文](README.md)

Vue 3 + Element Plus 管理台，对接后端 `/api/v1`。登录后以 JWT 调用知识库、文档、问答与租户管理接口。界面支持中英文切换。

仓库总览见 [../README.md](../README.md)。

## 开发

先启动后端 `:8080`，再在本目录安装并运行：

```bash
npm install
npm run dev
```

或在仓库根目录：

```bash
make web-install && make web
```

默认 http://localhost:5173 。开发模式下 Vite 把 `/api`、`/health` 代理到 `http://localhost:8080`，`.env.development` 中 `VITE_API_BASE` 为空（同源走代理）。

首次使用：在登录页把租户 ID 改为 `default`，用后端日志里该租户 `admin` 的初始密码登录。`default` 不允许自助注册，其他租户可在 `/register` 创建账号。

登录页和注册页会读取地址栏 `?tenant_id=` 并填入租户 ID；没有该参数或值为空时默认填 `guest`。两个页面互相跳转时会带上当前输入的租户 ID；登录页仍是 `default` 时不会带到注册页，注册页回落到 `guest`。

## 构建

```bash
npm run build      # 产出 dist/
npm run preview    # 预览生产构建
```

`.env.production` 默认 `VITE_API_BASE=http://localhost:8080`，浏览器直连后端（依赖 CORS）。Docker 镜像构建时会把 `VITE_API_BASE` 置空，由 nginx 同源反代 `/api`、`/health`，不必改该文件。

仓库根目录：

```bash
make web-build
# 或随后端一起：make docker-up
```

Docker 前端：http://localhost:5173 。

## 页面

未登录访问受保护路由会跳到 `/login`；`default` / `admin` 以外的账号看不到租户管理页。

| 路径 | 说明 |
|------|------|
| `/login` | 租户 ID + 用户名登录；`?tenant_id=` 预填，缺省 `guest` |
| `/register` | 在已有非 `default` 租户下注册并登录；`?tenant_id=` 预填，缺省 `guest` |
| `/knowledge-bases` | 知识库列表与 CRUD |
| `/knowledge-bases/:id` | 目录树、文档导入/列表/删除、重新索引、分块、索引记录、召回率与引用 |
| `/chat` | 知识问答：标准 RAG / Agent、会话记忆、目录过滤、语音输入与朗读、点赞评分、相关文档标注 |
| `/users` | 本租户用户列表；租户 `admin` 可开关他人登录 |
| `/tenant-manage` | 平台管理员：租户配额与当日用量 |
| `/tenants` | 平台管理员：创建租户 |

问答走 SSE（`/chat/query`、`/chat/agent`）。导入文件数、会话数、语音与 TTS 受租户配额限制，达上限时对应按钮会禁用。索引成功后源文件会被清理，文档列表里 `source_available=false` 的条目会禁用重新索引。

## 语言

登录页、注册页和顶栏均可切换 **中文 / English**。选择写入 `localStorage`（`eino_rag_locale`），刷新后保持；首次访问按浏览器语言猜测，默认中文。文案走 `vue-i18n`（`src/i18n/locales/`），Element Plus 组件语言随当前 locale 一起切换，页面标题也会更新。

## 鉴权

Token 与当前用户/租户写入 `localStorage`（`eino_rag_token` 等）。请求自动带 `Authorization: Bearer <token>`。401 或账号被禁止登录（403）时清会话并回到登录页。

## 目录结构

```
web/
  src/
    api/           # Axios + SSE 封装
    components/    # 语言切换等公共组件
    composables/   # 语音输入 / 朗读
    i18n/          # vue-i18n 与 zh-CN / en 文案
    layouts/       # 侧栏布局
    views/         # 页面
    router/        # 路由与登录守卫
    utils/         # JWT 本地存储、平台/租户管理员判断
  Dockerfile       # 多阶段：Vite build + nginx
  nginx.conf       # 静态资源 + 反代 /api、/health
  vite.config.js   # 开发代理 :8080
```

## 技术栈

Vue 3、Vite、Element Plus、Vue Router、Vue I18n、Axios。
