# syntax=docker/dockerfile:1.7
#
# Next.js standalone build on a pinned Node LTS image; runtime on distroless nodejs as nonroot.

ARG NODE_IMAGE=node:24.21.0-trixie-slim@sha256:8ec5d7557396cfe32d21c3f9c13072355ceab22b584578ca4bb28af31120cffe
ARG RUNTIME_IMAGE=gcr.io/distroless/nodejs24-debian13:nonroot@sha256:bb6b03d81066993293a10feda7250e8e1cc034035fe9b61cfceededa7c8bf04d

FROM ${NODE_IMAGE} AS deps
WORKDIR /app
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0
RUN corepack enable pnpm
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store pnpm install --frozen-lockfile

# ---- development: hot reload (docker compose --profile dev) --------------------------------
FROM deps AS dev
ENV NEXT_TELEMETRY_DISABLED=1
# Source is bind-mounted at /app with node_modules kept in a named volume.
CMD ["pnpm", "dev", "--hostname", "0.0.0.0"]

# ---- build --------------------------------------------------------------------------------
FROM deps AS build
ENV NEXT_TELEMETRY_DISABLED=1
COPY . .
RUN pnpm build

# ---- runtime ------------------------------------------------------------------------------
FROM ${RUNTIME_IMAGE} AS runtime
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 PORT=3000 HOSTNAME=0.0.0.0
COPY --from=build --chown=nonroot:nonroot /app/.next/standalone ./
USER nonroot:nonroot
EXPOSE 3000
CMD ["server.js"]
