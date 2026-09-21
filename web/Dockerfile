FROM node:22-alpine AS builder
WORKDIR /web

COPY package.json package-lock.json ./
RUN npm ci

COPY index.html ./
COPY public/ ./public/
COPY src/ ./src/
COPY vite.config.js ./

# 空字符串 = 同源；由 nginx 反代 /api、/health 到后端
ARG VITE_API_BASE=
ENV VITE_API_BASE=$VITE_API_BASE
# 代理 / 部署路径前缀，默认 /
ARG VITE_BASE=/
ENV VITE_BASE=$VITE_BASE
RUN npm run build

FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY nginx.prefix.conf /tmp/nginx.prefix.conf
ARG VITE_BASE=/
ENV VITE_BASE=$VITE_BASE
RUN set -e; \
    b=$(printf '%s' "$VITE_BASE" | tr -d '\r' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//'); \
    [ -z "$b" ] && b="/"; \
    case "$b" in /*) ;; *) b="/$b" ;; esac; \
    b=$(printf '%s' "$b" | sed 's|/*$||'); \
    if [ -n "$b" ]; then \
      sed "s|@@PREFIX@@|$b|g" /tmp/nginx.prefix.conf > /etc/nginx/conf.d/default.conf; \
    fi; \
    rm -f /tmp/nginx.prefix.conf
COPY --from=builder /web/dist/ /usr/share/nginx/html/
EXPOSE 80
HEALTHCHECK --interval=10s --timeout=3s --retries=5 \
  CMD wget -qO- "http://127.0.0.1${VITE_BASE}" >/dev/null || exit 1
