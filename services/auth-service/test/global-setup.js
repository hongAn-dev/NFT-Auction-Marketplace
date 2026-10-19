// Test chạy trên Postgres thật. DATABASE_URL phải trỏ tới DB thử (schema riêng),
// không bao giờ tới DB của sàn: mỗi lần chạy xoá sạch schema rồi dựng lại.
const { execSync } = require('child_process');

module.exports = async () => {
  const url = process.env.DATABASE_URL || '';
  if (!/schema=auth_test/.test(url)) {
    throw new Error('DATABASE_URL phải trỏ tới schema auth_test (xem docs/24b, Task 1)');
  }
  execSync('npx prisma db push --force-reset --skip-generate', { stdio: 'inherit' });
};
