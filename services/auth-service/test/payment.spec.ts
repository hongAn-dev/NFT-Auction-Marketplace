import { PrismaClient } from '@prisma/client';
import * as bcrypt from 'bcryptjs';
import { PaymentService } from '../src/auth/payment.service';

const prisma = new PrismaClient();
const payments = new PaymentService(prisma as any);
afterAll(() => prisma.$disconnect());

async function userWithBank(balance: number, held: number) {
  const u = await prisma.user.create({
    data: { email: `p${Math.random()}@test`, password: await bcrypt.hash('mk', 4), balance, held },
  });
  const bank = await prisma.userBankAccount.create({
    data: { userId: u.id, bankCode: 'MB', bankName: 'MBBank', accountNo: `${Math.random()}`, accountName: 'A' },
  });
  return { u, bank };
}

describe('PaymentService với số dư Decimal và ký quỹ', () => {
  it('rút tiền chỉ trừ phần khả dụng, không đụng tiền đang giữ', async () => {
    const { u, bank } = await userWithBank(100_000, 500_000);
    await expect(payments.withdraw(u.id, 200_000, bank.id, 'mk')).rejects.toThrow('Số dư');
    await payments.withdraw(u.id, 60_000, bank.id, 'mk');
    const after = await prisma.user.findUniqueOrThrow({ where: { id: u.id } });
    expect(Number(after.balance)).toBe(40_000);
    expect(Number(after.held)).toBe(500_000);
  });

  it('getBalance trả số (không phải chuỗi Decimal), kèm phần đang giữ', async () => {
    const { u } = await userWithBank(123_000, 7_000);
    expect(await payments.getBalance(u.id)).toEqual({ success: true, data: { balance: 123_000, held: 7_000 } });
  });

  it('từ chối lệnh rút thì cộng lại đúng số tiền', async () => {
    const { u, bank } = await userWithBank(100_000, 0);
    const w = await payments.withdraw(u.id, 50_000, bank.id, 'mk');
    await payments.rejectWithdrawal(w.data.id);
    expect(Number((await prisma.user.findUniqueOrThrow({ where: { id: u.id } })).balance)).toBe(100_000);
  });
});
