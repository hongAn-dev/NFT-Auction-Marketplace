import { Controller, Post, Body, Get, Param, Inject, OnModuleInit, HttpException, HttpStatus, UseGuards, Req } from '@nestjs/common';
import { ClientGrpc } from '@nestjs/microservices';
import { IsNotEmpty, IsNumber, IsString, Min } from 'class-validator';
import { firstValueFrom, Observable } from 'rxjs';
import { JwtAuthGuard } from '../guards/jwt-auth.guard';

class PlaceBidDto {
  @IsString()
  @IsNotEmpty()
  nftId: string;

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
  @UseGuards(JwtAuthGuard)
  async placeBid(@Req() req: any, @Body() placeBidDto: PlaceBidDto) {
    try {
      const userId = req.user.id;
      const email = req.user.email;
      const isWeb3User = email.endsWith('@web3.auth');
      const bidAmount = placeBidDto.amount;

      if (!isWeb3User) {
        // Web2 User: Validate credits balance from auth-service (1 USD = 25,000 VND)
        const bidAmountVND = bidAmount * 25000;
        const authServiceUrl = process.env.AUTH_SERVICE_URL || 'http://auth-service:4001';
        try {
          const balanceRes = await fetch(`${authServiceUrl}/auth/payment/balance`, {
            headers: { 'Authorization': req.headers.authorization }
          });
          if (!balanceRes.ok) {
            throw new HttpException(
              {
                success: false,
                error: {
                  code: 'BALANCE_VERIFICATION_FAILED',
                  message: 'Không thể kết nối đến hệ thống tài khoản để xác thực số dư.',
                },
              },
              HttpStatus.BAD_REQUEST,
            );
          }
          const balanceData = await balanceRes.json();
          if (!balanceData.success || balanceData.data?.balance === undefined || balanceData.data.balance < bidAmountVND) {
            const currentBal = balanceData.data?.balance ?? 0;
            throw new HttpException(
              {
                success: false,
                error: {
                  code: 'INSUFFICIENT_BALANCE',
                  message: `Số dư tài khoản không đủ. Số dư hiện tại: ${currentBal.toLocaleString('vi-VN')} VND. Giá thầu đặt: ${bidAmountVND.toLocaleString('vi-VN')} VND.`,
                },
              },
              HttpStatus.BAD_REQUEST,
            );
          }
        } catch (e) {
          if (e instanceof HttpException) throw e;
          console.error('Balance verification error:', e);
          throw new HttpException(
            {
              success: false,
              error: {
                code: 'BALANCE_VERIFICATION_FAILED',
                message: 'Có lỗi hệ thống xảy ra khi xác thực số dư tài khoản.',
              },
            },
            HttpStatus.INTERNAL_SERVER_ERROR,
          );
        }
      } else {
        // Web3 User: Validate ETH balance from EVM RPC node (1 ETH = 3,000 USD)
        const bidAmountETH = bidAmount / 3000;
        const address = email.split('@')[0];
        try {
          // Query the local Hardhat Node or blockchain service node
          const rpcUrl = 'http://blockchain-service:8545';
          const rpcRes = await fetch(rpcUrl, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              jsonrpc: '2.0',
              method: 'eth_getBalance',
              params: [address, 'latest'],
              id: 1,
            }),
          });
          if (rpcRes.ok) {
            const rpcData = await rpcRes.json();
            if (rpcData.result) {
              const wei = BigInt(rpcData.result);
              const ethBalance = Number(wei) / 1e18;
              if (ethBalance < bidAmountETH) {
                throw new HttpException(
                  {
                    success: false,
                    error: {
                      code: 'INSUFFICIENT_BALANCE',
                      message: `Số dư ETH ví của bạn không đủ. Số dư ví: ${ethBalance.toFixed(4)} ETH. Giá thầu đặt: ${bidAmountETH.toFixed(4)} ETH.`,
                    },
                  },
                  HttpStatus.BAD_REQUEST,
                );
              }
            }
          }
        } catch (e) {
          if (e instanceof HttpException) throw e;
          console.warn('EVM RPC node unreachable, relying on frontend Web3 validation:', e);
        }
      }

      const reqPayload = {
        nft_id: placeBidDto.nftId,
        user_id: userId,
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
