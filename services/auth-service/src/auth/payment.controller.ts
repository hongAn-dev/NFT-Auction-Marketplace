import { Controller, Post, Body, Get, UseGuards, Req, HttpCode, HttpStatus, Headers } from '@nestjs/common';
import { AuthGuard } from '@nestjs/passport';
import { PaymentService } from './payment.service';

@Controller('auth/payment')
export class PaymentController {
  constructor(private readonly paymentService: PaymentService) {}

  @Post('deposit')
  @UseGuards(AuthGuard('jwt'))
  async deposit(@Req() req: any, @Body('amount') amount: number) {
    const userId = req.user.id;
    return this.paymentService.createDepositLink(userId, amount);
  }

  @Get('balance')
  @UseGuards(AuthGuard('jwt'))
  async getBalance(@Req() req: any) {
    const userId = req.user.id;
    return this.paymentService.getBalance(userId);
  }

  @Get('transactions')
  @UseGuards(AuthGuard('jwt'))
  async getTransactions(@Req() req: any) {
    const userId = req.user.id;
    return this.paymentService.getTransactions(userId);
  }

  @Post('sepay-webhook')
  @HttpCode(HttpStatus.OK)
  async sepayWebhook(@Body() body: any, @Headers('authorization') authHeader: string) {
    return this.paymentService.handleWebhook(body, authHeader);
  }
}
