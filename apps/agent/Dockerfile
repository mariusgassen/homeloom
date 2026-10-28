# Slim (glibc), not Alpine: LiveKit's WebRTC bindings and the voice activity model are glibc builds.
FROM node:26-slim AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build && npm prune --omit=dev

FROM node:26-slim
WORKDIR /app
ENV NODE_ENV=production
COPY --from=build /src/node_modules node_modules
COPY --from=build /src/dist dist
COPY package.json ./
USER node
EXPOSE 8081
HEALTHCHECK --interval=30s --timeout=3s CMD node -e "fetch('http://localhost:8081/').then(r => process.exit(r.ok ? 0 : 1), () => process.exit(1))"
CMD ["node", "dist/agent.js", "start"]
