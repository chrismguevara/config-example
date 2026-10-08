# Dev-server image (Vite with HMR). Build from the repository root so the
# shared ../schemas directory is in the build context.
FROM node:22-alpine
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./frontend/
RUN cd frontend && npm ci --no-audit --no-fund
COPY schemas ./schemas
COPY frontend ./frontend
WORKDIR /app/frontend
RUN npm run gen:types
EXPOSE 5173
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
