import { Injectable } from '@nestjs/common';
import { Prisma } from '@prisma/client';
import { PrismaService } from '../prisma/prisma.service';

// Ký quỹ của sàn (docs/23 §4). Mọi số tiền là VND nguyên. Mỗi khoản giữ mang id
// do bidding-service cấp (= bidId) và id đó là khoá idempotency: gọi lại đúng thao
// tác đã làm trả kết quả cũ, thao tác mâu thuẫn trả CONFLICT. Nhờ vậy một khoản
// không bao giờ vừa được trả cho người bán vừa được hoàn cho người mua.

export type EscrowStatus = 'HELD' | 'RELEASED' | 'REFUNDED' | 'CAPTURED';

export type EscrowErrorCode =
  | 'BAD_AMOUNT'
  | 'USER_NOT_FOUND'
  | 'INSUFFICIENT_FUNDS'
  | 'NOT_FOUND'
  | 'CONFLICT';

export class EscrowError extends Error {
  constructor(
    readonly code: EscrowErrorCode,
    message: string,
  ) {
    super(message);
  }
}

export interface HoldView {
  id: string;
  userId: string;
  amount: number;
  status: EscrowStatus;
  counterpartyId: string | null;
  createdAt: Date;
}

type Tx = Prisma.TransactionClient;

function view(h: {
  id: string;
  userId: string;
  amount: Prisma.Decimal;
  status: string;
  counterpartyId: string | null;
  createdAt: Date;
}): HoldView {
  return {
    id: h.id,
    userId: h.userId,
    amount: Number(h.amount),
    status: h.status as EscrowStatus,
    counterpartyId: h.counterpartyId,
    createdAt: h.createdAt,
  };
}

@Injectable()
export class EscrowService {
  constructor(private readonly prisma: PrismaService) {}

  // hold: balance → held. Câu UPDATE có điều kiện balance >= amount nên hai lượt
  // đặt giá song song không thể cùng tiêu một đồng.
  async hold(id: string, userId: string, amount: number): Promise<HoldView> {
    if (!Number.isSafeInteger(amount) || amount <= 0) {
      throw new EscrowError('BAD_AMOUNT', `số tiền giữ phải là số nguyên VND dương, nhận ${amount}`);
    }
    try {
      return await this.prisma.$transaction(async (tx) => {
        const existing = await tx.escrowHold.findUnique({ where: { id } });
        if (existing) {
          return this.sameHold(existing, userId, amount);
        }
        const moved = await tx.$executeRaw`
          UPDATE users SET balance = balance - ${amount}, held = held + ${amount}
          WHERE id = ${userId} AND balance >= ${amount}`;
        if (moved === 0) {
          const u = await tx.user.findUnique({ where: { id: userId }, select: { id: true } });
          throw u
            ? new EscrowError('INSUFFICIENT_FUNDS', 'số dư khả dụng không đủ để giữ ký quỹ')
            : new EscrowError('USER_NOT_FOUND', `không có người dùng ${userId}`);
        }
        return view(await tx.escrowHold.create({ data: { id, userId, amount, status: 'HELD' } }));
      });
    } catch (e) {
      // Hai lần gọi song song cùng id: lần thua đụng khoá chính, transaction của nó
      // đã rollback (không trừ tiền). Trả về khoản của lần thắng.
      if (e instanceof Prisma.PrismaClientKnownRequestError && e.code === 'P2002') {
        const won = await this.prisma.escrowHold.findUnique({ where: { id } });
        if (won) return this.sameHold(won, userId, amount);
      }
      throw e;
    }
  }

  private sameHold(h: Parameters<typeof view>[0], userId: string, amount: number): HoldView {
    if (h.userId !== userId || Number(h.amount) !== amount) {
      throw new EscrowError('CONFLICT', `khoản giữ ${h.id} đã có với người dùng / số tiền khác`);
    }
    return view(h);
  }

  // release: người bị vượt giá, hoặc lượt đặt giá ghi DB hỏng.
  release(id: string): Promise<HoldView> {
    return this.settle(id, 'RELEASED', null);
  }

  // refund: vụ bán hỏng (FAILED / CANCELLED trên chuỗi).
  refund(id: string): Promise<HoldView> {
    return this.settle(id, 'REFUNDED', null);
  }

  // capture: vụ bán COMPLETED — tiền giữ của người mua sang balance người bán.
  capture(id: string, sellerId: string): Promise<HoldView> {
    if (!sellerId) {
      return Promise.reject(new EscrowError('BAD_AMOUNT', 'capture cần sellerId'));
    }
    return this.settle(id, 'CAPTURED', sellerId);
  }

  private settle(id: string, to: Exclude<EscrowStatus, 'HELD'>, sellerId: string | null): Promise<HoldView> {
    return this.prisma.$transaction(async (tx) => {
      const h = await this.lockHold(tx, id);
      if (h.status === to && (to !== 'CAPTURED' || h.counterpartyId === sellerId)) {
        return view(h);
      }
      if (h.status !== 'HELD') {
        throw new EscrowError('CONFLICT', `khoản giữ ${id} đang ${h.status}, không chuyển sang ${to} được`);
      }
      const amount = Number(h.amount);
      if (to === 'CAPTURED') {
        await tx.$executeRaw`UPDATE users SET held = held - ${amount} WHERE id = ${h.userId}`;
        const paid = await tx.$executeRaw`UPDATE users SET balance = balance + ${amount} WHERE id = ${sellerId}`;
        if (paid === 0) throw new EscrowError('USER_NOT_FOUND', `không có người bán ${sellerId}`);
      } else {
        await tx.$executeRaw`UPDATE users SET held = held - ${amount}, balance = balance + ${amount} WHERE id = ${h.userId}`;
      }
      return view(
        await tx.escrowHold.update({ where: { id }, data: { status: to, counterpartyId: sellerId } }),
      );
    });
  }

  // lockHold: SELECT … FOR UPDATE — hai lệnh cùng khoản chạy lần lượt.
  private async lockHold(tx: Tx, id: string) {
    const rows = await tx.$queryRaw<{ id: string }[]>`SELECT id FROM escrow_holds WHERE id = ${id} FOR UPDATE`;
    if (rows.length === 0) throw new EscrowError('NOT_FOUND', `không có khoản giữ ${id}`);
    return tx.escrowHold.findUniqueOrThrow({ where: { id } });
  }

  // staleHeld: khoản HELD cũ hơn olderThanSec — job đối soát của bidding-service
  // nhả các khoản không có lượt đặt giá nào đi kèm (docs/23 §5.2 bước 4).
  async staleHeld(olderThanSec: number): Promise<HoldView[]> {
    const before = new Date(Date.now() - olderThanSec * 1000);
    const rows = await this.prisma.escrowHold.findMany({
      where: { status: 'HELD', createdAt: { lt: before } },
      orderBy: { createdAt: 'asc' },
      take: 500,
    });
    return rows.map(view);
  }
}
