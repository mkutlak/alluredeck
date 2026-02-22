# ============================================================
# Stage 1 — build
# ============================================================
FROM node:20-alpine AS builder

WORKDIR /app

# Install dependencies first (layer-cached unless lockfile changes)
COPY package.json package-lock.json* ./
RUN npm ci --prefer-offline

# Copy source and build
COPY . .

# Pass dummy build-time values; real values are injected at runtime via env.js
ARG VITE_API_URL=http://localhost:5050
ARG VITE_APP_TITLE="Allure Dashboard"
ENV VITE_API_URL=$VITE_API_URL
ENV VITE_APP_TITLE=$VITE_APP_TITLE

RUN npm run build

# ============================================================
# Stage 2 — serve
# ============================================================
FROM nginx:1.27-alpine AS runner

# Remove default nginx config and copy ours
RUN rm /etc/nginx/conf.d/default.conf
COPY docker/nginx.conf /etc/nginx/conf.d/allure-dashboard.conf

# Copy built assets
COPY --from=builder /app/dist /usr/share/nginx/html

# Copy entrypoint script
COPY docker/docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

EXPOSE 7474

ENTRYPOINT ["/docker-entrypoint.sh"]
