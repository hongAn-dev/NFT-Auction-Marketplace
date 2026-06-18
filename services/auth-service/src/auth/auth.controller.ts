import { Controller, Post, Body, UseGuards, Req, Headers, UnauthorizedException } from '@nestjs/common';
import { AuthService } from './auth.service';
import { RegisterDto } from './dto/register.dto';
import { LoginDto } from './dto/login.dto';
import { RefreshTokenDto } from './dto/refresh-token.dto';
import { AuthGuard } from '@nestjs/passport';

@Controller('auth')
export class AuthController {
  constructor(private readonly authService: AuthService) {}

  @Post('register')
  async register(@Body() dto: RegisterDto) {
    return this.authService.register(dto);
  }

  @Post('login')
  async login(@Body() dto: LoginDto) {
    return this.authService.login(dto);
  }

  @Post('web3/nonce')
  async getNonce(@Body('address') address: string) {
    if (!address) {
      throw new UnauthorizedException('Vui lòng cung cấp địa chỉ ví');
    }
    return this.authService.getNonce(address);
  }

  @Post('web3/verify')
  async verifySignature(
    @Body('address') address: string,
    @Body('signature') signature: string,
  ) {
    if (!address || !signature) {
      throw new UnauthorizedException('Vui lòng cung cấp địa chỉ ví và chữ ký số');
    }
    return this.authService.verifySignature(address, signature);
  }

  @Post('refresh')
  async refresh(@Body() dto: RefreshTokenDto) {
    return this.authService.refreshTokens(dto.refreshToken);
  }

  @Post('logout')
  @UseGuards(AuthGuard('jwt'))
  async logout(@Req() req: any) {
    const userId = req.user.id;
    const authHeader = req.headers.authorization;
    if (!authHeader) {
      throw new UnauthorizedException('Không tìm thấy token');
    }
    const token = authHeader.split(' ')[1];
    return this.authService.logout(userId, token);
  }
}
