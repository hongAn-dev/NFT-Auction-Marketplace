import {
  WebSocketGateway,
  WebSocketServer,
  SubscribeMessage,
  OnGatewayInit,
  OnGatewayConnection,
  OnGatewayDisconnect,
} from '@nestjs/websockets';
import { Server, Socket } from 'socket.io';
import { Logger } from '@nestjs/common';
import Redis from 'ioredis';

@WebSocketGateway({
  cors: {
    origin: '*',
  },
})
export class BiddingGateway
  implements OnGatewayInit, OnGatewayConnection, OnGatewayDisconnect
{
  @WebSocketServer() server: Server;
  private logger: Logger = new Logger('BiddingGateway');
  private redisSubscriber: Redis;

  afterInit(server: Server) {
    this.logger.log('🌐 Socket.IO WebSocket Gateway đã được khởi tạo!');
    this.initRedisSubscriber();
  }

  handleConnection(client: Socket) {
    this.logger.log(`🔌 Client kết nối: ${client.id}`);
  }

  handleDisconnect(client: Socket) {
    this.logger.log(`🔌 Client ngắt kết nối: ${client.id}`);
  }

  private initRedisSubscriber() {
    const redisHost = process.env.REDIS_HOST || 'localhost';
    const redisPort = parseInt(process.env.REDIS_PORT || '6379', 10);

    this.logger.log(`🔍 Đang khởi tạo Redis Subscriber kết nối tới ${redisHost}:${redisPort}...`);

    this.redisSubscriber = new Redis({
      host: redisHost,
      port: redisPort,
      retryStrategy: (times) => {
        // Tự động kết nối lại sau 2 giây nếu bị ngắt kết nối
        return Math.min(times * 100, 2000);
      },
    });

    this.redisSubscriber.on('connect', () => {
      this.logger.log('✅ Redis Subscriber kết nối thành công!');
    });

    this.redisSubscriber.on('error', (err) => {
      // Bỏ qua log lỗi kết nối khi ở chế độ subscriber của ioredis nếu không ảnh hưởng
      if (err.message && err.message.includes('only subscriber commands may be used')) {
        return;
      }
      this.logger.error('⚠️ Lỗi kết nối Redis Subscriber:', err);
    });

    // Lắng nghe tất cả các kênh có dạng bid:updated:<nftId>
    // Gọi psubscribe trực tiếp để ioredis tự động xếp hàng sau các lệnh bắt tay khởi tạo nội bộ
    this.redisSubscriber.psubscribe('bid:updated:*', (err) => {
      if (err) {
        this.logger.error('⚠️ Lỗi đăng ký kênh psubscribe trên Redis:', err);
      } else {
        this.logger.log('📡 Đã đăng ký lắng nghe (psubscribe) thành công trên kênh: bid:updated:*');
      }
    });

    // Lắng nghe các tin nhắn từ các kênh đăng ký dạng Pattern
    this.redisSubscriber.on('pmessage', (pattern, channel, message) => {
      try {
        this.logger.log(`📥 Nhận sự kiện từ Redis Pub/Sub trên kênh [${channel}]`);
        
        // Trích xuất nftId từ kênh (dạng bid:updated:nft-100)
        const parts = channel.split(':');
        const nftId = parts[parts.length - 1];

        const rawEvent = JSON.parse(message);

        // Định dạng lại thông điệp trả về đồng bộ với format REST API camelCase cho Frontend
        const socketPayload = {
          bidId: rawEvent.bid_id,
          nftId: rawEvent.nft_id,
          userId: rawEvent.user_id,
          amount: rawEvent.amount,
          createdAt: new Date(rawEvent.created_at * 1000).toISOString(),
        };

        // Broadcast sự kiện tới room tương ứng: room:nft:<nftId>
        const roomName = `room:nft:${nftId}`;
        this.server.to(roomName).emit('bid:updated', socketPayload);
        this.logger.log(`⚡ Broadcasted bid:updated tới các Client đang xem NFT [${nftId}] (Room: ${roomName})`);
      } catch (err) {
        this.logger.error('⚠️ Lỗi phân tích cú pháp sự kiện Redis Pub/Sub:', err);
      }
    });
  }

  @SubscribeMessage('joinRoom')
  handleJoinRoom(client: Socket, payload: { nftId: string }) {
    if (!payload || !payload.nftId) {
      return { status: 'error', message: 'Vui lòng cung cấp mã nftId!' };
    }
    const roomName = `room:nft:${payload.nftId}`;
    client.join(roomName);
    this.logger.log(`📥 Client ${client.id} đã THAM GIA vào phòng: ${roomName}`);
    return { status: 'success', joined: roomName };
  }

  @SubscribeMessage('leaveRoom')
  handleLeaveRoom(client: Socket, payload: { nftId: string }) {
    if (!payload || !payload.nftId) {
      return { status: 'error', message: 'Vui lòng cung cấp mã nftId!' };
    }
    const roomName = `room:nft:${payload.nftId}`;
    client.leave(roomName);
    this.logger.log(`📤 Client ${client.id} đã RỜI khỏi phòng: ${roomName}`);
    return { status: 'success', left: roomName };
  }
}
