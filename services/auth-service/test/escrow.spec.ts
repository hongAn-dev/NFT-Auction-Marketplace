import { PrismaClient } from '@prisma/client';
import { EscrowService, EscrowError } from '../src/escrow/escrow.service';

// Chạy trên Postgres thật (docs/24b, Task 1): điều kiện "không giữ quá số dư" là
// một câu UPDATE có điều kiện, chỉ kiểm được trên DB thật.
const prisma = new PrismaClient();
const escrow = new EscrowService(prisma as any);

async function user(balance: number) {
  return prisma.user.create({
    data: { email: `u${Math.random()}@test`, password: 'x', balance },
  });
}
async function money(id: string) {
  const u = await prisma.user.findUniqueOrThrow({ where: { id } });
  return { balance: Number(u.balance), held: Number(u.held) };
}
async function code(p: Promise<unknown>) {
  try {
    await p;
    return 'OK';
  } catch (e) {
    return e instanceof EscrowError ? e.code : String(e);
  }
}

afterAll(() => prisma.$disconnect());

describe('EscrowService', () => {
  it('hold chuyển tiền từ balance sang held; thiếu tiền thì từ chối', async () => {
    const u = await user(1_000_000);
    await escrow.hold('h-1', u.id, 600_000);
    expect(await money(u.id)).toEqual({ balance: 400_000, held: 600_000 });
    expect(await code(escrow.hold('h-2', u.id, 500_000))).toBe('INSUFFICIENT_FUNDS');
    expect(await money(u.id)).toEqual({ balance: 400_000, held: 600_000 });
    expect(await code(escrow.hold('h-3', 'khong-co', 1))).toBe('USER_NOT_FOUND');
    expect(await code(escrow.hold('h-4', u.id, 0))).toBe('BAD_AMOUNT');
    expect(await code(escrow.hold('h-5', u.id, 1.5))).toBe('BAD_AMOUNT');
  });

  it('gọi lại hold cùng id là idempotent; khác số tiền là xung đột', async () => {
    const u = await user(1_000_000);
    await escrow.hold('h-idem', u.id, 300_000);
    const again = await escrow.hold('h-idem', u.id, 300_000);
    expect(again.status).toBe('HELD');
    expect(await money(u.id)).toEqual({ balance: 700_000, held: 300_000 });
    expect(await code(escrow.hold('h-idem', u.id, 200_000))).toBe('CONFLICT');
  });

  it('hai hold song song cùng id chỉ trừ tiền một lần', async () => {
    const u = await user(1_000_000);
    await Promise.allSettled([escrow.hold('h-race', u.id, 400_000), escrow.hold('h-race', u.id, 400_000)]);
    expect(await money(u.id)).toEqual({ balance: 600_000, held: 400_000 });
  });

  it('release / refund trả held về balance; capture trả cho người bán', async () => {
    const buyer = await user(1_000_000);
    const seller = await user(0);
    await escrow.hold('h-r', buyer.id, 100_000);
    await escrow.hold('h-f', buyer.id, 200_000);
    await escrow.hold('h-c', buyer.id, 300_000);
    await escrow.release('h-r');
    await escrow.refund('h-f');
    await escrow.capture('h-c', seller.id);
    expect(await money(buyer.id)).toEqual({ balance: 700_000, held: 0 });
    expect(await money(seller.id)).toEqual({ balance: 300_000, held: 0 });
  });

  it('thao tác lặp lại thì idempotent, mâu thuẫn thì CONFLICT', async () => {
    const buyer = await user(1_000_000);
    const seller = await user(0);
    await escrow.hold('h-x', buyer.id, 100_000);
    await escrow.capture('h-x', seller.id);
    await escrow.capture('h-x', seller.id);
    expect(await money(seller.id)).toEqual({ balance: 100_000, held: 0 });
    expect(await code(escrow.refund('h-x'))).toBe('CONFLICT'); // không bao giờ vừa trả vừa hoàn
    expect(await code(escrow.release('h-x'))).toBe('CONFLICT');
    expect(await code(escrow.capture('h-x', buyer.id))).toBe('CONFLICT');
    expect(await code(escrow.release('khong-co'))).toBe('NOT_FOUND');
  });

  it('tổng balance + held được bảo toàn qua hold / release / refund', async () => {
    const u = await user(900_000);
    await escrow.hold('h-s1', u.id, 100_000);
    await escrow.hold('h-s2', u.id, 200_000);
    await escrow.release('h-s1');
    await escrow.refund('h-s2');
    const m = await money(u.id);
    expect(m.balance + m.held).toBe(900_000);
  });

  it('staleHeld liệt kê khoản HELD cũ hơn ngưỡng', async () => {
    const u = await user(100_000);
    await escrow.hold('h-old', u.id, 10_000);
    await prisma.escrowHold.update({ where: { id: 'h-old' }, data: { createdAt: new Date(Date.now() - 600_000) } });
    await escrow.hold('h-new', u.id, 10_000);
    const ids = (await escrow.staleHeld(300)).map((h) => h.id);
    expect(ids).toContain('h-old');
    expect(ids).not.toContain('h-new');
  });
});
