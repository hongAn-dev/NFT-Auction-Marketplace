# Stage 1: Build ứng dụng NestJS Auth Service
FROM node:20-bullseye-slim AS builder
WORKDIR /app
COPY package.json tsconfig.json nest-cli.json ./
COPY prisma ./prisma/
RUN npm install
RUN npx prisma generate
COPY . .
RUN npm run build

# Stage 2: Runtime image gọn nhẹ chạy NodeJS
FROM node:20-bullseye-slim
WORKDIR /app
COPY package.json ./
COPY prisma ./prisma/
RUN npm install --only=production
RUN npx prisma generate
COPY --from=builder /app/dist ./dist
EXPOSE 4001
ENV PORT=4001
CMD ["node", "dist/main"]
