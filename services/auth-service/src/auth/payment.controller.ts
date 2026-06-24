import { Controller, Post, Body, Get, UseGuards, Req, HttpCode, HttpStatus, Headers, Param, UnauthorizedException } from '@nestjs/common';
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

  // ── LINKED BANK ACCOUNTS ENDPOINTS ────────────────────────────
  @Post('bank-accounts')
  @UseGuards(AuthGuard('jwt'))
  async linkBankAccount(
    @Req() req: any,
    @Body() body: { bankCode: string; bankName: string; accountNo: string; accountName: string },
  ) {
    const userId = req.user.id;
    return this.paymentService.linkBankAccount(userId, body);
  }

  @Get('bank-accounts')
  @UseGuards(AuthGuard('jwt'))
  async getLinkedBankAccounts(@Req() req: any) {
    const userId = req.user.id;
    return this.paymentService.getLinkedBankAccounts(userId);
  }

  @Post('bank-accounts/:id/unlink')
  @UseGuards(AuthGuard('jwt'))
  async unlinkBankAccount(@Req() req: any, @Param('id') bankAccountId: string) {
    const userId = req.user.id;
    return this.paymentService.unlinkBankAccount(userId, bankAccountId);
  }

  // ── WITHDRAWALS ENDPOINTS ──────────────────────────────────────
  @Post('withdraw')
  @UseGuards(AuthGuard('jwt'))
  async withdraw(
    @Req() req: any,
    @Body() body: { amount: number; bankAccountId: string; passwordConfirm: string },
  ) {
    const userId = req.user.id;
    return this.paymentService.withdraw(userId, body.amount, body.bankAccountId, body.passwordConfirm);
  }

  // ── ADMIN CONTROLS ─────────────────────────────────────────────
  @Get('admin/withdrawals')
  @UseGuards(AuthGuard('jwt'))
  async getPendingWithdrawals(@Req() req: any) {
    if (req.user.role !== 'admin') {
      throw new UnauthorizedException('Chỉ tài khoản admin mới có quyền thực hiện hành động này');
    }
    return this.paymentService.getPendingWithdrawals();
  }

  @Post('admin/withdrawals/:id/approve')
  @UseGuards(AuthGuard('jwt'))
  async approveWithdrawal(@Req() req: any, @Param('id') id: string) {
    if (req.user.role !== 'admin') {
      throw new UnauthorizedException('Chỉ tài khoản admin mới có quyền thực hiện hành động này');
    }
    return this.paymentService.approveWithdrawal(id);
  }

  @Post('admin/withdrawals/:id/reject')
  @UseGuards(AuthGuard('jwt'))
  async rejectWithdrawal(@Req() req: any, @Param('id') id: string) {
    if (req.user.role !== 'admin') {
      throw new UnauthorizedException('Chỉ tài khoản admin mới có quyền thực hiện hành động này');
    }
    return this.paymentService.rejectWithdrawal(id);
  }
}
