import { Injectable, CanActivate, ExecutionContext, UnauthorizedException, Logger } from '@nestjs/common';
import * as jwt from 'jsonwebtoken';
import Redis from 'ioredis';

@Injectable()
export class JwtAuthGuard implements CanActivate {
  private readonly logger = new Logger(JwtAuthGuard.name);
  private readonly redisClient: Redis;
  private readonly jwtSecret = process.env.JWT_SECRET || 'your-super-secret-key-change-this';

  constructor() {
    const redisHost = process.env.REDIS_HOST || 'localhost';
    const redisPort = parseInt(process.env.REDIS_PORT || '6379', 10);
    
    this.redisClient = new Redis({
      host: redisHost,
      port: redisPort,
      lazyConnect: true, // Only connect when needed or during bootstrap
    });
    
    this.redisClient.connect().catch((err) => {
      this.logger.error('❌ Failed to connect to Redis inside JwtAuthGuard:', err);
    });
  }

  async canActivate(context: ExecutionContext): Promise<boolean> {
    const request = context.switchToHttp().getRequest();
    const authHeader = request.headers.authorization;

    if (!authHeader || !authHeader.startsWith('Bearer ')) {
      throw new UnauthorizedException('Không tìm thấy token xác thực hoặc sai định dạng');
    }

    const token = authHeader.split(' ')[1];

    try {
      // 1. Check if token is blacklisted in Redis (user logged out)
      const isBlacklisted = await this.redisClient.get(`auth:blacklist:${token}`);
      if (isBlacklisted) {
        throw new UnauthorizedException('Token đã hết hạn hoặc bị vô hiệu hóa (Đã đăng xuất)');
      }

      // 2. Verify JWT signature and expiration
      const decoded = jwt.verify(token, this.jwtSecret) as any;
      
      // 3. Attach user payload to the request context
      request.user = {
        id: decoded.sub,
        email: decoded.email,
        role: decoded.role,
      };

      return true;
    } catch (err) {
      if (err instanceof UnauthorizedException) {
        throw err;
      }
      this.logger.warn(`⚠️ JWT Verification Failed: ${err.message}`);
      throw new UnauthorizedException('Token xác thực không hợp lệ hoặc đã hết hạn');
    }
  }
}
