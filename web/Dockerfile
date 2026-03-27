# ── Build Stage ──────────────────────────────────────
FROM node:22-alpine AS builder

WORKDIR /app

COPY package.json bun.lock* ./
RUN npm install

COPY . .
RUN npm run build

# ── Runtime Stage ────────────────────────────────────
FROM nginx:alpine

COPY --from=builder /app/dist /usr/share/nginx/html

# SPA fallback — serve index.html for all routes
RUN echo 'server { \
    listen 80; \
    root /usr/share/nginx/html; \
    index index.html; \
    location / { try_files $uri $uri/ /index.html; } \
}' > /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
