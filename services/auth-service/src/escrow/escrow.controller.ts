import { Body, Controller, Get, HttpException, Param, Post, Query, UseGuards } from '@nestjs/common';
import { EscrowError, EscrowErrorCode, EscrowService, HoldView } from './escrow.service';
import { ServiceTokenGuard } from './service-token.guard';

const STATUS: Record<EscrowErrorCode, number> = {
  BAD_AMOUNT: 400,
  USER_NOT_FOUND: 404,
  NOT_FOUND: 404,
  INSUFFICIENT_FUNDS: 402,
  CONFLICT: 409,
};

// toHttp: mã lỗi ký quỹ → HTTP; body mang `code` để bidding-service phân loại.
export function toHttp(e: EscrowError): HttpException {
  return new HttpException({ code: e.code, message: e.message }, STATUS[e.code]);
}

async function run(p: Promise<HoldView | HoldView[]>) {
  try {
    return { success: true, data: await p };
  } catch (e) {
    if (e instanceof EscrowError) throw toHttp(e);
    throw e;
  }
}

@Controller('internal/escrow')
@UseGuards(ServiceTokenGuard)
export class EscrowController {
  constructor(private readonly escrow: EscrowService) {}

  @Post('holds')
  hold(@Body() b: { id: string; userId: string; amount: number }) {
    return run(this.escrow.hold(String(b.id || ''), String(b.userId || ''), Number(b.amount)));
  }

  @Post('holds/:id/release')
  release(@Param('id') id: string) {
    return run(this.escrow.release(id));
  }

  @Post('holds/:id/refund')
  refund(@Param('id') id: string) {
    return run(this.escrow.refund(id));
  }

  @Post('holds/:id/capture')
  capture(@Param('id') id: string, @Body() b: { sellerId: string }) {
    return run(this.escrow.capture(id, String(b.sellerId || '')));
  }

  @Get('holds/stale')
  stale(@Query('olderThanSec') olderThanSec: string) {
    return run(this.escrow.staleHeld(Math.max(60, Number(olderThanSec) || 300)));
  }
}
