# Stage 1: Build ứng dụng NestJS
FROM node:20-alpine AS builder
WORKDIR /app
COPY package.json tsconfig.json nest-cli.json ./
RUN npm install
COPY . .
RUN npm run build

# Stage 2: Runtime image gọn nhẹ chạy NodeJS
FROM node:20-alpine
WORKDIR /app
COPY package.json ./
RUN npm install --only=production
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/proto ./proto
EXPOSE 4000
ENV PORT=4000
CMD ["node", "dist/main"]
