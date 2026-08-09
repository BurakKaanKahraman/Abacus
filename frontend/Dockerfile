# syntax=docker/dockerfile:1

# --- Build stage -------------------------------------------------------------
FROM node:22-alpine AS build

WORKDIR /app

# npm ci needs only the manifests, so dependency installation is cached until
# one of them actually changes.
COPY package.json package-lock.json ./
RUN npm ci

COPY . .

# Vite inlines VITE_ variables at build time, so the API location is decided
# here rather than at runtime. It defaults to a same-origin path because nginx
# proxies /api/v1 to the backend, which keeps the browser from ever making a
# cross-origin request.
ARG VITE_API_BASE_URL=/api/v1
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}

# The preview mode the app starts in, and how long typing must pause before a
# server preview is requested. Both are defaults only: the switch in the header
# overrides the mode at run time and the choice is remembered per browser.
ARG VITE_PREVIEW_MODE=local
ARG VITE_PREVIEW_DEBOUNCE_MS=300
ENV VITE_PREVIEW_MODE=${VITE_PREVIEW_MODE} \
    VITE_PREVIEW_DEBOUNCE_MS=${VITE_PREVIEW_DEBOUNCE_MS}

RUN npm run build

# --- Test stage --------------------------------------------------------------
# Not built by default. `docker build --target test` runs lint, types and the
# suite against the exact sources that produce the image.
FROM build AS test
RUN npm run lint && npm run typecheck && npm run test:run

# --- Runtime stage -----------------------------------------------------------
FROM nginx:1.27-alpine AS runtime

COPY nginx/nginx.conf /etc/nginx/nginx.conf
COPY nginx/security-headers.conf /etc/nginx/conf.d/security-headers.conf
COPY --from=build /app/dist /usr/share/nginx/html

# The stock image runs its master process as root to bind port 80. Listening
# above 1024 instead lets the whole server run unprivileged.
RUN chown -R nginx:nginx /usr/share/nginx/html /var/cache/nginx && \
    chmod -R 755 /usr/share/nginx/html

USER nginx

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --spider --tries=1 http://127.0.0.1:8080/ || exit 1

CMD ["nginx", "-g", "daemon off;"]
