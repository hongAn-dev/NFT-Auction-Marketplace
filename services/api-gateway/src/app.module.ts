import { Module } from '@nestjs/common';
import { ClientsModule, Transport } from '@nestjs/microservices';
import { join } from 'path';
import { BiddingController } from './controllers/bidding.controller';
import { AuthController } from './controllers/auth.controller';
import { BiddingGateway } from './gateways/bidding.gateway';

// Xử lý làm sạch URL kết nối gRPC
let biddingServiceUrl = process.env.BIDDING_SERVICE_URL || 'localhost:50051';
biddingServiceUrl = biddingServiceUrl.replace(/^https?:\/\//, '');
if (!biddingServiceUrl.includes(':')) {
  if (biddingServiceUrl === 'localhost') {
    biddingServiceUrl += ':50051';
  } else {
    biddingServiceUrl += ':443'; // Cổng mặc định cho SSL gRPC trên Render
  }
}

@Module({
  imports: [
    ClientsModule.register([
      {
        name: 'BIDDING_PACKAGE',
        transport: Transport.GRPC,
        options: {
          package: 'bidding',
          protoPath: join(process.cwd(), 'proto/bidding.proto'),
          url: biddingServiceUrl,
          loader: {
            keepCase: true,
          },
        },
      },
    ]),
  ],
  controllers: [BiddingController, AuthController],
  providers: [BiddingGateway],
})
export class AppModule {}
