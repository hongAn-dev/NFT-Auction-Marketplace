import { Module } from '@nestjs/common';
import { ClientsModule, Transport } from '@nestjs/microservices';
import { join } from 'path';
import { BiddingController } from './controllers/bidding.controller';
import { BiddingGateway } from './gateways/bidding.gateway';

@Module({
  imports: [
    ClientsModule.register([
      {
        name: 'BIDDING_PACKAGE',
        transport: Transport.GRPC,
        options: {
          package: 'bidding',
          protoPath: join(process.cwd(), 'proto/bidding.proto'),
          url: process.env.BIDDING_SERVICE_URL || 'localhost:50051',
          loader: {
            keepCase: true,
          },
        },
      },
    ]),
  ],
  controllers: [BiddingController],
  providers: [BiddingGateway],
})
export class AppModule {}
