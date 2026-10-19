import { HttpException } from '@nestjs/common';
import { ServiceTokenGuard } from '../src/escrow/service-token.guard';
import { toHttp } from '../src/escrow/escrow.controller';
import { EscrowError } from '../src/escrow/escrow.service';

function ctx(headers: Record<string, string>) {
  return { switchToHttp: () => ({ getRequest: () => ({ headers }) }) } as any;
}

describe('ServiceTokenGuard', () => {
  const old = process.env.SERVICE_TOKEN;
  afterEach(() => (process.env.SERVICE_TOKEN = old));

  it('chỉ cho qua đúng token dịch vụ', () => {
    process.env.SERVICE_TOKEN = 'bi-mat-noi-bo';
    const g = new ServiceTokenGuard();
    expect(g.canActivate(ctx({ 'x-service-token': 'bi-mat-noi-bo' }))).toBe(true);
    expect(g.canActivate(ctx({ 'x-service-token': 'sai' }))).toBe(false);
    expect(g.canActivate(ctx({}))).toBe(false);
  });

  it('thiếu SERVICE_TOKEN trong môi trường thì chặn tất cả (fail closed)', () => {
    delete process.env.SERVICE_TOKEN;
    expect(new ServiceTokenGuard().canActivate(ctx({ 'x-service-token': '' }))).toBe(false);
  });
});

describe('toHttp', () => {
  it.each([
    ['BAD_AMOUNT', 400],
    ['USER_NOT_FOUND', 404],
    ['NOT_FOUND', 404],
    ['INSUFFICIENT_FUNDS', 402],
    ['CONFLICT', 409],
  ])('%s → %d, body mang mã lỗi', (code, status) => {
    const e = toHttp(new EscrowError(code as any, 'x'));
    expect(e).toBeInstanceOf(HttpException);
    expect(e.getStatus()).toBe(status);
    expect((e.getResponse() as any).code).toBe(code);
  });
});
