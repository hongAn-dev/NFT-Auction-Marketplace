import { Injectable, ConflictException, UnauthorizedException, BadRequestException } from '@nestjs/common';
import { PrismaService } from '../prisma/prisma.service';
import { RedisService } from '../redis/redis.service';
import { JwtService } from '@nestjs/jwt';
import * as bcrypt from 'bcryptjs';
import { RegisterDto } from './dto/register.dto';
import { LoginDto } from './dto/login.dto';

@Injectable()
export class AuthService {
  constructor(
    private readonly prisma: PrismaService,
    private readonly redis: RedisService,
    private readonly jwtService: JwtService,
  ) {}

  async register(dto: RegisterDto) {
    const existingUser = await this.prisma.user.findUnique({
      where: { email: dto.email },
    });

    if (existingUser) {
      throw new ConflictException('Email này đã được sử dụng');
    }

    const hashedPassword = await bcrypt.hash(dto.password, 10);

    const user = await this.prisma.user.create({
      data: {
        email: dto.email,
        password: hashedPassword,
        role: 'user',
      },
    });

    return {
      success: true,
      message: 'Đăng ký tài khoản thành công',
      data: {
        id: user.id,
        email: user.email,
        role: user.role,
      },
    };
  }

  async login(dto: LoginDto) {
    const user = await this.prisma.user.findUnique({
      where: { email: dto.email },
    });

    if (!user) {
      throw new UnauthorizedException('Email hoặc mật khẩu không chính xác');
    }

    const isPasswordValid = await bcrypt.compare(dto.password, user.password);
    if (!isPasswordValid) {
      throw new UnauthorizedException('Email hoặc mật khẩu không chính xác');
    }

    const tokens = await this.generateTokens(user.id, user.email, user.role);

    // Save refresh token to Redis (7 days TTL)
    await this.redis.set(`auth:refresh:${user.id}`, tokens.refreshToken, 7 * 24 * 60 * 60);

    return {
      success: true,
      data: {
        user: {
          id: user.id,
          email: user.email,
          role: user.role,
        },
        ...tokens,
      },
    };
  }

  async logout(userId: string, accessToken: string) {
    // 1. Remove Refresh Token from Redis
    await this.redis.del(`auth:refresh:${userId}`);

    // 2. Blacklist Access Token (15 mins TTL)
    // Save to blacklist in Redis so Gateway can verify blacklist
    await this.redis.set(`auth:blacklist:${accessToken}`, '1', 15 * 60);

    return {
      success: true,
      message: 'Đăng xuất thành công',
    };
  }

  async refreshTokens(refreshToken: string) {
    try {
      // Decode and verify the refresh token
      const secret = process.env.JWT_REFRESH_SECRET || 'super-secret-refresh-key';
      const payload = this.jwtService.verify(refreshToken, { secret });
      
      const userId = payload.sub;
      const email = payload.email;
      const role = payload.role;

      // Check if refresh token is registered in Redis
      const cachedToken = await this.redis.get(`auth:refresh:${userId}`);
      if (!cachedToken || cachedToken !== refreshToken) {
        throw new UnauthorizedException('Refresh token đã bị vô hiệu hóa hoặc hết hạn');
      }

      // Generate new pair
      const tokens = await this.generateTokens(userId, email, role);

      // Save new refresh token in Redis (7 days)
      await this.redis.set(`auth:refresh:${userId}`, tokens.refreshToken, 7 * 24 * 60 * 60);

      return {
        success: true,
        data: tokens,
      };
    } catch (e) {
      throw new UnauthorizedException('Refresh token không hợp lệ hoặc đã hết hạn');
    }
  }

  async getNonce(address: string) {
    // Tạo chuỗi nonce ngẫu nhiên gồm 6 chữ số
    const nonce = Math.floor(100000 + Math.random() * 900000).toString();
    
    // Lưu nonce vào Redis (hạn dùng 5 phút) liên kết với ví
    await this.redis.set(`auth:nonce:${address.toLowerCase()}`, nonce, 5 * 60);
    
    return {
      success: true,
      data: {
        nonce,
        message: `Đăng nhập vào Curatorial NFT. Nonce của bạn là: ${nonce}`,
      },
    };
  }

  async verifySignature(address: string, signature: string) {
    // 1. Lấy nonce từ Redis
    const cachedNonce = await this.redis.get(`auth:nonce:${address.toLowerCase()}`);
    if (!cachedNonce) {
      throw new BadRequestException('Nonce đã hết hạn hoặc không tồn tại. Vui lòng lấy nonce mới!');
    }

    // 2. Xác thực chữ ký số bằng thư viện ethers
    const message = `Đăng nhập vào Curatorial NFT. Nonce của bạn là: ${cachedNonce}`;
    let recoveredAddress: string;
    try {
      const { ethers } = require('ethers');
      recoveredAddress = ethers.verifyMessage(message, signature);
    } catch (err) {
      throw new UnauthorizedException('Chữ ký số cung cấp không hợp lệ');
    }

    if (recoveredAddress.toLowerCase() !== address.toLowerCase()) {
      throw new UnauthorizedException('Chữ ký số không khớp với địa chỉ ví cung cấp');
    }

    // 3. Xóa nonce đã dùng khỏi Redis
    await this.redis.del(`auth:nonce:${address.toLowerCase()}`);

    // 4. Tìm kiếm hoặc khởi tạo User Web3 trong database
    const email = `${address.toLowerCase()}@web3.auth`;
    let user = await this.prisma.user.findUnique({
      where: { email },
    });

    if (!user) {
      // Đăng ký tài khoản tự động với mật khẩu ngẫu nhiên bảo mật
      const randomPassword = await bcrypt.hash(Math.random().toString(36), 10);
      user = await this.prisma.user.create({
        data: {
          email,
          password: randomPassword,
          role: 'user',
        },
      });
    }

    // 5. Cấp phát cặp JWT Tokens
    const tokens = await this.generateTokens(user.id, user.email, user.role);

    // Lưu refresh token vào Redis (TTL 7 ngày)
    await this.redis.set(`auth:refresh:${user.id}`, tokens.refreshToken, 7 * 24 * 60 * 60);

    return {
      success: true,
      data: {
        user: {
          id: user.id,
          email: user.email,
          role: user.role,
        },
        ...tokens,
      },
    };
  }

  private async generateTokens(userId: string, email: string, role: string) {
    const payload = { sub: userId, email, role };

    const accessSecret = process.env.JWT_SECRET || 'your-super-secret-key-change-this';
    const refreshSecret = process.env.JWT_REFRESH_SECRET || 'super-secret-refresh-key';

    const [accessToken, refreshToken] = await Promise.all([
      this.jwtService.signAsync(payload, {
        secret: accessSecret,
        expiresIn: '15m',
      }),
      this.jwtService.signAsync(payload, {
        secret: refreshSecret,
        expiresIn: '7d',
      }),
    ]);

    return {
      accessToken,
      refreshToken,
    };
  }
}
