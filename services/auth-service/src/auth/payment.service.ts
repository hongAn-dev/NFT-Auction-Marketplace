import { Injectable, BadRequestException, NotFoundException, UnauthorizedException } from '@nestjs/common';
import { PrismaService } from '../prisma/prisma.service';

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
      take: 10,
    });
    return { success: true, data: transactions };
  }
}
