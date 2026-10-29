# Slim (glibc), not Alpine: LiveKit's WebRTC bindings and the voice activity model are glibc builds.
FROM node:26-slim AS build
WORKDIR /src
# ONNX Runtime runs on the CPU here; skip its CUDA libraries (hundreds of MB).
ENV ONNXRUNTIME_NODE_INSTALL=skip
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
# DTLN noise cancellation models (MIT), pinned to a commit of the authors' repository.
ADD --chmod=644 https://github.com/breizhn/DTLN/raw/1de1f15a8b5b7e1c44905618ff2ef70ca8277fbc/pretrained_model/model_1.onnx \
  https://github.com/breizhn/DTLN/raw/1de1f15a8b5b7e1c44905618ff2ef70ca8277fbc/pretrained_model/model_2.onnx models/
USER node
EXPOSE 8081
HEALTHCHECK --interval=30s --timeout=3s CMD node -e "fetch('http://localhost:8081/').then(r => process.exit(r.ok ? 0 : 1), () => process.exit(1))"
CMD ["node", "dist/agent.js", "start"]
