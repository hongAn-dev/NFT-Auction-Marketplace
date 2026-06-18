import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module';
import { Logger, ValidationPipe } from '@nestjs/common';

async function bootstrap() {
  const logger = new Logger('AuthBootstrap');
  const app = await NestFactory.create(AppModule);

  // Validate global inputs using class-validator
  app.useGlobalPipes(
    new ValidationPipe({
      whitelist: true,
      transform: true,
    }),
  );

  app.enableCors({
    origin: '*',
    credentials: true,
  });

  const port = process.env.PORT || 4001;
  await app.listen(port);
  logger.log(`🚀 Standalone Auth Service is running on: http://localhost:${port}`);
}
bootstrap();
