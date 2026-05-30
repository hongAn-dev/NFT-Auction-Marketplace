import { Controller, Post, Body, Get, Param, Inject, OnModuleInit, HttpException, HttpStatus } from '@nestjs/common';
import { ClientGrpc } from '@nestjs/microservices';
import { IsNotEmpty, IsNumber, IsString, Min } from 'class-validator';
import { firstValueFrom, Observable } from 'rxjs';

class PlaceBidDto {
  @IsString()
  @IsNotEmpty()
  nftId: string;

  @IsString()
  @IsNotEmpty()
  userId: string;

  @IsNumber()
  @Min(0.01)
  amount: number;
}

interface BiddingServiceGrpc {
  placeBid(data: { nft_id: string; user_id: string; amount: number }): Observable<any>;
  getHighestBid(data: { nft_id: string }): Observable<any>;
}

@Controller('api/v1')
export class BiddingController implements OnModuleInit {
  private biddingService: BiddingServiceGrpc;

  constructor(@Inject('BIDDING_PACKAGE') private client: ClientGrpc) {}

  onModuleInit() {
    this.biddingService = this.client.getService<BiddingServiceGrpc>('BiddingService');
  }

  @Post('bids')
  async placeBid(@Body() placeBidDto: PlaceBidDto) {
    try {
      // Chuyển đổi từ DTO sang dạng snake_case khớp với trường dữ liệu gRPC
      const reqPayload = {
        nft_id: placeBidDto.nftId,
        user_id: placeBidDto.userId,
        amount: placeBidDto.amount,
      };

      const result = await firstValueFrom(this.biddingService.placeBid(reqPayload));
      
      if (!result.success) {
        throw new HttpException(
          {
            success: false,
            error: {
              code: 'BID_FAILED',
              message: result.message,
            },
          },
          HttpStatus.BAD_REQUEST,
        );
      }

      // Trả về dữ liệu chuẩn REST API format camelCase
      return {
        success: true,
        data: {
          bidId: result.bid.id,
          nftId: result.bid.nft_id,
          userId: result.bid.user_id,
          amount: result.bid.amount,
          createdAt: new Date(result.bid.created_at * 1000).toISOString(),
        },
        message: result.message,
      };
    } catch (err) {
      if (err instanceof HttpException) {
        throw err;
      }
      throw new HttpException(
        {
          success: false,
          error: {
            code: 'INTERNAL_SERVER_ERROR',
            message: err.message || 'Lỗi hệ thống khi đặt thầu',
          },
        },
        HttpStatus.INTERNAL_SERVER_ERROR,
      );
    }
  }

  @Get('nfts/:nftId/highest-bid')
  async getHighestBid(@Param('nftId') nftId: string) {
    try {
      const result = await firstValueFrom(this.biddingService.getHighestBid({ nft_id: nftId }));
      
      if (!result.success || !result.highest_bid) {
        return {
          success: true,
          data: null,
          message: 'Chưa có lượt đặt thầu nào cho NFT này.',
        };
      }

      return {
        success: true,
        data: {
          bidId: result.highest_bid.id,
          nftId: result.highest_bid.nft_id,
          userId: result.highest_bid.user_id,
          amount: result.highest_bid.amount,
          createdAt: new Date(result.highest_bid.created_at * 1000).toISOString(),
        },
      };
    } catch (err) {
      throw new HttpException(
        {
          success: false,
          error: {
            code: 'INTERNAL_SERVER_ERROR',
            message: err.message || 'Lỗi hệ thống khi lấy giá thầu cao nhất',
          },
        },
        HttpStatus.INTERNAL_SERVER_ERROR,
      );
    }
  }
}
