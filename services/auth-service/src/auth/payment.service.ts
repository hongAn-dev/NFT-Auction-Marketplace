import { Injectable, BadRequestException, NotFoundException, UnauthorizedException } from '@nestjs/common';
import { PrismaService } from '../prisma/prisma.service';
import * as bcrypt from 'bcryptjs';

@Injectable()
export class PaymentService {
  private readonly bankName = process.env.SEPAY_BANK_NAME || 'MBBank';
  private readonly accountNo = process.env.SEPAY_ACCOUNT_NO || '0999999999';
  private readonly accountName = process.env.SEPAY_ACCOUNT_NAME || 'NGUYEN VAN A';
  private readonly webhookToken = process.env.SEPAY_WEBHOOK_TOKEN || 'your-sepay-webhook-token';

  constructor(private readonly prisma: PrismaService) {}

  async createDepositLink(userId: string, amount: number) {
    if (amount < 2000) {
      throw new BadRequestException('Số tiền nạp tối thiểu là 2,000 VND');
    }

    // 1. Tìm kiếm user
    const user = await this.prisma.user.findUnique({
      where: { id: userId },
    });
    if (!user) {
      throw new NotFoundException('Không tìm thấy thông tin tài khoản');
    }

    // 2. Tạo mã đơn hàng ngẫu nhiên
    const orderCode = Math.floor(100000 + Math.random() * 900000);
    const memo = `NFT${orderCode}`;

    // 3. Tạo bản ghi giao dịch PENDING trong database
    await this.prisma.fiatTransaction.create({
      data: {
        userId,
        amount,
        gatewayRefId: memo,
        status: 'PENDING',
      },
    });

    // 4. Tạo mã QR VietQR động
    const qrUrl = `https://img.vietqr.io/image/${this.bankName}-${this.accountNo}-qr_only.png?amount=${amount}&addInfo=${memo}&accountName=${encodeURIComponent(this.accountName)}`;

    return {
      success: true,
      data: {
        qrUrl,
        bankName: this.bankName,
        accountNo: this.accountNo,
        accountName: this.accountName,
        memo,
        amount,
      },
    };
  }

  async handleWebhook(body: any, authHeader?: string) {
    try {
      // 1. Xác thực webhook token từ SePay
      if (!authHeader) {
        throw new UnauthorizedException('Thiếu mã xác thực Webhook');
      }
      
      const expectedToken = `Bearer ${this.webhookToken}`;
      if (authHeader !== expectedToken) {
        console.warn(`[SePay Webhook] Unauthorized request. Received: ${authHeader}`);
        throw new UnauthorizedException('Mã xác thực Webhook không hợp lệ');
      }

      // 2. Trích xuất thông tin giao dịch từ request body của SePay
      const content = body.transactionContent || '';
      const match = content.match(/NFT\d+/i);
      if (!match) {
        console.warn(`[SePay Webhook] No matching NFT code pattern found in: ${content}`);
        return { success: false, message: 'Nội dung chuyển khoản không hợp lệ' };
      }
      
      const memo = match[0].toUpperCase();
      const amountIn = Number(body.amountIn || 0);

      // 3. Tìm kiếm transaction PENDING trong database
      const transaction = await this.prisma.fiatTransaction.findUnique({
        where: { gatewayRefId: memo },
      });

      if (!transaction) {
        console.warn(`[SePay Webhook] Transaction not found for memo: ${memo}`);
        return { success: false, message: 'Giao dịch không tồn tại' };
      }

      if (transaction.status !== 'PENDING') {
        console.log(`[SePay Webhook] Transaction ${memo} already processed: ${transaction.status}`);
        return { success: true, message: 'Giao dịch đã được xử lý trước đó' };
      }

      // 4. Sử dụng Prisma Transaction để cập nhật balance và trạng thái giao dịch một cách nguyên tử (ACID)
      await this.prisma.$transaction(async (tx) => {
        // Cập nhật trạng thái giao dịch
        await tx.fiatTransaction.update({
          where: { id: transaction.id },
          data: { status: 'SUCCESS' },
        });

        // Tăng balance người dùng
        const user = await tx.user.findUnique({
          where: { id: transaction.userId },
        });

        if (user) {
          await tx.user.update({
            where: { id: user.id },
            data: { balance: user.balance + amountIn },
          });
          console.log(`[SePay Webhook] Successfully credited ${amountIn} VND to user ${user.email}`);
        }
      });

      return { success: true, message: 'Cập nhật số dư thành công' };
    } catch (error) {
      console.error('[SePay Webhook Error] Failed to process webhook:', error);
      if (error instanceof UnauthorizedException) {
        throw error;
      }
      throw new BadRequestException('Xử lý Webhook thất bại');
    }
  }

  async getBalance(userId: string) {
    const user = await this.prisma.user.findUnique({
      where: { id: userId },
      select: { balance: true },
    });
    if (!user) {
      throw new NotFoundException('Không tìm thấy người dùng');
    }
    return { success: true, data: { balance: user.balance } };
  }

  async getTransactions(userId: string) {
    const transactions = await this.prisma.fiatTransaction.findMany({
      where: { userId },
      orderBy: { createdAt: 'desc' },
      take: 15,
    });
    return { success: true, data: transactions };
  }

  // ── LINKED BANK ACCOUNTS METHODS ──────────────────────────────
  async linkBankAccount(userId: string, data: { bankCode: string; bankName: string; accountNo: string; accountName: string }) {
    const existing = await this.prisma.userBankAccount.findFirst({
      where: {
        userId,
        bankCode: data.bankCode,
        accountNo: data.accountNo,
      },
    });
    if (existing) {
      throw new BadRequestException('Tài khoản ngân hàng này đã được liên kết');
    }

    const linkedAccount = await this.prisma.userBankAccount.create({
      data: {
        userId,
        bankCode: data.bankCode,
        bankName: data.bankName,
        accountNo: data.accountNo,
        accountName: data.accountName,
      },
    });

    return { success: true, data: linkedAccount };
  }

  async getLinkedBankAccounts(userId: string) {
    const accounts = await this.prisma.userBankAccount.findMany({
      where: { userId },
      orderBy: { createdAt: 'desc' },
    });
    return { success: true, data: accounts };
  }

  async unlinkBankAccount(userId: string, bankAccountId: string) {
    const account = await this.prisma.userBankAccount.findFirst({
      where: { id: bankAccountId, userId },
    });
    if (!account) {
      throw new NotFoundException('Không tìm thấy tài khoản ngân hàng liên kết');
    }
    await this.prisma.userBankAccount.delete({
      where: { id: bankAccountId },
    });
    return { success: true, message: 'Đã hủy liên kết tài khoản ngân hàng thành công' };
  }

  // ── WITHDRAWALS METHODS (FREEZE FUNDS & LOG GD) ───────────────
  async withdraw(userId: string, amount: number, bankAccountId: string, passwordConfirm: string) {
    if (amount < 50000) {
      throw new BadRequestException('Số tiền rút tối thiểu là 50,000 VND');
    }

    const user = await this.prisma.user.findUnique({
      where: { id: userId },
    });
    if (!user) {
      throw new NotFoundException('Không tìm thấy người dùng');
    }

    // 1. Xác nhận mật khẩu để tăng tính bảo mật
    const isPasswordValid = await bcrypt.compare(passwordConfirm, user.password);
    if (!isPasswordValid) {
      throw new BadRequestException('Mật khẩu xác nhận không chính xác');
    }

    // 2. Kiểm tra tài khoản ngân hàng liên kết
    const bankAccount = await this.prisma.userBankAccount.findFirst({
      where: { id: bankAccountId, userId },
    });
    if (!bankAccount) {
      throw new BadRequestException('Tài khoản ngân hàng liên kết không hợp lệ');
    }

    // 3. Kiểm tra số dư khả dụng
    if (user.balance < amount) {
      throw new BadRequestException('Số dư tài khoản không đủ để thực hiện yêu cầu rút tiền');
    }

    // 4. Chạy ACID Transaction: Trừ tiền (Đóng băng) và Tạo GD PENDING
    const orderCode = Math.floor(100000 + Math.random() * 900000);
    const memo = `WDR${orderCode}`;

    const transaction = await this.prisma.$transaction(async (tx) => {
      // Khấu trừ tiền trực tiếp từ số dư (Đóng băng số tiền rút)
      await tx.user.update({
        where: { id: userId },
        data: { balance: user.balance - amount },
      });

      const bankInfoStr = JSON.stringify({
        bankCode: bankAccount.bankCode,
        bankName: bankAccount.bankName,
        accountNo: bankAccount.accountNo,
        accountName: bankAccount.accountName,
      });

      // Tạo giao dịch rút tiền PENDING
      return tx.fiatTransaction.create({
        data: {
          userId,
          amount,
          gatewayRefId: memo,
          status: 'PENDING',
          type: 'WITHDRAW',
          bankAccountInfo: bankInfoStr,
        },
      });
    });

    return {
      success: true,
      message: 'Yêu cầu rút tiền của bạn đang chờ phê duyệt từ Ban quản trị.',
      data: transaction,
    };
  }

  // ── ADMIN METHODS: APPROVE / REJECT ──────────────────────────
  async getPendingWithdrawals() {
    const withdrawals = await this.prisma.fiatTransaction.findMany({
      where: { type: 'WITHDRAW', status: 'PENDING' },
      orderBy: { createdAt: 'desc' },
    });
    return { success: true, data: withdrawals };
  }

  async approveWithdrawal(transactionId: string) {
    const transaction = await this.prisma.fiatTransaction.findUnique({
      where: { id: transactionId },
    });
    if (!transaction) {
      throw new NotFoundException('Không tìm thấy yêu cầu giao dịch');
    }
    if (transaction.type !== 'WITHDRAW' || transaction.status !== 'PENDING') {
      throw new BadRequestException('Giao dịch không hợp lệ để phê duyệt');
    }

    const updated = await this.prisma.fiatTransaction.update({
      where: { id: transactionId },
      data: { status: 'SUCCESS' },
    });

    return { success: true, message: 'Phê duyệt yêu cầu rút tiền thành công', data: updated };
  }

  async rejectWithdrawal(transactionId: string) {
    const transaction = await this.prisma.fiatTransaction.findUnique({
      where: { id: transactionId },
    });
    if (!transaction) {
      throw new NotFoundException('Không tìm thấy yêu cầu giao dịch');
    }
    if (transaction.type !== 'WITHDRAW' || transaction.status !== 'PENDING') {
      throw new BadRequestException('Giao dịch không hợp lệ để từ chối');
    }

    const updated = await this.prisma.$transaction(async (tx) => {
      // Hoàn trả lại tiền về số dư tài khoản của User
      const user = await tx.user.findUnique({
        where: { id: transaction.userId },
      });
      if (user) {
        await tx.user.update({
          where: { id: user.id },
          data: { balance: user.balance + transaction.amount },
        });
      }

      // Đổi trạng thái giao dịch thành FAILED
      return tx.fiatTransaction.update({
        where: { id: transactionId },
        data: { status: 'FAILED' },
      });
    });

    return { success: true, message: 'Đã từ chối yêu cầu rút tiền và hoàn trả số dư tài khoản.', data: updated };
  }
}
