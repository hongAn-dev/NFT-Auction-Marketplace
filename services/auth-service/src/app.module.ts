import { Module } from '@nestjs/common';
import { PrismaModule } from './prisma/prisma.module';
import { RedisModule } from './redis/redis.module';
import { AuthModule } from './auth/auth.module';
import { EscrowModule } from './escrow/escrow.module';

@Module({
  imports: [PrismaModule, RedisModule, AuthModule, EscrowModule],
})
export class AppModule {}
