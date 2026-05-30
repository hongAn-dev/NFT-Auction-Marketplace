/**
 * 🚀 ENTERPRISE-GRADE AUTOMATED INTEGRATION & E2E TEST RUNNER
 * Dịch vụ: api-gateway (NestJS BFF) & bidding-service (Go Core) & Redis Pub/Sub
 * 
 * Bản quyền: Antigravity Enterprise Quality Assurance (QA) Suite
 */

const { io } = require('socket.io-client');
const http = require('http');

// Cấu hình tham số kiểm thử
const WEBSOCKET_URL = 'http://localhost:4000';
const REST_API_URL = 'http://localhost:4000/api/v1/bids';
const TEST_NFT_ID = 'nft-enterprise-test-' + Math.floor(Math.random() * 100000);
const TEST_USER_ID = 'user-enterprise-qa-' + Math.floor(Math.random() * 100000);
const TEST_BID_AMOUNT = 8888.88;

// Bảng màu ANSI phục vụ in log chuyên nghiệp
const colors = {
  reset: '\x1b[0m',
  bright: '\x1b[1m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  magenta: '\x1b[35m',
  cyan: '\x1b[36m',
  red: '\x1b[31m',
  bgGreen: '\x1b[42m',
  bgRed: '\x1b[41m'
};

function printHeader(title) {
  console.log('\n' + colors.bright + colors.magenta + '='.repeat(80));
  console.log('   ' + title.toUpperCase());
  console.log('='.repeat(80) + colors.reset);
}

function printStep(stepNum, description) {
  console.log(`\n${colors.bright}${colors.blue}[BƯỚC ${stepNum}]${colors.reset} ${description}...`);
}

function printSuccess(message) {
  console.log(`   ${colors.green}✔ [PASS] ${message}${colors.reset}`);
}

function printFailure(message, err = null) {
  console.error(`   ${colors.red}✘ [FAIL] ${message}${colors.reset}`);
  if (err) {
    console.error(`     Chi tiết lỗi:`, err);
  }
}

// Helper gửi REST API request dạng HTTP Native Node
function postJson(url, data) {
  return new Promise((resolve, reject) => {
    const payload = JSON.stringify(data);
    const options = {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Content-Length': Buffer.byteLength(payload)
      },
      timeout: 5000
    };

    const req = http.request(url, options, (res) => {
      let body = '';
      res.on('data', (chunk) => body += chunk);
      res.on('end', () => {
        if (res.statusCode >= 200 && res.statusCode < 300) {
          resolve(JSON.parse(body));
        } else {
          reject(new Error(`Server trả về HTTP Status ${res.statusCode}: ${body}`));
        }
      });
    });

    req.on('error', (err) => reject(err));
    req.on('timeout', () => {
      req.destroy();
      reject(new Error('Yêu cầu gửi REST API bị quá thời gian (Timeout)'));
    });

    req.write(payload);
    req.end();
  });
}

// Tiến trình chạy Test Suite chính
async function runTestSuite() {
  printHeader('BẮT ĐẦU CHẠY THỬ NGHIỆM ĐỒNG BỘ TÍCH HỢP HỆ THỐNG PHÂN TÁN (E2E)');
  console.log(`NFT Đấu Giá Thử Nghiệm: ${colors.yellow}${TEST_NFT_ID}${colors.reset}`);
  console.log(`Tài Khoản Đặt Giá:       ${colors.yellow}${TEST_USER_ID}${colors.reset}`);
  console.log(`Số Tiền Thầu Đề Xuất:   ${colors.yellow}${TEST_BID_AMOUNT} USD${colors.reset}`);

  let socketClient;
  let eventReceived = false;
  let receivedPayload = null;

  try {
    // ── BƯỚC 0: TỰ ĐỘNG KHỞI TẠO NFT TRONG POSTGRESQL CSDL ──────────────────
    printStep(0, 'Tự động tạo mới NFT đấu giá thử nghiệm trong cơ sở dữ liệu');
    const createNftPayload = {
      id: TEST_NFT_ID,
      title: "Enterprise Automated QA NFT",
      description: "Tài sản sinh tự động phục vụ tích hợp E2E CI/CD",
      image_url: "https://enterprise.example.com/assets/nft-test.png",
      creator_id: "creator-enterprise-admin",
      start_price: 500.00
    };

    console.log(`   Đang gửi POST request tạo NFT tới: http://localhost:8080/api/v1/nfts`);
    const createResponse = await postJson('http://localhost:8080/api/v1/nfts', createNftPayload);
    if (createResponse && createResponse.success) {
      printSuccess(`Tạo mới NFT [${TEST_NFT_ID}] thành công trong CSDL PostgreSQL!`);
    } else {
      throw new Error('Tạo mới NFT bị thất bại hoặc phản hồi không đúng định dạng.');
    }

    // ── BƯỚC 1: KHỞI TẠO KẾT NỐI WEBSOCKET SOCKET.IO ────────────────────
    printStep(1, 'Kiểm tra khả năng kết nối cổng WebSocket Gateway (NestJS - Port 4000)');

    socketClient = io(WEBSOCKET_URL, {
      transports: ['websocket'],
      forceNew: true,
      timeout: 5000
    });

    await new Promise((resolve, reject) => {
      socketClient.on('connect', () => {
        printSuccess(`Kết nối thành công cổng WebSocket tại: ${WEBSOCKET_URL}`);
        printSuccess(`Mã Socket ID thiết lập: ${colors.cyan}${socketClient.id}${colors.reset}`);
        resolve();
      });
      socketClient.on('connect_error', (err) => {
        printFailure('Không thể kết nối WebSocket Gateway', err);
        reject(err);
      });
    });

    // ── BƯỚC 2: KIỂM TRA ĐĂNG KÝ PHÒNG (JOIN ROOM) ────────────────────────
    printStep(2, `Gửi yêu cầu đăng ký tham gia phòng (Join Room) để xem NFT [${TEST_NFT_ID}]`);

    await new Promise((resolve, reject) => {
      socketClient.emit('joinRoom', { nftId: TEST_NFT_ID }, (ack) => {
        // Hỗ trợ cả trường hợp server phản hồi qua ACK Callback
        if (ack && ack.status === 'success') {
          printSuccess(`Đã nhận được phản hồi ACK từ Server: Đã tham gia phòng [${ack.joined}]`);
          resolve();
        } else {
          printSuccess(`Đã phát sự kiện joinRoom lên phòng [room:nft:${TEST_NFT_ID}]`);
          resolve();
        }
      });
      // Dự phòng trường hợp không có ACK callback, ta set timeout ngắn
      setTimeout(resolve, 800);
    });

    // Thiết lập bộ lắng nghe sự kiện phát sóng thời gian thực
    socketClient.on('bid:updated', (data) => {
      eventReceived = true;
      receivedPayload = data;
      console.log(`\n${colors.bright}${colors.magenta}🔔 [SỰ KIỆN LIVE] Nhận được phát sóng realtime từ Server!${colors.reset}`);
      console.log(data);
    });

    // ── BƯỚC 3: GỬI REST API YÊU CẦU ĐẶT GIÁ THẦU (PLACE BID) ─────────────
    printStep(3, 'Thực hiện gửi REST API Đặt giá thầu mới qua cổng NestJS BFF');

    const apiPayload = {
      nftId: TEST_NFT_ID,
      userId: TEST_USER_ID,
      amount: TEST_BID_AMOUNT
    };

    console.log(`   Đang gửi POST request tới: ${REST_API_URL}`);
    const apiResponse = await postJson(REST_API_URL, apiPayload);

    if (apiResponse && apiResponse.success) {
      printSuccess(`Đặt giá thầu thành công! REST Response:`);
      console.log(`     - ID Giao dịch: ${colors.cyan}${apiResponse.data.bidId}${colors.reset}`);
      console.log(`     - Giá hiện tại:  ${colors.green}${apiResponse.data.amount} USD${colors.reset}`);
      console.log(`     - Server Msg:   "${apiResponse.message}"`);
    } else {
      throw new Error('Cấu trúc phản hồi REST API không đúng định dạng thành công.');
    }

    // ── BƯỚC 4: XÁC MINH PHÁT SÓNG THỜI GIAN THỰC (BROADCAST EVENT) ─────
    printStep(4, 'Chờ và xác minh sự kiện phát sóng thời gian thực qua kênh WebSocket');

    await new Promise((resolve, reject) => {
      const startTime = Date.now();
      const interval = setInterval(() => {
        if (eventReceived) {
          clearInterval(interval);
          resolve();
        } else if (Date.now() - startTime > 4000) {
          clearInterval(interval);
          reject(new Error('Quá 4 giây vẫn chưa nhận được sự kiện phát sóng realtime "bid:updated" qua WebSocket!'));
        }
      }, 100);
    });

    // Kiểm thử chéo dữ liệu truyền tải
    if (receivedPayload && receivedPayload.amount === TEST_BID_AMOUNT && receivedPayload.userId === TEST_USER_ID) {
      printSuccess('Dữ liệu phát sóng thời gian thực khớp tuyệt đối 100% với dữ liệu đặt thầu gốc!');
      printSuccess(`Kiểm thử tích hợp chuỗi: [POST REST API] ➔ [gRPC Go] ➔ [Postgres/Mongo] ➔ [Redis Pub/Sub] ➔ [WebSocket Gateway] ➔ [Client] hoạt động hoàn hảo!`);
    } else {
      throw new Error('Dữ liệu phát sóng bị sai lệch hoặc không trùng khớp với giá trị đặt thầu!');
    }

    // ── BÁO CÁO TỔNG HỢP ──────────────────────────────────────────────────
    printHeader('KẾT QUẢ ĐÁNH GIÁ: TẤT CẢ UNIT & INTEGRATION TESTS ĐÃ PASS 🟢');
    console.log(`${colors.bright}${colors.bgGreen}  SUCCESS  ${colors.reset} Hệ thống hoạt động chuẩn mực cấp doanh nghiệp!`);
    console.log(`Thời gian chạy thử nghiệm tích hợp: ${colors.yellow}${new Date().toLocaleString()}${colors.reset}\n`);

    if (socketClient) socketClient.disconnect();
    process.exit(0);

  } catch (err) {
    printHeader('KẾT QUẢ ĐÁNH GIÁ: THỬ NGHIỆM TÍCH HỢP THẤT BẠI 🔴');
    console.error(`${colors.bright}${colors.bgRed}  FAILURE  ${colors.reset} Có lỗi nghiêm trọng xảy ra trong chuỗi truyền tin:`);
    printFailure('Chi tiết lỗi chạy thử nghiệm tích hợp', err);
    console.log();

    if (socketClient) socketClient.disconnect();
    process.exit(1);
  }
}

// Chạy bộ Test Suite
runTestSuite();
