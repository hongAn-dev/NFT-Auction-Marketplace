import { Module } from '@nestjs/common';
import { JwtModule } from '@nestjs/jwt';
import { PassportModule } from '@nestjs/passport';
import { AuthService } from './auth.service';
import { AuthController } from './auth.controller';
import { PaymentService } from './payment.service';
import { PaymentController } from './payment.controller';
import { JwtStrategy } from './jwt.strategy';

@Module({
  imports: [
    PassportModule.register({ defaultStrategy: 'jwt' }),
    JwtModule.register({}),
  ],
  controllers: [AuthController, PaymentController],
  providers: [AuthService, PaymentService, JwtStrategy],
  exports: [AuthService, PaymentService],
})
export class AuthModule {}

