import { Injectable, UnauthorizedException } from '@nestjs/common';
import { PassportStrategy } from '@nestjs/passport';
import { ExtractJwt, Strategy } from 'passport-jwt';
import { RedisService } from '../redis/redis.service';

@Injectable()
export class JwtStrategy extends PassportStrategy(Strategy) {
  constructor(private readonly redis: RedisService) {
    super({
      jwtFromRequest: ExtractJwt.fromAuthHeaderAsBearerToken(),
      ignoreExpiration: false,
      secretOrKey: process.env.JWT_SECRET || 'your-super-secret-key-change-this',
      passReqToCallback: true,
    });
  }

  async validate(req: any, payload: any) {
    const rawToken = ExtractJwt.fromAuthHeaderAsBearerToken()(req);
    
    // Check if token is blacklisted in Redis
    if (rawToken) {
      const isBlacklisted = await this.redis.exists(`auth:blacklist:${rawToken}`);
      if (isBlacklisted) {
        throw new UnauthorizedException('Token đã bị vô hiệu hóa (Đăng xuất)');
      }
    }

    return { id: payload.sub, email: payload.email, role: payload.role };
  }
}
