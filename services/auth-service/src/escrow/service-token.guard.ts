import { CanActivate, ExecutionContext, Injectable } from '@nestjs/common';
import { timingSafeEqual } from 'crypto';

// ServiceTokenGuard: endpoint ký quỹ chỉ dành cho dịch vụ nội bộ (bidding-service),
// không đi qua api-gateway. Header x-service-token phải trùng SERVICE_TOKEN; thiếu
// biến môi trường thì chặn tất cả (fail closed).
@Injectable()
export class ServiceTokenGuard implements CanActivate {
  canActivate(context: ExecutionContext): boolean {
    const want = process.env.SERVICE_TOKEN || '';
    const got = String(context.switchToHttp().getRequest().headers['x-service-token'] || '');
    if (!want || got.length !== want.length) return false;
    return timingSafeEqual(Buffer.from(got), Buffer.from(want));
  }
}
